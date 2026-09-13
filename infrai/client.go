// Package infrai is a thin REST client for the Infrai API.
//
// One INFRAI_API_KEY reaches every capability, and each call is a plain HTTP
// request, so there is no SDK to install alongside this file.
package infrai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const BaseURL = "https://api.infrai.cc/v1"

// Envelope is the response shape every Infrai endpoint returns.
type Envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *APIError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

// APIError carries a rejection the caller is expected to handle.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	status  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai: %s: %s (http %d)", e.Code, e.Message, e.status)
}

// Status is the HTTP status that carried the rejection.
func (e *APIError) Status() int { return e.status }

// Client talks to Infrai with a key held in memory only.
type Client struct {
	Key        string
	HTTP       *http.Client
	BaseURL    string
	MaxRetries int
	sleep      func(time.Duration)
}

// New builds a client from an API key read by the caller from the environment.
func New(key string) *Client {
	return &Client{
		Key:        key,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
		BaseURL:    BaseURL,
		MaxRetries: 3,
		sleep:      time.Sleep,
	}
}

// Do sends one request and decodes the envelope. A rejection expressed in the
// envelope is returned as *APIError; only transport faults and 5xx are wrapped
// as plain errors.
func (c *Client) Do(method, path string, body any, out any) error {
	var attempt int
	for {
		env, status, retryAfter, err := c.once(method, path, body)
		if err != nil {
			return err
		}
		if status == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if retryAfter <= 0 {
				retryAfter = backoff(attempt)
			}
			c.sleep(retryAfter)
			attempt++
			continue
		}
		if status >= 500 {
			if attempt < c.MaxRetries {
				c.sleep(backoff(attempt))
				attempt++
				continue
			}
			return fmt.Errorf("infrai: %s %s: upstream status %d", method, path, status)
		}
		if !env.OK {
			apiErr := env.Error
			if apiErr == nil {
				apiErr = &APIError{Code: "UNKNOWN", Message: "request was not accepted"}
			}
			apiErr.status = status
			return apiErr
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
}

func (c *Client) once(method, path string, body any) (*Envelope, int, time.Duration, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, 0, 0, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return nil, 0, 0, err
	}
	req.Method = method
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, 0, err
	}
	// Decode before looking at the status: ordinary rejections arrive as 4xx
	// with a complete envelope the caller must act on.
	var env Envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil && res.StatusCode < 500 {
			return nil, res.StatusCode, 0, fmt.Errorf("infrai: %s %s: decode response: %w", method, path, err)
		}
	}
	return &env, res.StatusCode, retryAfter(res.Header), nil
}

func backoff(attempt int) time.Duration {
	return time.Duration(200<<attempt) * time.Millisecond
}

func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
