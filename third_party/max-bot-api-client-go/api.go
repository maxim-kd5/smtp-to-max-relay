package maxbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

var (
	ErrEmptyToken = errors.New("bot token is empty")
	ErrInvalidURL = errors.New("invalid API URL")
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type APIError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	StatusCode int    `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 3)
	if e.Code != "" {
		parts = append(parts, e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.Details != "" {
		parts = append(parts, e.Details)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("max api error status=%d", e.StatusCode)
	}
	return strings.Join(parts, ": ")
}

func (e *APIError) IsAttachmentNotReady() bool {
	if e == nil {
		return false
	}
	text := strings.ToLower(e.Code + " " + e.Message + " " + e.Details)
	return strings.Contains(text, "attachment.not.ready") ||
		strings.Contains(text, "not.ready") ||
		strings.Contains(text, "not.processed")
}

type Api struct {
	Bots     *bots
	Messages *messages
	Uploads  *uploads

	client      *client
	pollTimeout time.Duration
	pauseDelay  time.Duration

	mu        sync.Mutex
	updatesCh chan schemes.UpdateInterface
}

type client struct {
	baseURL    *url.URL
	httpClient HttpClient
	token      string
	errors     chan error
}

func New(token string, opts ...Option) (*Api, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrEmptyToken
	}

	baseURL, err := url.Parse("https://platform-api.max.ru")
	if err != nil {
		return nil, ErrInvalidURL
	}

	api := &Api{
		client: &client{
			baseURL:    baseURL,
			httpClient: &http.Client{Timeout: 35 * time.Second},
			token:      token,
			errors:     make(chan error, 32),
		},
		pollTimeout: 30 * time.Second,
		pauseDelay:  time.Second,
	}
	api.Bots = &bots{client: api.client}
	api.Messages = &messages{client: api.client}
	api.Uploads = &uploads{client: api.client}

	for _, opt := range opts {
		if opt != nil {
			opt(api)
		}
	}
	if api.client.baseURL == nil {
		return nil, ErrInvalidURL
	}
	if api.client.httpClient == nil {
		api.client.httpClient = &http.Client{Timeout: 35 * time.Second}
	}
	if api.pollTimeout <= 0 {
		api.pollTimeout = 30 * time.Second
	}
	if api.pauseDelay <= 0 {
		api.pauseDelay = time.Second
	}

	return api, nil
}

func (a *Api) GetErrors() <-chan error {
	return a.client.errors
}

func (a *Api) GetUpdates(ctx context.Context) <-chan schemes.UpdateInterface {
	a.mu.Lock()
	if a.updatesCh != nil {
		ch := a.updatesCh
		a.mu.Unlock()
		return ch
	}

	ch := make(chan schemes.UpdateInterface, 100)
	a.updatesCh = ch
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.updatesCh = nil
			a.mu.Unlock()
			close(ch)
		}()

		var marker *int64
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			resp, err := a.getUpdatesPage(ctx, marker)
			if err != nil {
				a.notifyError(err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(a.pauseDelay):
					continue
				}
			}

			marker = resp.Marker
			for _, upd := range resp.Updates {
				select {
				case <-ctx.Done():
					return
				case ch <- upd:
				}
			}
		}
	}()

	return ch
}

type updatesPage struct {
	Updates []schemes.UpdateInterface
	Marker  *int64
}

type updatesEnvelope struct {
	Updates []json.RawMessage `json:"updates"`
	Marker  *int64            `json:"marker"`
}

func (a *Api) getUpdatesPage(ctx context.Context, marker *int64) (updatesPage, error) {
	values := url.Values{}
	values.Set("limit", "100")
	values.Set("timeout", strconv.Itoa(int(a.pollTimeout.Seconds())))
	values.Set("types", string(schemes.UpdateTypeMessageCreated))
	if marker != nil {
		values.Set("marker", strconv.FormatInt(*marker, 10))
	}

	var env updatesEnvelope
	if err := a.client.requestJSON(ctx, http.MethodGet, "updates", values, nil, &env); err != nil {
		return updatesPage{}, err
	}

	out := updatesPage{Marker: env.Marker}
	for _, raw := range env.Updates {
		upd, err := decodeUpdate(raw)
		if err != nil {
			a.notifyError(fmt.Errorf("decode update: %w", err))
			continue
		}
		if upd != nil {
			out.Updates = append(out.Updates, upd)
		}
	}

	return out, nil
}

type updateHeader struct {
	UpdateType schemes.UpdateType `json:"update_type"`
}

func decodeUpdate(raw json.RawMessage) (schemes.UpdateInterface, error) {
	var header updateHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, err
	}

	switch header.UpdateType {
	case schemes.UpdateTypeMessageCreated:
		var upd schemes.MessageCreatedUpdate
		if err := json.Unmarshal(raw, &upd); err != nil {
			return nil, err
		}
		return &upd, nil
	default:
		return nil, nil
	}
}

func (a *Api) notifyError(err error) {
	if err == nil {
		return
	}
	select {
	case a.client.errors <- err:
	default:
	}
}
