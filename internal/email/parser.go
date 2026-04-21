package email

import (
	"bytes"
	"fmt"
	"mime"
	"strings"

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

	seen := make(map[string]struct{})
	appendPartsAsAttachments(&res.Attachments, env.Attachments, true, seen)
	appendPartsAsAttachments(&res.Attachments, env.Inlines, false, seen)
	appendPartsAsAttachments(&res.Attachments, env.OtherParts, false, seen)

	return res, nil
}

func appendPartsAsAttachments(dst *[]Attachment, parts []*enmime.Part, includeText bool, seen map[string]struct{}) {
	for _, part := range parts {
		if !shouldExposeAsAttachment(part, includeText) {
			continue
		}
		if _, ok := seen[part.PartID]; ok {
			continue
		}
		seen[part.PartID] = struct{}{}

		*dst = append(*dst, Attachment{
			Filename:    attachmentFileName(part),
			ContentType: part.ContentType,
			SizeBytes:   int64(len(part.Content)),
			Data:        part.Content,
		})
	}
}

func shouldExposeAsAttachment(part *enmime.Part, includeText bool) bool {
	if part == nil || len(part.Content) == 0 {
		return false
	}

	contentType := strings.ToLower(strings.TrimSpace(part.ContentType))
	if strings.HasPrefix(contentType, "multipart/") {
		return false
	}

	if includeText {
		return true
	}

	if part.TextContent() {
		return false
	}

	return strings.HasPrefix(contentType, "image/") ||
		strings.HasPrefix(contentType, "audio/") ||
		strings.HasPrefix(contentType, "video/") ||
		strings.TrimSpace(part.FileName) != "" ||
		strings.TrimSpace(part.ContentID) != ""
}

func attachmentFileName(part *enmime.Part) string {
	if name := strings.TrimSpace(part.FileName); name != "" {
		return name
	}

	base := sanitizeAttachmentToken(part.ContentID)
	if base == "" {
		base = "part"
		if id := sanitizeAttachmentToken(part.PartID); id != "" {
			base += "-" + id
		}
	}

	ext := ".bin"
	if exts, err := mime.ExtensionsByType(strings.TrimSpace(part.ContentType)); err == nil && len(exts) > 0 {
		ext = exts[0]
	}

	return base + ext
}

func sanitizeAttachmentToken(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "<>")
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
