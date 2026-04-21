package smtp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"smtp-to-max-relay/internal/relay"
)

type Server struct {
	addr     string
	domain   string
	maxBytes int64
	relaySvc *relay.Service
}

func NewServer(addr, domain string, maxBytes int64, relaySvc *relay.Service) *Server {
	return &Server{addr: addr, domain: domain, maxBytes: maxBytes, relaySvc: relaySvc}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()
	log.Printf("SMTP listening on %s", s.addr)

	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return fmt.Errorf("accept: %w", err)
		}

		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			defer c.Close()
			s.handleConn(ctx, c)
		}(conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	writeLine(w, "220 smtp-to-max-relay ESMTP")

	var rcpts []string
	authenticated := false

	for {
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
			return
		}
		line, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				writeLine(w, "421 read error")
			}
			return
		}
		cmd := strings.TrimSpace(line)
		ucmd := strings.ToUpper(cmd)

		switch {
		case strings.HasPrefix(ucmd, "EHLO"):
			writeLine(w, "250-Hello")
			writeLine(w, "250-AUTH PLAIN")
			writeLine(w, "250 SIZE 15728640")

		case strings.HasPrefix(ucmd, "HELO"):
			writeLine(w, "250 Hello")

		case strings.HasPrefix(ucmd, "AUTH PLAIN"):
			// relay mode: accept any credentials
			authenticated = true
			writeLine(w, "235 2.7.0 Authentication successful")

		case strings.HasPrefix(ucmd, "MAIL FROM:"):
			// auth is optional in relay mode; accept both authenticated and anonymous flows
			_ = authenticated
			rcpts = rcpts[:0]
			writeLine(w, "250 OK")

		case strings.HasPrefix(ucmd, "RCPT TO:"):
			addr := extractSMTPPath(cmd[len("RCPT TO:"):])
			if addr == "" {
				writeLine(w, "501 bad recipient")
				continue
			}
			if !isAllowedRecipient(addr, s.domain) {
				writeLine(w, "550 recipient domain is not allowed")
				continue
			}
			if s.relaySvc != nil && s.relaySvc.Recipients != nil {
				if _, err := s.relaySvc.Recipients.Parse(addr); err != nil {
					writeLine(w, "550 recipient is invalid")
					continue
				}
			}
			rcpts = append(rcpts, addr)
			writeLine(w, "250 OK")

		case ucmd == "DATA":
			if len(rcpts) == 0 {
				writeLine(w, "503 need RCPT TO first")
				continue
			}
			writeLine(w, "354 End data with <CR><LF>.<CR><LF>")
			raw, err := readData(r, s.maxBytes)
			if err != nil {
				writeLine(w, "552 message exceeds fixed maximum message size")
				continue
			}
			failed := false
			for _, rcpt := range rcpts {
				if err := s.relaySvc.Relay(ctx, rcpt, raw); err != nil {
					log.Printf("relay error for rcpt=%s: %v", rcpt, err)
					failed = true
					break
				}
			}
			if failed {
				writeLine(w, "451 relay failure")
				continue
			}
			writeLine(w, "250 OK")

		case ucmd == "RSET":
			rcpts = rcpts[:0]
			writeLine(w, "250 OK")

		case ucmd == "NOOP":
			writeLine(w, "250 OK")

		case ucmd == "QUIT":
			writeLine(w, "221 Bye")
			return

		default:
			writeLine(w, "502 command not implemented")
		}
	}
}

func writeLine(w *bufio.Writer, line string) {
	_, _ = w.WriteString(line + "\r\n")
	_ = w.Flush()
}

func extractSMTPPath(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "<>")
	return strings.TrimSpace(v)
}

func readData(r *bufio.Reader, maxBytes int64) ([]byte, error) {
	var buf bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == ".\r\n" || line == ".\n" {
			break
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}
		if int64(buf.Len()+len(line)) > maxBytes {
			return nil, fmt.Errorf("too large")
		}
		buf.WriteString(line)
	}
	return buf.Bytes(), nil
}

func isAllowedRecipient(addr, allowedDomain string) bool {
	a := strings.TrimSpace(strings.ToLower(addr))
	a = strings.Trim(a, "<>")
	return strings.HasSuffix(a, "@"+strings.ToLower(strings.TrimSpace(allowedDomain)))
}
