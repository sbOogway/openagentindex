package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type robotsServer struct {
	*httptest.Server
	robotsRequests atomic.Int32
	pageRequests   atomic.Int32
}

func newRobotsServer(t *testing.T, status int, robots string) *robotsServer {
	t.Helper()
	s := &robotsServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		s.robotsRequests.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(robots))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		s.pageRequests.Add(1)
		_, _ = w.Write([]byte("page"))
	})
	s.Server = httptest.NewTLSServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestGetObeysRobots(t *testing.T) {
	tests := []struct {
		name   string
		status int
		robots string
		path   string
		want   error
	}{
		{
			name:   "path disallowed for this agent",
			status: http.StatusOK,
			robots: "User-agent: OpenAgentIndexBot\nDisallow: /private\n",
			path:   "/private/card.json",
			want:   ErrDisallowed,
		},
		{
			name:   "other path for this agent",
			status: http.StatusOK,
			robots: "User-agent: OpenAgentIndexBot\nDisallow: /private\n",
			path:   "/.well-known/agent-card.json",
		},
		{
			name:   "rules for another agent",
			status: http.StatusOK,
			robots: "User-agent: OtherBot\nDisallow: /\n",
			path:   "/.well-known/agent-card.json",
		},
		{
			name:   "rules for all agents",
			status: http.StatusOK,
			robots: "User-agent: *\nDisallow: /\n",
			path:   "/.well-known/agent-card.json",
			want:   ErrDisallowed,
		},
		{
			name:   "this agent allowed, all others disallowed",
			status: http.StatusOK,
			robots: "User-agent: *\nDisallow: /\n\nUser-agent: OpenAgentIndexBot\nAllow: /\n",
			path:   "/.well-known/agent-card.json",
		},
		{
			name:   "no robots.txt",
			status: http.StatusNotFound,
			path:   "/.well-known/agent-card.json",
		},
		{
			name:   "robots.txt server error",
			status: http.StatusInternalServerError,
			path:   "/.well-known/agent-card.json",
			want:   ErrDisallowed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newRobotsServer(t, tt.status, tt.robots)
			c := newTestClient(srv.Server)

			_, _, err := c.Get(context.Background(), srv.URL+tt.path)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			wantPages := int32(1)
			if tt.want != nil {
				wantPages = 0
			}
			if got := srv.pageRequests.Load(); got != wantPages {
				t.Errorf("page requests = %d, want %d", got, wantPages)
			}
		})
	}
}

func TestGetRobotsNoResponse(t *testing.T) {
	srv := newRobotsServer(t, http.StatusOK, "")
	c := newTestClient(srv.Server)
	c.allow = isPublic

	if _, _, err := c.Get(context.Background(), srv.URL+"/page"); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
	if got := srv.robotsRequests.Load() + srv.pageRequests.Load(); got != 0 {
		t.Errorf("server got %d requests, want 0", got)
	}
}

func TestGetCachesRobots(t *testing.T) {
	srv := newRobotsServer(t, http.StatusOK, "User-agent: *\nAllow: /\n")
	c := newTestClient(srv.Server)
	now := time.Unix(1792000000, 0)
	c.now = func() time.Time { return now }

	get := func() {
		t.Helper()
		if _, _, err := c.Get(context.Background(), srv.URL+"/page"); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	get()
	now = now.Add(robotsTTL - time.Second)
	get()
	if got := srv.robotsRequests.Load(); got != 1 {
		t.Errorf("robots.txt requests before 24 h = %d, want 1", got)
	}
	now = now.Add(time.Second)
	get()
	if got := srv.robotsRequests.Load(); got != 2 {
		t.Errorf("robots.txt requests after 24 h = %d, want 2", got)
	}
}
