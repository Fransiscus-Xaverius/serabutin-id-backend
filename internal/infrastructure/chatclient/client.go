package chatclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ConversationMeta is the metadata-only row returned by the chat backend's
// service-to-service admin endpoint. It deliberately carries no message
// content fields (ticket 8, v1 scope: metadata only).
type ConversationMeta struct {
	ConversationID string `json:"conversation_id"`
	UserAID        uint   `json:"user_a_id"`
	UserBID        uint   `json:"user_b_id"`
	MessageCount   int64  `json:"message_count"`
	LastActivityAt string `json:"last_activity_at"`
}

type pageMeta struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type listResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items      []ConversationMeta `json:"items"`
		Pagination pageMeta           `json:"pagination"`
	} `json:"data"`
}

// Client proxies the chat backend admin conversations endpoint over
// service-to-service auth (X-Service-Token). Timeouts are explicit per
// AGENT.md: the HTTP client caps at timeout and every call additionally
// derives a timeout context.
type Client struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
	timeout      time.Duration
}

func NewClient(baseURL, serviceToken string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: timeout},
		timeout:      timeout,
	}
}

// ListConversations fetches one metadata page. userID == 0 means no filter.
func (c *Client) ListConversations(ctx context.Context, userID uint, page, limit int) ([]ConversationMeta, int64, error) {
	if c.baseURL == "" {
		return nil, 0, fmt.Errorf("chat backend not configured (CHAT_BACKEND_BASE_URL empty)")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	q := url.Values{}
	if userID != 0 {
		q.Set("user_id", strconv.FormatUint(uint64(userID), 10))
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/admin/chats?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build chat directory request: %w", err)
	}
	if c.serviceToken != "" {
		req.Header.Set("X-Service-Token", c.serviceToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("contact chat backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("chat backend returned status %d", resp.StatusCode)
	}

	var out listResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("decode chat directory response: %w", err)
	}
	return out.Data.Items, out.Data.Pagination.Total, nil
}
