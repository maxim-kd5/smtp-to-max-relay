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
	addr         string
	domain       string
	maxBytes     int64
	relaySvc     *relay.Service
	writeTimeout time.Duration
}

func NewServer(addr, domain string, maxBytes int64, relaySvc *relay.Service) *Server {
	return &Server{
		addr:         addr,
		domain:       domain,
		maxBytes:     maxBytes,
		relaySvc:     relaySvc,
		writeTimeout: 30 * time.Second,
	}
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

	if !writeLine(conn, w, "220 smtp-to-max-relay ESMTP", s.writeTimeout) {
		return
	}

	var rcpts []string
	authenticated := false

	for {
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
			return
		}
		line, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				_ = writeLine(conn, w, "421 read error", s.writeTimeout)
			}
			return
		}
		cmd := strings.TrimSpace(line)
		ucmd := strings.ToUpper(cmd)

		switch {
		case strings.HasPrefix(ucmd, "EHLO"):
			if !writeLine(conn, w, "250-Hello", s.writeTimeout) {
				return
			}
			if !writeLine(conn, w, "250-AUTH PLAIN", s.writeTimeout) {
				return
			}
			if !writeLine(conn, w, fmt.Sprintf("250 SIZE %d", s.maxBytes), s.writeTimeout) {
				return
			}

		case strings.HasPrefix(ucmd, "HELO"):
			if !writeLine(conn, w, "250 Hello", s.writeTimeout) {
				return
			}

		case strings.HasPrefix(ucmd, "AUTH PLAIN"):
			// relay mode: accept any credentials
			authenticated = true
			if !writeLine(conn, w, "235 2.7.0 Authentication successful", s.writeTimeout) {
				return
			}

		case strings.HasPrefix(ucmd, "MAIL FROM:"):
			// auth is optional in relay mode; accept both authenticated and anonymous flows
			_ = authenticated
			rcpts = rcpts[:0]
			if !writeLine(conn, w, "250 OK", s.writeTimeout) {
				return
			}

		case strings.HasPrefix(ucmd, "RCPT TO:"):
			addr := extractSMTPPath(cmd[len("RCPT TO:"):])
			if addr == "" {
				if !writeLine(conn, w, "501 bad recipient", s.writeTimeout) {
					return
				}
				continue
			}
			if !isAllowedRecipient(addr, s.domain) {
				if !writeLine(conn, w, "550 recipient domain is not allowed", s.writeTimeout) {
					return
				}
				continue
			}
			if s.relaySvc != nil && s.relaySvc.Recipients != nil {
				if _, err := s.relaySvc.Recipients.Parse(addr); err != nil {
					if !writeLine(conn, w, "550 recipient is invalid", s.writeTimeout) {
						return
					}
					continue
				}
			}
			rcpts = append(rcpts, addr)
			if !writeLine(conn, w, "250 OK", s.writeTimeout) {
				return
			}

		case ucmd == "DATA":
			if len(rcpts) == 0 {
				if !writeLine(conn, w, "503 need RCPT TO first", s.writeTimeout) {
					return
				}
				continue
			}
			if !writeLine(conn, w, "354 End data with <CR><LF>.<CR><LF>", s.writeTimeout) {
				return
			}
			raw, err := readData(r, s.maxBytes)
			if err != nil {
				if !writeLine(conn, w, "552 message exceeds fixed maximum message size", s.writeTimeout) {
					return
				}
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
				if !writeLine(conn, w, "451 relay failure", s.writeTimeout) {
					return
				}
				continue
			}
			if !writeLine(conn, w, "250 OK", s.writeTimeout) {
				return
			}

		case ucmd == "RSET":
			rcpts = rcpts[:0]
			if !writeLine(conn, w, "250 OK", s.writeTimeout) {
				return
			}

		case ucmd == "NOOP":
			if !writeLine(conn, w, "250 OK", s.writeTimeout) {
				return
			}

		case ucmd == "QUIT":
			_ = writeLine(conn, w, "221 Bye", s.writeTimeout)
			return

		default:
			if !writeLine(conn, w, "502 command not implemented", s.writeTimeout) {
				return
			}
		}
	}
}

func writeLine(conn net.Conn, w *bufio.Writer, line string, timeout time.Duration) bool {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return false
	}
	if _, err := w.WriteString(line + "\r\n"); err != nil {
		return false
	}
	return w.Flush() == nil
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
