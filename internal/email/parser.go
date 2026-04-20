package email

import (
	"bytes"
	"fmt"

	"github.com/jhillyerd/enmime"
)

type Parser interface {
	Parse(raw []byte) (ParsedEmail, error)
}

type parser struct {
	maxBytes int64
}

func NewParser(maxBytes int64) Parser {
	return &parser{maxBytes: maxBytes}
}

func (p *parser) Parse(raw []byte) (ParsedEmail, error) {
	if int64(len(raw)) > p.maxBytes {
		return ParsedEmail{}, fmt.Errorf("message too large: %d > %d", len(raw), p.maxBytes)
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("read message: %w", err)
	}

	res := ParsedEmail{
		Subject:   env.GetHeader("Subject"),
		From:      env.GetHeader("From"),
		MessageID: env.GetHeader("Message-Id"),
		TextBody:  env.Text,
		HTMLBody:  env.HTML,
	}
	if to := env.GetHeader("To"); to != "" {
		res.To = []string{to}
	}

	for _, a := range env.Attachments {
		res.Attachments = append(res.Attachments, Attachment{
			Filename:    a.FileName,
			ContentType: a.ContentType,
			SizeBytes:   int64(len(a.Content)),
			Data:        a.Content,
		})
	}

	return res, nil
}
