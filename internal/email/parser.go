package email

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
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

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("read message: %w", err)
	}

	res := ParsedEmail{
		Subject:   msg.Header.Get("Subject"),
		From:      msg.Header.Get("From"),
		MessageID: msg.Header.Get("Message-Id"),
	}
	if to := msg.Header.Get("To"); to != "" {
		res.To = []string{to}
	}

	ct := msg.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(ct)

	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("read body: %w", err)
	}

	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		boundary := params["boundary"]
		if boundary == "" {
			res.TextBody = string(bodyBytes)
			return res, nil
		}

		mr := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("multipart read: %w", err)
			}

			pct := part.Header.Get("Content-Type")
			pmt, _, _ := mime.ParseMediaType(pct)
			pb, _ := io.ReadAll(part)

			if strings.HasPrefix(pmt, "text/plain") && res.TextBody == "" {
				res.TextBody = string(pb)
				continue
			}
			if strings.HasPrefix(pmt, "text/html") && res.HTMLBody == "" {
				res.HTMLBody = string(pb)
				continue
			}

			if cd := strings.ToLower(part.Header.Get("Content-Disposition")); strings.Contains(cd, "attachment") {
				res.Attachments = append(res.Attachments, Attachment{
					Filename:    part.FileName(),
					ContentType: pmt,
					SizeBytes:   int64(len(pb)),
					Data:        pb,
				})
			}
		}

	default:
		if strings.HasPrefix(mediaType, "text/html") {
			res.HTMLBody = string(bodyBytes)
		} else {
			res.TextBody = string(bodyBytes)
		}
	}

	return res, nil
}
