package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token string
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), token: cfg.Token, http: &http.Client{Timeout: cfg.Timeout}}
}

func (c *Client) Create(ctx context.Context) (Sandbox, error) {
	var out Sandbox
	return out, c.do(ctx, http.MethodPost, "/sandboxes", nil, &out)
}

func (c *Client) Execute(ctx context.Context, id, command string) (ExecuteResponse, error) {
	var out ExecuteResponse
	return out, c.do(ctx, http.MethodPost, "/sandboxes/"+id+"/execute", ExecuteRequest{Command: command}, &out)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/sandboxes/"+id, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil { return err }
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil { return err }
	req.Header.Set("Accept", "application/json")
	if body != nil { req.Header.Set("Content-Type", "application/json") }
	if c.token != "" { req.Header.Set("Authorization", "Bearer "+c.token) }
	resp, err := c.http.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil { return err }
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil { return fmt.Errorf("decode %s %s: %w", method, path, err) }
	}
	return nil
}

func timed(fn func() error) (float64, error) {
	start := time.Now()
	err := fn()
	return float64(time.Since(start).Microseconds()) / 1000, err
}
