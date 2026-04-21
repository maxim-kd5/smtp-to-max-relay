package maxbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (c *client) requestJSON(ctx context.Context, method, endpoint string, values url.Values, body any, out any) error {
	req, err := c.newRequest(ctx, method, endpoint, values, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		payload, _ := io.ReadAll(resp.Body)
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, apiErr)
			if apiErr.Message == "" && apiErr.Code == "" {
				apiErr.Message = strings.TrimSpace(string(payload))
			}
		}
		return apiErr
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s response: %w", method, endpoint, err)
	}
	return nil
}

func (c *client) newRequest(ctx context.Context, method, endpoint string, values url.Values, body any) (*http.Request, error) {
	if c.baseURL == nil {
		return nil, ErrInvalidURL
	}

	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)
	if len(values) > 0 {
		u.RawQuery = values.Encode()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal %s %s payload: %w", method, endpoint, err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("create %s %s request: %w", method, endpoint, err)
	}
	req.Header.Set("Authorization", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
