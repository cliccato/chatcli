// Package api talks to the ch.at plain-text endpoint.
package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	endpoint = "https://ch.at/"
	timeout  = 3 * time.Minute

	// MaxHistory is the largest transcript accepted by the server.
	MaxHistory = 64 * 1024

	// ch.at renders differently per user agent: a browser gets an HTML page
	// and non-curl tools get the whole transcript back. The "curl/" prefix
	// selects the plain single-exchange reply, plus our name for tracing.
	userAgent = "curl/8.5.0 chatcli/1.0"

	chunkSize = 16 * 1024
)

type Client struct {
	http *http.Client
}

// NewClient returns a client that reuses connections between questions.
func NewClient() *Client {
	return &Client{http: &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        8,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}}
}

// Ask sends the question with the transcript and streams the answer through
// emit. It returns the full answer text.
func (c *Client) Ask(ctx context.Context, question, history string, emit func([]byte)) (string, error) {
	if emit == nil {
		emit = func([]byte) {}
	}

	form := url.Values{}
	form.Set("q", question)
	form.Set("h", history)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %s", resp.Status)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return "", fmt.Errorf("server returned an HTML page instead of an answer")
	}

	s := newAnswerStream(question, emit)
	buf := make([]byte, chunkSize)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			s.feed(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return s.finish()
}
