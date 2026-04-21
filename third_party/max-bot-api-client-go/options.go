package maxbot

import (
	"net/url"
	"time"
)

type Option func(api *Api)

func WithBaseURL(baseURL string) Option {
	return func(api *Api) {
		u, err := url.Parse(baseURL)
		if err != nil {
			return
		}
		api.client.baseURL = u
	}
}

func WithHTTPClient(httpClient HttpClient) Option {
	return func(api *Api) {
		api.client.httpClient = httpClient
	}
}

func WithApiTimeout(timeout time.Duration) Option {
	return func(api *Api) {
		api.pollTimeout = timeout
	}
}
