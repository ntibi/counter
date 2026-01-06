package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type pushRequest struct {
	Streams []stream `json:"streams"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

func (c *Client) Push(labels map[string]string, timestamp time.Time, line string) error {
	req := pushRequest{
		Streams: []stream{
			{
				Stream: labels,
				Values: [][]string{
					{strconv.FormatInt(timestamp.UnixNano(), 10), line},
				},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal push request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/loki/api/v1/push",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("push to loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("loki push failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	return nil
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value  []interface{} `json:"value"`
			Values [][]string    `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (c *Client) CountSince(labels map[string]string, since time.Time) (int, error) {
	labelSelector := "{"
	first := true
	for k, v := range labels {
		if !first {
			labelSelector += ","
		}
		labelSelector += fmt.Sprintf(`%s="%s"`, k, v)
		first = false
	}
	labelSelector += "}"

	duration := time.Since(since).Truncate(time.Second)
	if duration < time.Second {
		duration = time.Second
	}
	query := fmt.Sprintf(`count_over_time(%s[%s])`, labelSelector, duration)

	params := url.Values{}
	params.Set("query", query)
	params.Set("time", strconv.FormatInt(time.Now().Unix(), 10))

	resp, err := c.httpClient.Get(c.baseURL + "/loki/api/v1/query?" + params.Encode())
	if err != nil {
		return 0, fmt.Errorf("query loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("loki query failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var result queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode query response: %w", err)
	}

	if len(result.Data.Result) == 0 {
		return 0, nil
	}

	if len(result.Data.Result[0].Value) < 2 {
		return 0, nil
	}

	countStr, ok := result.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected count type: %T", result.Data.Result[0].Value[1])
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("parse count: %w", err)
	}

	return count, nil
}

func (c *Client) LastTimestamp(labels map[string]string) (time.Time, error) {
	labelSelector := "{"
	first := true
	for k, v := range labels {
		if !first {
			labelSelector += ","
		}
		labelSelector += fmt.Sprintf(`%s="%s"`, k, v)
		first = false
	}
	labelSelector += "}"

	params := url.Values{}
	params.Set("query", labelSelector)
	params.Set("limit", "1")
	params.Set("direction", "backward")
	params.Set("start", strconv.FormatInt(time.Now().Add(-24*time.Hour).UnixNano(), 10))
	params.Set("end", strconv.FormatInt(time.Now().UnixNano(), 10))

	resp, err := c.httpClient.Get(c.baseURL + "/loki/api/v1/query_range?" + params.Encode())
	if err != nil {
		return time.Time{}, fmt.Errorf("query loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return time.Time{}, fmt.Errorf("loki query failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var result queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, fmt.Errorf("decode query response: %w", err)
	}

	if len(result.Data.Result) == 0 || len(result.Data.Result[0].Values) == 0 {
		return time.Time{}, nil
	}

	tsStr := result.Data.Result[0].Values[0][0]
	tsNano, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp: %w", err)
	}

	return time.Unix(0, tsNano), nil
}
