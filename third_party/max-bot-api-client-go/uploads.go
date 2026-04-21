package maxbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type uploads struct {
	client *client
}

func (u *uploads) UploadMediaFromReaderWithName(ctx context.Context, uploadType schemes.UploadType, reader io.Reader, name string) (*schemes.UploadedInfo, error) {
	endpoint, err := u.getUploadURL(ctx, uploadType)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileName := strings.TrimSpace(name)
	if fileName == "" {
		fileName = "attachment.bin"
	}
	part, err := writer.CreateFormFile("data", filepath.Base(fileName))
	if err != nil {
		return nil, fmt.Errorf("create upload form file: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return nil, fmt.Errorf("write upload body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close upload body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, &body)
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", u.client.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := u.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(payload))}
	}

	var uploaded schemes.UploadedInfo
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		return nil, fmt.Errorf("decode upload response: %w", err)
	}
	if strings.TrimSpace(uploaded.Token) == "" && strings.TrimSpace(endpoint.Token) != "" {
		uploaded.Token = endpoint.Token
	}
	if strings.TrimSpace(uploaded.Token) == "" {
		return nil, fmt.Errorf("upload response does not contain token")
	}
	return &uploaded, nil
}

func (u *uploads) getUploadURL(ctx context.Context, uploadType schemes.UploadType) (*schemes.UploadEndpoint, error) {
	values := url.Values{}
	values.Set("type", string(uploadType))

	var endpoint schemes.UploadEndpoint
	if err := u.client.requestJSON(ctx, http.MethodPost, "uploads", values, nil, &endpoint); err != nil {
		return nil, err
	}
	if strings.TrimSpace(endpoint.URL) == "" {
		return nil, fmt.Errorf("empty upload url")
	}
	return &endpoint, nil
}
