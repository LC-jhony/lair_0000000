package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func smtpConnect(t *testing.T, port int) (*textproto.Conn, net.Conn) {
	t.Helper()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial smtp: %v", err)
	}
	tc := textproto.NewConn(conn)
	if line, err := tc.ReadLine(); err != nil || !strings.HasPrefix(line, "220") {
		t.Fatalf("greeting = %q, %v", line, err)
	}
	return tc, conn
}

func smtpExpect(t *testing.T, tc *textproto.Conn, prefix string) string {
	t.Helper()
	line, err := tc.ReadLine()
	if err != nil || !strings.HasPrefix(line, prefix) {
		t.Fatalf("wanted %q, got %q (%v)", prefix, line, err)
	}
	return line
}

func smtpSend(t *testing.T, port int, subject, body string) string {
	t.Helper()
	tc, conn := smtpConnect(t, port)
	defer conn.Close()
	defer tc.Close()
	if err := tc.PrintfLine("EHLO localhost"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		line := smtpExpect(t, tc, "250")
		if strings.HasPrefix(line, "250 ") {
			break
		}
	}
	if err := tc.PrintfLine("MAIL FROM:<remitente@example.com>"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("RCPT TO:<destino@example.test>"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("DATA"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "354")
	dw := tc.DotWriter()
	fmt.Fprintf(dw, "From: Remitente <remitente@example.com>\r\nTo: destino@example.test\r\nSubject: %s\r\n\r\n%s\r\n", subject, body)
	if err := dw.Close(); err != nil {
		t.Fatal(err)
	}
	line := smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("QUIT"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "221")
	return strings.TrimSpace(strings.TrimPrefix(line, "250 OK: queued as "))
}

func TestMailInboxFromAuthUser(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	st, err := e.MailControl(ctx, MailRequest{Op: "start", Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.MailControl(ctx, MailRequest{Op: "stop"})

	tc, conn := smtpConnect(t, st.Port)
	defer conn.Close()
	defer tc.Close()
	if err := tc.PrintfLine("EHLO localhost"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		line := smtpExpect(t, tc, "250")
		if strings.HasPrefix(line, "250 ") {
			break
		}
	}
	blob := base64.StdEncoding.EncodeToString([]byte("\x00Mi App\x00ignored"))
	if err := tc.PrintfLine("AUTH PLAIN %s", blob); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "235")
	if err := tc.PrintfLine("MAIL FROM:<a@example.com>"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("RCPT TO:<b@example.test>"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("DATA"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "354")
	dw := tc.DotWriter()
	fmt.Fprint(dw, "Subject: Bandeja\r\n\r\nHola\r\n")
	if err := dw.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("DATA response: %s", smtpExpect(t, tc, "250"))

	listed, err := e.MailControl(ctx, MailRequest{Op: "list", Inbox: "mi-app"})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || listed.Messages[0].Subject != "Bandeja" {
		t.Fatalf("inbox = %+v", listed)
	}
	other, err := e.MailControl(ctx, MailRequest{Op: "list", Inbox: "otra"})
	if err != nil || other.Count != 0 {
		t.Fatalf("other inbox = %+v, %v", other, err)
	}
}

func TestMailSMTPLifecycle(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	ctx := context.Background()

	st, err := e.MailControl(ctx, MailRequest{Op: "start", Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.Port <= 0 {
		t.Fatalf("start result = %+v", st)
	}
	defer e.MailControl(ctx, MailRequest{Op: "stop"})

	// NOOP/RSET are answered without an envelope.
	tc, conn := smtpConnect(t, st.Port)
	if err := tc.PrintfLine("NOOP"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("RSET"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "250")
	if err := tc.PrintfLine("DATA"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "503")
	if err := tc.PrintfLine("QUIT"); err != nil {
		t.Fatal(err)
	}
	smtpExpect(t, tc, "221")
	tc.Close()
	conn.Close()

	id := smtpSend(t, st.Port, "Hola", "Cuerpo del mensaje")

	listed, err := e.MailControl(ctx, MailRequest{Op: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || len(listed.Messages) != 1 {
		t.Fatalf("list = %+v", listed)
	}
	msg := listed.Messages[0]
	if msg.ID != id || msg.Subject != "Hola" || msg.Body != "Cuerpo del mensaje" {
		t.Fatalf("message = %+v", msg)
	}
	if msg.From != "remitente@example.com" || msg.To != "destino@example.test" {
		t.Fatalf("envelope = %+v", msg)
	}

	single, err := e.MailControl(ctx, MailRequest{Op: "message", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if single.Message == nil || single.Message.ID != id {
		t.Fatalf("message result = %+v", single)
	}
	if _, err := e.MailControl(ctx, MailRequest{Op: "message", ID: "../escape"}); err == nil {
		t.Fatal("message traversal accepted")
	}

	inbox := filepath.Join(e.Paths.Root, "mail", "inbox")
	if _, err := os.Stat(filepath.Join(inbox, id+".eml")); err != nil {
		t.Fatalf("eml not persisted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "index.json")); err != nil {
		t.Fatalf("index not persisted: %v", err)
	}
	idxInfo, err := os.Stat(filepath.Join(inbox, "index.json"))
	if err != nil || idxInfo.Mode().Perm() != 0o600 {
		t.Fatalf("index mode = %v, %v", idxInfo, err)
	}

	// Status always carries the full MAIL_* block.
	status, err := e.MailControl(ctx, MailRequest{Op: "status"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MAIL_MAILER", "MAIL_HOST", "MAIL_PORT", "MAIL_USERNAME", "MAIL_PASSWORD", "MAIL_SCHEME"} {
		if !strings.Contains(status.Env, key+"=") {
			t.Fatalf("env missing %s: %q", key, status.Env)
		}
	}
	if !strings.Contains(status.Env, fmt.Sprintf("MAIL_PORT=%d", st.Port)) {
		t.Fatalf("wrong port in env: %q", status.Env)
	}

	cleared, err := e.MailControl(ctx, MailRequest{Op: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Count != 0 {
		t.Fatalf("clear count = %d", cleared.Count)
	}
	if _, err := os.Stat(filepath.Join(inbox, id+".eml")); !os.IsNotExist(err) {
		t.Fatalf("eml survived clear: %v", err)
	}
	// clear keeps the tray directory and the agent's other state intact.
	if _, err := os.Stat(inbox); err != nil {
		t.Fatalf("inbox removed by clear: %v", err)
	}
}

func TestMailInboxConcurrency(t *testing.T) {
	e, srv := testEnv(t)
	defer srv.Close()
	st, err := e.MailControl(context.Background(), MailRequest{Op: "start", Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.MailControl(context.Background(), MailRequest{Op: "stop"})

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tc, conn, err := func() (*textproto.Conn, net.Conn, error) {
				conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", st.Port))
				if err != nil {
					return nil, nil, err
				}
				return textproto.NewConn(conn), conn, nil
			}()
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()
			defer tc.Close()
			if line, err := tc.ReadLine(); err != nil || !strings.HasPrefix(line, "220") {
				errs <- fmt.Errorf("greeting %q: %v", line, err)
				return
			}
			_ = tc.PrintfLine("HELO localhost")
			if _, err := tc.ReadLine(); err != nil {
				errs <- err
				return
			}
			_ = tc.PrintfLine("MAIL FROM:<a@example.test>")
			if _, err := tc.ReadLine(); err != nil {
				errs <- err
				return
			}
			_ = tc.PrintfLine("RCPT TO:<b@example.test>")
			if _, err := tc.ReadLine(); err != nil {
				errs <- err
				return
			}
			_ = tc.PrintfLine("DATA")
			if _, err := tc.ReadLine(); err != nil {
				errs <- err
				return
			}
			dw := tc.DotWriter()
			fmt.Fprintf(dw, "Subject: msg-%d\r\n\r\nbody-%d\r\n", i, i)
			if err := dw.Close(); err != nil {
				errs <- err
				return
			}
			if _, err := tc.ReadLine(); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent session: %v", err)
	}

	out, err := e.MailControl(context.Background(), MailRequest{Op: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != n || len(out.Messages) != n {
		t.Fatalf("concurrent inbox = %d messages (%d listed), want %d", out.Count, len(out.Messages), n)
	}
	seen := map[string]bool{}
	for _, msg := range out.Messages {
		if seen[msg.ID] {
			t.Fatalf("duplicate id %s", msg.ID)
		}
		seen[msg.ID] = true
	}
}
