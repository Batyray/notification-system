package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type SendRequest struct {
	To            string `json:"to"`
	Channel       string `json:"channel"`
	Content       string `json:"content"`
	CorrelationID string `json:"-"`
}

type SendResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

type SendError struct {
	StatusCode int
	Body       string
}

func (e *SendError) Error() string {
	return fmt.Sprintf("delivery failed with status %d: %s", e.StatusCode, e.Body)
}

func (e *SendError) IsTransient() bool {
	return e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests || e.StatusCode == 0
}

func (e *SendError) IsRateLimited() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Send(ctx context.Context, req SendRequest) (*SendResponse, error) {
	payload := map[string]string{
		"to":      req.To,
		"channel": req.Channel,
		"content": req.Content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.CorrelationID != "" {
		httpReq.Header.Set("X-Correlation-ID", req.CorrelationID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &SendError{StatusCode: 0, Body: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return nil, &SendError{StatusCode: resp.StatusCode}
	}

	var sendResp SendResponse
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &sendResp, nil
}
