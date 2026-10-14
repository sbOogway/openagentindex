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
	"syscall"
	"time"
)

const (
	timeout      = 10 * time.Second
	maxBodyBytes = 1 << 20
	maxRedirects = 5
)

var (
	ErrNotHTTPS         = errors.New("URL is not https")
	ErrTooManyRedirects = errors.New("too many redirects")
	ErrBodyTooLarge     = errors.New("response body is larger than 1 MiB")
	ErrBlockedAddress   = errors.New("address is not public")
)

type Client struct {
	http      *http.Client
	userAgent string
	allow     func(netip.Addr) bool
}

func New(userAgent string) *Client {
	c := &Client{userAgent: userAgent, allow: isPublic}
	dialer := &net.Dialer{Timeout: timeout, Control: c.control}
	c.http = &http.Client{
		Timeout: timeout,
		// No proxy: a proxy would hide the target address from the SSRF check.
		Transport:     &http.Transport{DialContext: dialer.DialContext, ForceAttemptHTTP2: true},
		CheckRedirect: checkRedirect,
	}
	return c
}

// Get returns the status code and the body of the response.
func (c *Client) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, err
	}
	if err := requireHTTPS(u); err != nil {
		return 0, nil, err
	}
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
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if !c.allow(ip) {
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
