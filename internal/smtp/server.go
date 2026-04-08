package smtp

import (
	"context"
	"fmt"
	"io"
	"log"

	gosmtp "github.com/emersion/go-smtp"

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
	be := &backend{domain: s.domain, maxBytes: s.maxBytes, relaySvc: s.relaySvc}

	srv := gosmtp.NewServer(be)
	srv.Addr = s.addr
	srv.Domain = s.domain
	// Relay mode: SMTP auth is optional.
	// If client sends AUTH PLAIN, any login/password is accepted.
	srv.AllowInsecureAuth = true
	srv.MaxMessageBytes = s.maxBytes
	srv.MaxRecipients = 50

	errCh := make(chan error, 1)
	go func() {
		log.Printf("SMTP listening on %s", s.addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = srv.Close()
		return nil
	case err := <-errCh:
		if err == nil || err == gosmtp.ErrServerClosed {
			return nil
		}
		return err
	}
}

type backend struct {
	domain   string
	maxBytes int64
	relaySvc *relay.Service
}

func (b *backend) NewSession(_ *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{backend: b}, nil
}

type session struct {
	backend *backend
	from    string
	rcpt    []string
}

// Accept any credentials in relay mode (AUTH is optional).
func (s *session) AuthPlain(username, password string) error {
	_ = username
	_ = password
	return nil
}

func (s *session) Mail(from string, _ *gosmtp.MailOptions) error {
	s.from = from
	s.rcpt = s.rcpt[:0]
	return nil
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(r, s.backend.maxBytes+1))
	if err != nil {
		return fmt.Errorf("read DATA: %w", err)
	}
	if int64(len(raw)) > s.backend.maxBytes {
		return fmt.Errorf("message exceeds max size")
	}

	ctx := context.Background()
	for _, rcpt := range s.rcpt {
		if err := s.backend.relaySvc.Relay(ctx, rcpt, raw); err != nil {
			return fmt.Errorf("relay to %s: %w", rcpt, err)
		}
	}
	return nil
}

func (s *session) Reset() {}

func (s *session) Logout() error { return nil }
