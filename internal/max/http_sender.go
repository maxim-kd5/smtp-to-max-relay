package max

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
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
	Text   string `json:"text"`
	Notify bool   `json:"notify"`
}

type Chat struct {
	ID    string
	Title string
}

type Subscription struct {
	URL string
}

type Message struct {
	ID   string
	Text string
}

func (s *HTTPSender) ListChats(ctx context.Context) ([]Chat, error) {
	chats, err := s.listChatsPaginated(ctx)
	if err == nil {
		return chats, nil
	}

	paths := []string{
		"dialogs",
		"conversations",
		"me/chats",
		"bot/chats",
	}
	lastErr := err
	for _, p := range paths {
		chats, err := s.listChatsByPath(ctx, p)
		if err == nil {
			return chats, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no chat list endpoints configured")
	}
	return nil, lastErr
}

func (s *HTTPSender) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	u, err := s.endpointURL("subscriptions")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create list subscriptions request: %w", err)
	}
	req.Header.Set("Authorization", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("list subscriptions endpoint %q returned 404", "subscriptions")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list subscriptions failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read list subscriptions response: %w", err)
	}
	subs, err := parseSubscriptionsResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse list subscriptions response: %w", err)
	}
	return subs, nil
}

func (s *HTTPSender) ListMessagesByChat(ctx context.Context, chatID string, count int) ([]Message, error) {
	if strings.TrimSpace(chatID) == "" {
		return nil, fmt.Errorf("chatID is required")
	}
	if count <= 0 {
		count = 50
	}
	if count > 100 {
		count = 100
	}

	u, err := s.endpointURL("messages")
	if err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("invalid messages URL: %w", err)
	}
	q := parsedURL.Query()
	q.Set("chat_id", chatID)
	q.Set("count", strconv.Itoa(count))
	parsedURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create list messages request: %w", err)
	}
	req.Header.Set("Authorization", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list messages request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("list messages endpoint %q returned 404", "messages")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list messages failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read list messages response: %w", err)
	}
	messages, err := parseMessagesResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse list messages response: %w", err)
	}
	return messages, nil
}

func (s *HTTPSender) SendText(ctx context.Context, chatID, threadID, text string, silent bool) error {
	chatIDInt, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat_id %q: %w", chatID, err)
	}
	if strings.TrimSpace(threadID) != "" {
		// MAX /messages does not support thread_id parameter.
		// Keep relay behavior predictable: ignore thread_id and send to chat.
	}
	payload := sendTextRequest{Text: text, Notify: !silent}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal text payload: %w", err)
	}

	u, err := s.endpointURL("messages")
	if err != nil {
		return err
	}
	parsedURL, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid messages URL: %w", err)
	}
	q := parsedURL.Query()
	q.Set("chat_id", strconv.FormatInt(chatIDInt, 10))
	parsedURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsedURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send text request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send text failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
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
	req.Header.Set("Authorization", s.token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send file request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send file failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
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

func (s *HTTPSender) listChatsByPath(ctx context.Context, endpointPath string) ([]Chat, error) {
	u, err := s.endpointURL(endpointPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create list chats request for %q: %w", endpointPath, err)
	}
	req.Header.Set("Authorization", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list chats request to %q: %w", endpointPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("list chats endpoint %q returned 404", endpointPath)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list chats endpoint %q failed with status %d", endpointPath, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read list chats response from %q: %w", endpointPath, err)
	}
	chats, err := parseChatsResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse list chats response from %q: %w", endpointPath, err)
	}
	return chats, nil
}

type chatsPageResponse struct {
	Chats  []map[string]any `json:"chats"`
	Marker *int64           `json:"marker"`
}

func (s *HTTPSender) listChatsPaginated(ctx context.Context) ([]Chat, error) {
	var (
		out    []Chat
		marker *int64
	)
	for {
		u, err := s.endpointURL("chats")
		if err != nil {
			return nil, err
		}
		parsedURL, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid chats URL: %w", err)
		}
		q := parsedURL.Query()
		q.Set("count", "100")
		if marker != nil {
			q.Set("marker", strconv.FormatInt(*marker, 10))
		}
		parsedURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create paginated chats request: %w", err)
		}
		req.Header.Set("Authorization", s.token)

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("paginated chats request: %w", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, fmt.Errorf("list chats endpoint %q returned 404", "chats")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("list chats endpoint %q failed with status %d", "chats", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read paginated chats response: %w", err)
		}

		page, err := parseChatsPageResponse(body)
		if err != nil {
			return nil, fmt.Errorf("parse paginated chats response: %w", err)
		}
		for _, c := range page.Chats {
			chatID := firstString(c, "id", "chat_id", "chatId")
			if chatID == "" {
				continue
			}
			title := firstString(c, "title", "name", "display_name")
			if title == "" {
				title = "<no-title>"
			}
			out = append(out, Chat{ID: chatID, Title: title})
		}
		if page.Marker == nil {
			break
		}
		next := *page.Marker
		marker = &next
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no chats in response")
	}
	return out, nil
}

func parseChatsPageResponse(body []byte) (chatsPageResponse, error) {
	var page chatsPageResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return chatsPageResponse{}, err
	}
	return page, nil
}

func parseSubscriptionsResponse(body []byte) ([]Subscription, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := flattenToObjects(raw["subscriptions"])
	if len(items) == 0 {
		return nil, fmt.Errorf("no subscriptions in response")
	}

	out := make([]Subscription, 0, len(items))
	for _, item := range items {
		subURL := firstString(item, "url", "endpoint", "webhook_url")
		if subURL == "" {
			subURL = "<no-url>"
		}
		out = append(out, Subscription{URL: subURL})
	}
	return out, nil
}

func parseMessagesResponse(body []byte) ([]Message, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := flattenToObjects(raw["messages"])
	if len(items) == 0 {
		return nil, fmt.Errorf("no messages in response")
	}

	out := make([]Message, 0, len(items))
	for _, item := range items {
		msgID := firstString(item, "id", "message_id", "messageId")
		if msgID == "" {
			msgID = "<no-id>"
		}
		text := firstString(item, "text", "body")
		if text == "" {
			text = "<no-text>"
		}
		out = append(out, Message{ID: msgID, Text: text})
	}
	return out, nil
}

func parseChatsResponse(body []byte) ([]Chat, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := flattenToObjects(raw)
	if len(items) == 0 {
		return nil, fmt.Errorf("no chats in response")
	}

	out := make([]Chat, 0, len(items))
	for _, item := range items {
		chatID := firstString(item, "id", "chat_id", "chatId")
		if chatID == "" {
			continue
		}
		title := firstString(item, "title", "name", "display_name")
		if title == "" {
			title = "<no-title>"
		}
		out = append(out, Chat{ID: chatID, Title: title})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no chat objects with id fields")
	}
	return out, nil
}

func flattenToObjects(v any) []map[string]any {
	switch x := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, it := range x {
			obj, ok := it.(map[string]any)
			if ok {
				out = append(out, obj)
			}
		}
		return out
	case map[string]any:
		for _, key := range []string{"chats", "items", "results", "data"} {
			if nested, ok := x[key]; ok {
				if arr := flattenToObjects(nested); len(arr) > 0 {
					return arr
				}
			}
		}
		return nil
	default:
		return nil
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			case float64:
				return strconv.FormatInt(int64(t), 10)
			}
		}
	}
	return ""
}
