package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
	timeout time.Duration
}

type HTTPError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func IsConcurrencyLimit(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusTooManyRequests
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		http:    &http.Client{Timeout: cfg.Timeout},
		timeout: cfg.Timeout,
	}
}

func (c *Client) Create(ctx context.Context) (Sandbox, error) {
	var out Sandbox
	if err := c.do(ctx, http.MethodPost, "/sandboxes", nil, &out); err != nil {
		return Sandbox{}, err
	}
	if out.ID == "" {
		return Sandbox{}, fmt.Errorf("POST /sandboxes returned no sandboxId")
	}
	return out, nil
}

func (c *Client) Execute(ctx context.Context, id, command string) (ExecuteResponse, error) {
	if id == "" {
		return ExecuteResponse{}, fmt.Errorf("refusing to execute with an empty sandboxId")
	}
	var out ExecuteResponse
	timeoutMS := int(c.timeout.Milliseconds())
	if timeoutMS < 1 {
		timeoutMS = 1
	}
	body := ExecuteRequest{Cmd: "sh", Args: []string{"-c", command}, TimeoutMS: timeoutMS}
	path := "/sandboxes/" + url.PathEscape(id) + "/exec"
	return out, c.do(ctx, http.MethodPost, path, body, &out)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("refusing to delete with an empty sandboxId")
	}
	return c.do(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(id), nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &HTTPError{Method: method, Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

func timed(fn func() error) (float64, error) {
	start := time.Now()
	err := fn()
	return float64(time.Since(start).Microseconds()) / 1000, err
}
