package dns

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DefaultPort    = 53535 // unprivileged; routed to by systemd-resolved
	defaultTTL     = 60
	defaultTimeout = 2 * time.Second
)

// Server answers *.test locally and forwards other queries upstream.
type Server struct {
	Domain    string // e.g. "test"
	Port      int
	Upstreams []string // "host:port"; discovered when empty
	Timeout   time.Duration
	IPv4      [4]byte
	IPv6      [16]byte

	mu       sync.Mutex
	upstream *net.UDPConn
	tcpLn    net.Listener
	closed   bool
}

// NewServer returns a Server with loopback addresses and the given domain.
func NewServer(domain string, port int) *Server {
	if domain == "" {
		domain = "test"
	}
	if port == 0 {
		port = DefaultPort
	}
	s := &Server{Domain: strings.TrimSuffix(strings.ToLower(domain), "."), Port: port, IPv4: [4]byte{127, 0, 0, 1}}
	// ::1
	s.IPv6[15] = 1
	return s
}

// LocalName reports whether a query name belongs to the dev domain. It matches
// "<domain>." and "anything.<domain>." but not "notdomain." or "test.evil.".
func (s *Server) LocalName(name string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if name == s.Domain {
		return true
	}
	return strings.HasSuffix(name, "."+s.Domain)
}

// Resolve builds a response for a query. It is exported for testing and for
// callers that want to handle the UDP transport themselves.
func (s *Server) Resolve(query []byte) ([]byte, error) {
	q, err := ParseQuestion(query)
	if err != nil {
		return nil, err
	}
	if s.LocalName(q.Name) {
		switch q.Type {
		case typeA:
			return BuildResponse(query, ARecord(s.IPv4, defaultTTL))
		case typeAAAA:
			// NODATA, like dnsmasq's address=/test/127.0.0.1. Answering AAAA
			// ::1 makes getent prefer IPv6 while the web server listens on
			// IPv4 only, so the browser would get ECONNREFUSED.
			return BuildResponse(query)
		default:
			// NODATA: a valid empty answer rather than NXDOMAIN.
			return BuildResponse(query)
		}
	}
	return s.forward(query)
}

// ResolveIfLocal answers only names in the local domain, returning ok=false
// for everything else. Useful for callers that handle forwarding separately.
func (s *Server) ResolveIfLocal(query []byte) ([]byte, bool, error) {
	q, err := ParseQuestion(query)
	if err != nil {
		return nil, false, err
	}
	if !s.LocalName(q.Name) {
		return nil, false, nil
	}
	resp, err := s.Resolve(query)
	return resp, true, err
}

// DiscoverUpstreams reads nameservers from /etc/resolv.conf, falling back to
// systemd-resolved's 127.0.0.53 and the classic localhost resolver.
func DiscoverUpstreams(path string) []string {
	out := []string{}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "nameserver") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					addr := fields[1]
					if !strings.Contains(addr, ":") {
						addr += ":53"
					}
					out = append(out, addr)
				}
			}
		}
	}
	if len(out) == 0 {
		out = []string{"127.0.0.53:53", "127.0.0.1:53"}
	}
	return out
}

func (s *Server) upstreamAddrs() []string {
	if len(s.Upstreams) > 0 {
		return s.Upstreams
	}
	path := "/etc/resolv.conf"
	if v := os.Getenv("LAIR_RESOLV_CONF"); v != "" {
		path = v
	}
	return DiscoverUpstreams(path)
}

func (s *Server) forward(query []byte) ([]byte, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	var lastErr error
	for _, addr := range s.upstreamAddrs() {
		raddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			lastErr = err
			continue
		}
		conn.SetDeadline(time.Now().Add(timeout))
		_, err = conn.Write(query)
		if err == nil {
			buf := make([]byte, 4096)
			var n int
			n, _, err = conn.ReadFromUDP(buf)
			conn.Close()
			if err == nil {
				return append([]byte(nil), buf[:n]...), nil
			}
		} else {
			conn.Close()
		}
		lastErr = err
	}
	return nil, fmt.Errorf("forward DNS query: %w", lastErr)
}

// ListenAndServe starts the UDP DNS server and blocks until ctx is cancelled
// or Close is called. It always binds only 127.0.0.1.
func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: s.Port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen DNS on %s: %w", addr, err)
	}
	s.mu.Lock()
	s.upstream = conn
	s.mu.Unlock()

	// systemd-resolved switches a server to TCP when it considers UDP
	// degraded. Without a TCP listener those queries are refused with ECONNREFUSED
	// and *.test stops resolving even though UDP works. dnsmasq serves both.
	if tcpLn, err := net.Listen("tcp", addr.String()); err == nil {
		s.mu.Lock()
		s.tcpLn = tcpLn
		s.mu.Unlock()
		go s.serveTCP(ctx, tcpLn)
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		query := append([]byte(nil), buf[:n]...)
		go func() {
			resp, err := s.Resolve(query)
			if err != nil {
				resp, err = BuildNXDomain(query)
				if err != nil {
					return
				}
			}
			conn.WriteToUDP(resp, remote)
		}()
	}
}

// serveTCP handles the length-prefixed DNS messages used by systemd-resolved
// when it falls back from UDP to TCP.
func (s *Server) serveTCP(ctx context.Context, ln net.Listener) {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleTCPConn(conn)
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(defaultTimeout))
	for {
		var size [2]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint16(size[:]))
		if n == 0 || n > 4096 {
			return
		}
		query := make([]byte, n)
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}
		resp, err := s.Resolve(query)
		if err != nil {
			resp, err = BuildNXDomain(query)
			if err != nil {
				return
			}
		}
		out := make([]byte, 2+len(resp))
		binary.BigEndian.PutUint16(out[:2], uint16(len(resp)))
		copy(out[2:], resp)
		if _, err := conn.Write(out); err != nil {
			return
		}
	}
}

// Close stops the DNS server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var first error
	if s.upstream != nil {
		first = s.upstream.Close()
	}
	if s.tcpLn != nil {
		if err := s.tcpLn.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
