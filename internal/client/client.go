package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"lnr-cli/internal/encoding"
)

const (
	DefaultTimeout   = 20 * time.Second
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

// HttpClient wraps http.Client with custom headers, cookies, and charset streaming.
type HttpClient struct {
	client *http.Client
}

// NewHttpClient creates an HTTP client configured with cookie session for Wenku8.
func NewHttpClient() (*HttpClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	c := &HttpClient{
		client: &http.Client{
			Jar:     jar,
			Timeout: DefaultTimeout,
		},
	}

	// Set default wenku8 cookies across supported domains
	domains := []string{"https://www.wenku8.cc", "https://www.wenku8.net", "https://www.wenku8.com"}
	for _, rawURL := range domains {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		c.client.Jar.SetCookies(u, []*http.Cookie{
			{
				Name:  "jieqiUserInfo",
				Value: "jieqiUserId=1125456,jieqiUserName=yyhyy,jieqiUserGroup=3,jieqiUserVip=0,jieqiUserPassword=eb62861281462fd923fb99218735fef0,jieqiUserName_un=yyhyy,jieqiUserHonor_un=&#x4E2D;&#x7EA7;&#x4F1A;&#x5458;,jieqiUserGroupName_un=&#x666E;&#x901A;&#x4F1A;&#x5458;,jieqiUserLogin=1739294499",
			},
			{
				Name:  "jieqiVisitInfo",
				Value: "jieqiUserLogin=1739294499,jieqiUserId=1125456",
			},
			{
				Name:  "HMACCOUNT",
				Value: "E7837B0FF79F0590",
			},
		})
	}

	return c, nil
}

// GetStream makes an HTTP GET request and returns an io.ReadCloser streaming UTF-8 bytes.
// It automatically converts GB18030 content into UTF-8 without buffering the entire body.
func (c *HttpClient) GetStream(ctx context.Context, targetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Stream GB18030 to UTF-8
	utf8Reader := encoding.GB18030ToUTF8Reader(resp.Body)

	return &streamCloser{
		Reader: utf8Reader,
		Closer: resp.Body,
	}, nil
}

// GetRawStream makes an HTTP GET request and returns the raw response body stream (e.g. for images/binary).
func (c *HttpClient) GetRawStream(ctx context.Context, targetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Referer", "https://www.wenku8.cc/")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

type streamCloser struct {
	io.Reader
	io.Closer
}

func (s *streamCloser) Read(p []byte) (n int, err error) {
	return s.Reader.Read(p)
}

func (s *streamCloser) Close() error {
	return s.Closer.Close()
}
