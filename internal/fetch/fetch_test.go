package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
)

const userAgent = "OpenAgentIndexBot/test"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/redirect/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		if n == 0 {
			_, _ = w.Write([]byte("done"))
			return
		}
		http.Redirect(w, r, "/redirect/"+strconv.Itoa(n-1), http.StatusFound)
	})
	mux.HandleFunc("/to-http", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/ok", http.StatusFound)
	})
	mux.HandleFunc("/size/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		_, _ = w.Write([]byte(strings.Repeat("a", n)))
	})
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestClient trusts the test server certificate and allows loopback addresses.
func newTestClient(srv *httptest.Server) *Client {
	c := New(userAgent)
	c.allow = func(netip.Addr) bool { return true }
	transport := c.http.Transport.(*http.Transport)
	transport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	return c
}

func TestGet(t *testing.T) {
	srv := newServer(t)
	c := newTestClient(srv)

	status, body, err := c.Get(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status != http.StatusTeapot {
		t.Errorf("status = %d, want %d", status, http.StatusTeapot)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetLimits(t *testing.T) {
	srv := newServer(t)
	c := newTestClient(srv)

	tests := []struct {
		name string
		url  string
		want error
	}{
		{name: "5 redirects", url: srv.URL + "/redirect/5"},
		{name: "6 redirects", url: srv.URL + "/redirect/6", want: ErrTooManyRedirects},
		{name: "http URL", url: strings.Replace(srv.URL, "https://", "http://", 1) + "/ok", want: ErrNotHTTPS},
		{name: "redirect to http", url: srv.URL + "/to-http", want: ErrNotHTTPS},
		{name: "body of 1 MiB", url: srv.URL + "/size/" + strconv.Itoa(maxBodyBytes)},
		{name: "body larger than 1 MiB", url: srv.URL + "/size/" + strconv.Itoa(maxBodyBytes+1), want: ErrBodyTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := c.Get(context.Background(), tt.url)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGetTimeout(t *testing.T) {
	srv := newServer(t)
	c := newTestClient(srv)
	c.http.Timeout = 50 * time.Millisecond

	if _, _, err := c.Get(context.Background(), srv.URL+"/slow"); err == nil {
		t.Fatal("Get: no error, want a timeout")
	}
}

func TestGetBlocksLoopback(t *testing.T) {
	srv := newServer(t)
	c := newTestClient(srv)
	c.allow = isPublic

	if _, _, err := c.Get(context.Background(), srv.URL+"/ok"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestIsPublic(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1"},
		{addr: "::1"},
		{addr: "10.0.0.1"},
		{addr: "172.16.0.1"},
		{addr: "192.168.1.1"},
		{addr: "169.254.169.254"},
		{addr: "fc00::1"},
		{addr: "fe80::1"},
		{addr: "224.0.0.1"},
		{addr: "0.0.0.0"},
		{addr: "::ffff:127.0.0.1"},
		{addr: "1.1.1.1", want: true},
		{addr: "2606:4700:4700::1111", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := isPublic(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("isPublic(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}
