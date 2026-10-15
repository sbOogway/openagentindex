package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/temoto/robotstxt"
)

const (
	timeout      = 10 * time.Second
	maxBodyBytes = 1 << 20
	maxRedirects = 5
	robotsTTL    = 24 * time.Hour
)

var (
	ErrNotHTTPS         = errors.New("URL is not https")
	ErrTooManyRedirects = errors.New("too many redirects")
	ErrBodyTooLarge     = errors.New("response body is larger than 1 MiB")
	ErrBlockedAddress   = errors.New("address is not public")
	ErrDisallowed       = errors.New("robots.txt disallows the URL")
)

type Client struct {
	http      *http.Client
	userAgent string
	allow     func(netip.Addr) bool
	now       func() time.Time

	mu     sync.Mutex
	robots map[string]robotsRules
}

type robotsRules struct {
	data    *robotstxt.RobotsData
	expires time.Time
}

func New(userAgent string) *Client {
	c := &Client{userAgent: userAgent, allow: isPublic, now: time.Now, robots: map[string]robotsRules{}}
	c.http = newHTTPClient(timeout, c.control)
	return c
}

// HTTPClient returns an HTTP client for requests that agent cards control, for
// example the chat. Like Client, it connects only to public addresses and
// follows at most 5 redirects. It sends only HTTPS requests.
func HTTPClient(timeout time.Duration) *http.Client {
	hc := newHTTPClient(timeout, func(_, address string, _ syscall.RawConn) error {
		return checkAddress(isPublic, address)
	})
	hc.Transport = httpsOnly{next: hc.Transport}
	return hc
}

func newHTTPClient(timeout time.Duration, control func(network, address string, c syscall.RawConn) error) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, Control: control}
	return &http.Client{
		Timeout: timeout,
		// No proxy: a proxy would hide the target address from the SSRF check.
		Transport:     &http.Transport{DialContext: dialer.DialContext, ForceAttemptHTTP2: true},
		CheckRedirect: checkRedirect,
	}
}

type httpsOnly struct {
	next http.RoundTripper
}

func (t httpsOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := requireHTTPS(req.URL); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// Get returns the status code and the body of the response.
// It sends no request if robots.txt disallows the URL.
func (c *Client) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, err
	}
	if err := requireHTTPS(u); err != nil {
		return 0, nil, err
	}
	if err := c.checkRobots(ctx, u); err != nil {
		return 0, nil, err
	}
	return c.get(ctx, u)
}

func (c *Client) checkRobots(ctx context.Context, u *url.URL) error {
	robots, err := c.robotsData(ctx, u.Host)
	if err != nil {
		return fmt.Errorf("robots.txt: %w", err)
	}
	if !robots.TestAgent(u.RequestURI(), c.userAgent) {
		return fmt.Errorf("%w: %s", ErrDisallowed, u.Redacted())
	}
	return nil
}

func (c *Client) robotsData(ctx context.Context, host string) (*robotstxt.RobotsData, error) {
	c.mu.Lock()
	rules, ok := c.robots[host]
	c.mu.Unlock()
	if ok && c.now().Before(rules.expires) {
		return rules.data, nil
	}
	status, body, err := c.get(ctx, &url.URL{Scheme: "https", Host: host, Path: "/robots.txt"})
	if err != nil {
		return nil, err
	}
	data, err := robotstxt.FromStatusAndBytes(status, body)
	if err != nil {
		return nil, err
	}
	rules = robotsRules{data: data, expires: c.now().Add(robotsTTL)}
	c.mu.Lock()
	c.robots[host] = rules
	c.mu.Unlock()
	return rules.data, nil
}

func (c *Client) get(ctx context.Context, u *url.URL) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	return resp.StatusCode, body, err
}

func (c *Client) control(_, address string, _ syscall.RawConn) error {
	return checkAddress(c.allow, address)
}

func checkAddress(allow func(netip.Addr) bool, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if !allow(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
	}
	return nil
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate()
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return ErrTooManyRedirects
	}
	return requireHTTPS(req.URL)
}

func requireHTTPS(u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("%w: %s", ErrNotHTTPS, u.Redacted())
	}
	return nil
}

func readLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBodyBytes {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}
