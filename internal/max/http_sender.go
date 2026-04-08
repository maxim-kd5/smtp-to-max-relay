package max

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"smtp-to-max-relay/internal/email"
)

type HTTPSender struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPSender(baseURL, token string, timeout time.Duration) (*HTTPSender, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("MAX_API_BASE_URL is required for http sender")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("MAX_BOT_TOKEN is required for http sender")
	}
	return &HTTPSender{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

type sendTextRequest struct {
	ChatID   string `json:"chat_id"`
	ThreadID string `json:"thread_id,omitempty"`
	Text     string `json:"text"`
	Silent   bool   `json:"silent"`
}

func (s *HTTPSender) SendText(ctx context.Context, chatID, threadID, text string, silent bool) error {
	payload := sendTextRequest{ChatID: chatID, ThreadID: threadID, Text: text, Silent: silent}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal text payload: %w", err)
	}

	u, err := s.endpointURL("messages/send")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send text request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send text failed with status %d", resp.StatusCode)
	}
	return nil
}

func (s *HTTPSender) SendFile(ctx context.Context, chatID, threadID string, a email.Attachment, silent bool) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	_ = w.WriteField("chat_id", chatID)
	if threadID != "" {
		_ = w.WriteField("thread_id", threadID)
	}
	_ = w.WriteField("silent", fmt.Sprintf("%t", silent))

	fileName := a.Filename
	if fileName == "" {
		fileName = "attachment.bin"
	}

	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("create multipart file: %w", err)
	}
	if _, err := part.Write(a.Data); err != nil {
		return fmt.Errorf("write multipart file: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	u, err := s.endpointURL("messages/sendFile")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &b)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send file request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send file failed with status %d", resp.StatusCode)
	}
	return nil
}

func (s *HTTPSender) endpointURL(p string) (string, error) {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid MAX_API_BASE_URL: %w", err)
	}
	u.Path = path.Join(u.Path, p)
	return u.String(), nil
}
