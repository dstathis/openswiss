package email

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// runFakeSMTP starts a minimal SMTP server on a random localhost port that
// accepts a single client and echoes the canned responses needed for
// smtp.SendMail to complete (no auth, no STARTTLS). It records the message
// body and signals on a channel when the conversation finishes.
func runFakeSMTP(t *testing.T) (addrHost, addrPort string, body <-chan string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())

	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		w := bufio.NewWriter(conn)
		r := bufio.NewReader(conn)
		w.WriteString("220 fake.local ESMTP\r\n")
		w.Flush()
		var collected strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				break
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if inData {
				if trimmed == "." {
					w.WriteString("250 2.0.0 Ok\r\n")
					w.Flush()
					inData = false
					out <- collected.String()
					continue
				}
				collected.WriteString(line)
				continue
			}
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				// Advertise no extensions so smtp.SendMail does not try STARTTLS.
				w.WriteString("250 fake.local\r\n")
			case strings.HasPrefix(upper, "MAIL FROM"):
				w.WriteString("250 2.1.0 OK\r\n")
			case strings.HasPrefix(upper, "RCPT TO"):
				w.WriteString("250 2.1.5 OK\r\n")
			case upper == "DATA":
				w.WriteString("354 End data with <CR><LF>.<CR><LF>\r\n")
				inData = true
			case upper == "QUIT":
				w.WriteString("221 Bye\r\n")
				w.Flush()
				return
			default:
				w.WriteString("250 OK\r\n")
			}
			w.Flush()
		}
	}()

	return host, port, out, func() { ln.Close() }
}

func TestSender_SendSTARTTLS_NoAuth(t *testing.T) {
	host, port, body, stop := runFakeSMTP(t)
	defer stop()

	s := &Sender{Config: Config{
		Host: host,
		Port: port,
		From: "noreply@example.com",
	}}
	err := s.send("user@example.com", "Hi", "Hello world")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got := <-body
	if !strings.Contains(got, "Subject: Hi") {
		t.Errorf("missing Subject in body: %q", got)
	}
	if !strings.Contains(got, "To: user@example.com") {
		t.Errorf("missing To in body: %q", got)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("missing body content: %q", got)
	}
}

func TestSender_SendPasswordReset_RoutesThrough(t *testing.T) {
	host, port, body, stop := runFakeSMTP(t)
	defer stop()

	s := &Sender{Config: Config{
		Host: host,
		Port: port,
		From: "noreply@example.com",
	}}
	err := s.SendPasswordReset("user@example.com", "https://example.com/reset?token=abc")
	if err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	got := <-body
	if !strings.Contains(got, "Password Reset") {
		t.Errorf("expected reset subject in body, got %q", got)
	}
	if !strings.Contains(got, "https://example.com/reset?token=abc") {
		t.Errorf("expected reset URL in body, got %q", got)
	}
}

func TestSender_BuildMessage_Format(t *testing.T) {
	s := &Sender{Config: Config{From: "n@example.com"}}
	msg := s.buildMessage("u@example.com", "S", "B")
	str := string(msg)
	if !strings.HasPrefix(str, "From: n@example.com\r\n") {
		t.Errorf("expected From first, got %q", str)
	}
	// Body separator: blank \r\n line
	if !strings.Contains(str, "\r\n\r\nB") {
		t.Errorf("expected blank line before body, got %q", str)
	}
	if !strings.Contains(str, "MIME-Version: 1.0") {
		t.Error("expected MIME-Version header")
	}
	if !strings.Contains(str, "Content-Type: text/plain; charset=\"utf-8\"") {
		t.Error("expected Content-Type header")
	}
}
