package clickhouse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) Query(ctx context.Context, endpoint, user, password, query string) ([]byte, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse ClickHouse endpoint: %w", err)
	}
	values := requestURL.Query()
	values.Set("query", query)
	requestURL.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), strings.NewReader(""))
	if err != nil {
		return nil, fmt.Errorf("create ClickHouse request: %w", err)
	}
	if user != "" {
		request.SetBasicAuth(user, password)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read ClickHouse response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ClickHouse returned HTTP %d", response.StatusCode)
	}
	return body, nil
}
