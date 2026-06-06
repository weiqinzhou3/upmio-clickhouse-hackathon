package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Sample struct {
	Metric map[string]string
	Value  float64
}

type queryResponse struct {
	Status    string    `json:"status"`
	ErrorType string    `json:"errorType"`
	Error     string    `json:"error"`
	Data      queryData `json:"data"`
}

type queryData struct {
	ResultType string        `json:"resultType"`
	Result     []queryResult `json:"result"`
}

type queryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []json.RawMessage `json:"value"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Query(ctx context.Context, query string) ([]Sample, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("build Prometheus query URL: %w", err)
	}
	values := endpoint.Query()
	values.Set("query", query)
	endpoint.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Prometheus query request: %w", err)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query Prometheus: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read Prometheus query response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Prometheus query returned HTTP %d", response.StatusCode)
	}

	var payload queryResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Prometheus query response: %w", err)
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("Prometheus query failed: %s: %s", payload.ErrorType, payload.Error)
	}
	if payload.Data.ResultType != "vector" {
		return nil, fmt.Errorf("Prometheus query returned unsupported result type %q", payload.Data.ResultType)
	}

	samples := make([]Sample, 0, len(payload.Data.Result))
	for _, result := range payload.Data.Result {
		if len(result.Value) != 2 {
			return nil, fmt.Errorf("Prometheus vector sample has invalid value")
		}
		var valueText string
		if err := json.Unmarshal(result.Value[1], &valueText); err != nil {
			return nil, fmt.Errorf("decode Prometheus sample value: %w", err)
		}
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			return nil, fmt.Errorf("parse Prometheus sample value: %w", err)
		}
		samples = append(samples, Sample{Metric: result.Metric, Value: value})
	}
	return samples, nil
}
