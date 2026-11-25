package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lair-sh/lair/internal/config"
)

// MailRequest is the wire payload of the `mail` daemon action. Port is an
// optional override: 0 uses config.json, -1 asks the kernel for a free port
// (used by tests and diagnostics). The desktop app only sends op and id.
type MailRequest struct {
	Op      string `json:"op"`
	ID      string `json:"id"`
	Port    int    `json:"port,omitempty"`
	Inbox   string `json:"inbox,omitempty"`
	Confirm bool   `json:"confirm,omitempty"` // required for op=clear
}

// MailAttachment is one file attached to a captured message. The body itself
// stays in the .eml file; only metadata is kept in the index.
type MailAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

// MailMessage is one captured email.
type MailMessage struct {
	ID          string           `json:"id"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Subject     string           `json:"subject"`
	Date        string           `json:"date"`
	Body        string           `json:"body"`
	Attachments []MailAttachment `json:"attachments,omitempty"`
}

// MailResult is returned by every mail operation. Env is always present so a
// project can be configured even while the catcher is stopped.
type MailResult struct {
	Running  bool          `json:"running"`
	Port     int           `json:"port"`
	Count    int           `json:"count"`
	Env      string        `json:"env"`
	Messages []MailMessage `json:"messages,omitempty"`
	Message  *MailMessage  `json:"message,omitempty"`
	Output   string        `json:"output,omitempty"`
	Raw      string        `json:"raw,omitempty"`
}

type mailIndex struct {
	Messages []MailMessage `json:"messages"`
}

const mailIndexFile = "index.json"

// mailServer is a minimal in-process ESMTP server. It follows the supervisor
// lifecycle (Start/Stop, context cancellation, idempotent calls) without
// spawning a separate process: the daemon already owns the listener.
type mailServer struct {
	dir   string
	inbox string
	mu    sync.Mutex

	port       int
	actualPort int
	ln         net.Listener
	cancel     context.CancelFunc
	conns      map[net.Conn]struct{}

	boxMu sync.Mutex
}

func newMailServer(root string, port int) *mailServer {
	return &mailServer{dir: root, inbox: filepath.Join(root, "inbox"), port: port}
}

func (s *mailServer) setPort(port int) {
	s.mu.Lock()
	s.port = port
	s.mu.Unlock()
}

func (s *mailServer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ln != nil
}

func (s *mailServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.actualPort > 0 {
		return s.actualPort
	}
	return s.port
}

// Start binds 127.0.0.1:<port> and starts accepting. It returns the bound
// port, which can differ from the requested one when 0 was requested.
func (s *mailServer) Start(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return s.actualPort, nil
	}
	if err := os.MkdirAll(s.inbox, 0o700); err != nil {
		return 0, fmt.Errorf("create mail inbox: %w", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("listen smtp on %s: %w", addr, err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.ln, s.cancel, s.conns = ln, cancel, map[net.Conn]struct{}{}
	s.actualPort = ln.Addr().(*net.TCPAddr).Port
	go s.acceptLoop(runCtx, ln)
	return s.actualPort, nil
}

// Stop closes the listener and every active session. It is safe to call twice.
func (s *mailServer) Stop() error {
	s.mu.Lock()
	ln, cancel, conns := s.ln, s.cancel, s.conns
	s.ln, s.cancel, s.conns, s.actualPort = nil, nil, nil, 0
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if ln != nil {
		_ = ln.Close()
	}
	for c := range conns {
		_ = c.Close()
	}
	return nil
}

func (s *mailServer) acceptLoop(ctx context.Context, ln net.Listener) {
	// If the caller's context dies before an explicit Stop, do not report a
	// dead listener as "running".
	defer func() {
		s.mu.Lock()
		if s.ln == ln {
			s.ln, s.cancel, s.conns, s.actualPort = nil, nil, nil, 0
		}
		s.mu.Unlock()
	}()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		current := s.ln == ln
		if current {
			if s.conns == nil {
				s.conns = map[net.Conn]struct{}{}
			}
			s.conns[conn] = struct{}{}
		}
		s.mu.Unlock()
		if !current {
			_ = conn.Close()
			return
		}
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
				_ = conn.Close()
			}()
			_ = s.serveConn(ctx, conn)
		}()
	}
}

const (
	mailMaxRecipients = 1000
	mailMaxMessage    = 25 << 20 // 25 MB
)

func (s *mailServer) serveConn(ctx context.Context, conn net.Conn) error {
	tc := textproto.NewConn(conn)
	defer tc.Close()
	_ = tc.PrintfLine("220 lair.local ESMTP ready")

	var from, to, inbox string
	recipients := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// One minute per command, including the body of DATA.
		_ = conn.SetDeadline(time.Now().Add(time.Minute))
		line, err := tc.ReadLine()
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		cmd, arg := line, ""
		if i := strings.IndexByte(line, ' '); i >= 0 {
			cmd, arg = line[:i], strings.TrimSpace(line[i+1:])
		}
		switch strings.ToUpper(cmd) {
		case "EHLO", "HELO":
			if arg == "" {
				_ = tc.PrintfLine("501 Syntax: %s hostname", strings.ToUpper(cmd))
				continue
			}
			if strings.EqualFold(cmd, "EHLO") {
				_ = tc.PrintfLine("250-lair.local")
				_ = tc.PrintfLine("250-AUTH PLAIN LOGIN")
				_ = tc.PrintfLine("250 SIZE 26214400")
			} else {
				_ = tc.PrintfLine("250 lair.local")
			}
		case "AUTH":
			user, ok := s.authenticate(tc, arg)
			if !ok {
				_ = tc.PrintfLine("535 Authentication failed")
				continue
			}
			inbox = mailInboxName(user)
			_ = tc.PrintfLine("235 2.7.0 Authentication successful")
		case "MAIL":
			if !strings.HasPrefix(strings.ToUpper(arg), "FROM:") {
				_ = tc.PrintfLine("501 Syntax: MAIL FROM:<address>")
				continue
			}
			from = mailAddress(arg[len("FROM:"):])
			if from == "" {
				_ = tc.PrintfLine("501 Invalid MAIL FROM")
			} else {
				_ = tc.PrintfLine("250 OK")
			}
		case "RCPT":
			if !strings.HasPrefix(strings.ToUpper(arg), "TO:") {
				_ = tc.PrintfLine("501 Syntax: RCPT TO:<address>")
				continue
			}
			addr := mailAddress(arg[len("TO:"):])
			if addr == "" {
				_ = tc.PrintfLine("501 Invalid RCPT TO")
				continue
			}
			if recipients >= mailMaxRecipients {
				_ = tc.PrintfLine("452 Too many recipients")
				continue
			}
			if to == "" {
				to = addr
			} else {
				to += "," + addr
			}
			recipients++
			_ = tc.PrintfLine("250 OK")
		case "DATA":
			if from == "" || to == "" {
				_ = tc.PrintfLine("503 Need MAIL FROM and RCPT TO first")
				continue
			}
			_ = tc.PrintfLine("354 End data with <CR><LF>.<CR><LF>")
			raw, err := io.ReadAll(io.LimitReader(tc.DotReader(), mailMaxMessage+1))
			if err != nil {
				return err
			}
			if len(raw) > mailMaxMessage {
				_ = tc.PrintfLine("552 Message too large (max 25 MB)")
				from, to, recipients = "", "", 0
				continue
			}
			msg, err := s.save(from, to, raw, inbox)
			if err != nil {
				_ = tc.PrintfLine("451 Cannot store message: %v", err)
				continue
			}
			_ = tc.PrintfLine("250 OK: queued as %s", msg.ID)
			from, to, recipients = "", "", 0
		case "RSET":
			from, to, recipients = "", "", 0
			_ = tc.PrintfLine("250 OK")
		case "NOOP":
			_ = tc.PrintfLine("250 OK")
		case "QUIT":
			_ = tc.PrintfLine("221 Bye")
			return nil
		default:
			_ = tc.PrintfLine("500 Command not recognized")
		}
	}
}

// authenticate accepts any credentials. The password is deliberately ignored;
// only the username selects the mailbox.
func (s *mailServer) authenticate(tc *textproto.Conn, arg string) (string, bool) {
	decode := func(line, prompt string) (string, bool) {
		value := strings.TrimSpace(line)
		if value == "" {
			_ = tc.PrintfLine("334 %s", prompt)
			line, err := tc.ReadLine()
			if err != nil {
				return "", false
			}
			value = strings.TrimSpace(line)
		}
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", false
		}
		return string(decoded), true
	}
	parts := strings.Fields(arg)
	if len(parts) == 0 {
		return "", false
	}
	if strings.EqualFold(parts[0], "LOGIN") {
		user, ok := decode("", "VXNlcm5hbWU6") // "Username:"
		if !ok {
			return "", false
		}
		if _, ok := decode("", "UGFzc3dvcmQ6"); !ok { // "Password:"
			return "", false
		}
		return user, true
	}
	if !strings.EqualFold(parts[0], "PLAIN") {
		return "", false
	}
	blob := ""
	if len(parts) > 1 {
		blob = parts[1]
	}
	decoded, ok := decode(blob, "")
	if !ok {
		return "", false
	}
	fields := strings.Split(string(decoded), "\x00")
	if len(fields) < 2 {
		return "", false
	}
	return fields[1], true
}

func mailAddress(arg string) string {
	arg = strings.TrimSpace(arg)
	if i, j := strings.IndexByte(arg, '<'), strings.IndexByte(arg, '>'); i >= 0 && j > i {
		arg = arg[i+1 : j]
	}
	if i := strings.IndexByte(arg, ' '); i >= 0 {
		arg = arg[:i]
	}
	return strings.Trim(arg, "<> ")
}

// mailInboxName turns an SMTP username into a safe folder name. Any
// credential is accepted; the username only selects the mailbox.
func mailInboxName(user string) string {
	user = strings.ToLower(strings.TrimSpace(user))
	var b strings.Builder
	for _, r := range user {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			if b.Len() > 0 {
				b.WriteRune('-')
			}
		}
	}
	name := strings.Trim(b.String(), "._-")
	name = strings.ReplaceAll(name, "--", "-")
	if name == "" {
		return "default"
	}
	return name
}

func (s *mailServer) inboxDir(name string) string {
	if name == "" || name == "default" {
		return s.inbox
	}
	return filepath.Join(s.dir, "inboxes", name)
}

func newMailID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("mail-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("mail-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// save parses and persists one message. Both the .eml source and the JSON
// index are written under the mailbox lock, so concurrent sessions cannot
// interleave index updates.
func (s *mailServer) save(from, to string, raw []byte, inbox string) (MailMessage, error) {
	id := newMailID()
	msg := parseMailMessage(id, from, to, raw)
	s.boxMu.Lock()
	defer s.boxMu.Unlock()
	dir := s.inboxDir(inbox)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return MailMessage{}, fmt.Errorf("mail inbox: %w", err)
	}
	// Write to a temporary file and rename: readers never see half a message.
	tmp, err := os.CreateTemp(dir, ".incoming-*")
	if err != nil {
		return MailMessage{}, fmt.Errorf("create mail: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return MailMessage{}, fmt.Errorf("write message: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return MailMessage{}, err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return MailMessage{}, err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, id+".eml")); err != nil {
		return MailMessage{}, fmt.Errorf("store message: %w", err)
	}
	idx, err := s.readIndexLocked(dir)
	if err != nil {
		return MailMessage{}, err
	}
	idx.Messages = append(idx.Messages, msg)
	if err := s.writeIndexLocked(idx, dir); err != nil {
		return MailMessage{}, err
	}
	return msg, nil
}

func parseMailMessage(id, from, to string, raw []byte) MailMessage {
	msg := MailMessage{
		ID: id, From: from, To: to,
		Date: time.Now().UTC().Format(time.RFC3339),
	}
	r := bufio.NewReader(bytes.NewReader(raw))
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		msg.Body = string(raw)
		return msg
	}
	if v := header.Get("From"); v != "" {
		msg.From = mailAddress(v)
	}
	if v := header.Get("To"); v != "" {
		msg.To = v
	}
	if v := header.Get("Subject"); v != "" {
		decoded, err := new(mime.WordDecoder).DecodeHeader(v)
		if err == nil {
			msg.Subject = decoded
		} else {
			msg.Subject = v
		}
	}
	if v := header.Get("Date"); v != "" {
		msg.Date = v
	}
	body, _ := io.ReadAll(r)
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err == nil && strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		var parts []string
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if name := part.FileName(); name != "" {
				msg.Attachments = append(msg.Attachments, MailAttachment{
					Name: name, ContentType: part.Header.Get("Content-Type"), Size: len(data),
				})
			} else {
				parts = append(parts, string(data))
			}
		}
		msg.Body = strings.TrimRight(strings.Join(parts, "\n"), "\n")
	} else {
		msg.Body = strings.TrimRight(string(body), "\r\n")
	}
	return msg
}

func (s *mailServer) readIndexLocked(dir string) (mailIndex, error) {
	data, err := os.ReadFile(filepath.Join(dir, mailIndexFile))
	if os.IsNotExist(err) {
		return mailIndex{Messages: []MailMessage{}}, nil
	}
	if err != nil {
		return mailIndex{}, fmt.Errorf("read mail index: %w", err)
	}
	var idx mailIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return mailIndex{}, fmt.Errorf("parse mail index: %w", err)
	}
	if idx.Messages == nil {
		idx.Messages = []MailMessage{}
	}
	return idx, nil
}

func (s *mailServer) writeIndexLocked(idx mailIndex, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mail inbox: %w", err)
	}
	buf, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mail index: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".index-*")
	if err != nil {
		return fmt.Errorf("create mail index: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return fmt.Errorf("write mail index: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod mail index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close mail index: %w", err)
	}
	if err := os.Rename(name, filepath.Join(dir, mailIndexFile)); err != nil {
		return fmt.Errorf("rename mail index: %w", err)
	}
	return nil
}

func (s *mailServer) messages(inbox string) ([]MailMessage, error) {
	s.boxMu.Lock()
	defer s.boxMu.Unlock()
	idx, err := s.readIndexLocked(s.inboxDir(inbox))
	if err != nil {
		return nil, err
	}
	return append([]MailMessage(nil), idx.Messages...), nil
}

func (s *mailServer) message(id, inbox string) (*MailMessage, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return nil, fmt.Errorf("invalid message id")
	}
	s.boxMu.Lock()
	defer s.boxMu.Unlock()
	idx, err := s.readIndexLocked(s.inboxDir(inbox))
	if err != nil {
		return nil, err
	}
	for i := range idx.Messages {
		if idx.Messages[i].ID == id {
			msg := idx.Messages[i]
			return &msg, nil
		}
	}
	return nil, fmt.Errorf("no mail message %q", id)
}

// raw returns the original .eml source of one message.
func (s *mailServer) raw(id, inbox string) ([]byte, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return nil, fmt.Errorf("invalid message id")
	}
	s.boxMu.Lock()
	defer s.boxMu.Unlock()
	return os.ReadFile(filepath.Join(s.inboxDir(inbox), id+".eml"))
}

// clear removes only the captured mail (the tray), never the listener or any
// other Lair state.
func (s *mailServer) clear(inbox string) error {
	s.boxMu.Lock()
	defer s.boxMu.Unlock()
	dir := s.inboxDir(inbox)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return s.writeIndexLocked(mailIndex{Messages: []MailMessage{}}, dir)
		}
		return fmt.Errorf("read mail inbox: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".eml") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("remove mail %s: %w", entry.Name(), err)
		}
	}
	return s.writeIndexLocked(mailIndex{Messages: []MailMessage{}}, dir)
}

// mailEnv is the Laravel block always returned by `status`.
func mailEnv(port int) string {
	if port == 0 {
		port = config.DefaultMailPort
	}
	return fmt.Sprintf("MAIL_MAILER=smtp\nMAIL_HOST=127.0.0.1\nMAIL_PORT=%d\nMAIL_USERNAME=lair\nMAIL_PASSWORD=null\nMAIL_SCHEME=null\n", port)
}

// MailControl implements the `mail` daemon action.
func (e *Engine) MailControl(ctx context.Context, req MailRequest) (MailResult, error) {
	var out MailResult
	switch req.Op {
	case "", "status", "list", "message", "raw", "clear", "start", "stop":
	default:
		return out, fmt.Errorf("unknown mail op %q", req.Op)
	}

	c, err := e.Config.View()
	if err != nil {
		return out, fmt.Errorf("mail config: %w", err)
	}
	port := c.Mail.Port
	if port <= 0 {
		port = config.DefaultMailPort
	}
	e.mailMu.Lock()
	if e.mail == nil {
		e.mail = newMailServer(filepath.Join(e.Paths.Root, "mail"), port)
	}
	server := e.mail
	e.mailMu.Unlock()

	switch req.Op {
	case "start":
		desired := req.Port
		if desired == -1 {
			desired = 0 // kernel-assigned ephemeral port
		} else if desired == 0 {
			desired = port
		}
		if !server.Running() {
			server.setPort(desired)
		}
		bound, err := server.Start(ctx)
		if err != nil {
			return out, fmt.Errorf("start mail: %w", err)
		}
		out.Output = fmt.Sprintf("SMTP capturando en 127.0.0.1:%d", bound)
	case "stop":
		if err := server.Stop(); err != nil {
			return out, fmt.Errorf("stop mail: %w", err)
		}
		out.Output = "SMTP detenido"
	case "clear":
		if !req.Confirm {
			return MailResult{}, fmt.Errorf("vaciar la bandeja requiere confirmación (confirm: true)")
		}
		if err := server.clear(req.Inbox); err != nil {
			return out, fmt.Errorf("clear mail: %w", err)
		}
		out.Output = "Bandeja vaciada"
	case "list":
		messages, err := server.messages(req.Inbox)
		if err != nil {
			return out, fmt.Errorf("list mail: %w", err)
		}
		out.Messages = messages
		out.Count = len(messages)
	case "message":
		msg, err := server.message(req.ID, req.Inbox)
		if err != nil {
			return out, fmt.Errorf("read mail: %w", err)
		}
		out.Message = msg
	case "raw":
		raw, err := server.raw(req.ID, req.Inbox)
		if err != nil {
			return out, fmt.Errorf("read raw mail: %w", err)
		}
		out.Raw = string(raw)
	}

	messages, err := server.messages(req.Inbox)
	if err != nil {
		return out, fmt.Errorf("count mail: %w", err)
	}
	out.Running, out.Port, out.Count = server.Running(), server.Port(), len(messages)
	out.Env = mailEnv(out.Port)
	return out, nil
}
