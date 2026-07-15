package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"streammon/internal/config"
	"streammon/internal/util/logging"
)

const (
	testChannelID      = "UCaaaaaaaaaaaaaaaaaaaaaa"
	otherTestChannelID = "UCbbbbbbbbbbbbbbbbbbbbbb"
	testVideoID        = "WSrznNCR8LA"
	otherVideoID       = "XSrznNCR8LB"
)

type rewritingTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func TestEvaluateLivePageBody_RejectsDifferentOwnerFromStructuredPayload(t *testing.T) {
	body := `<html><head>` +
		`<link rel="canonical" href="https://www.youtube.com/watch?v=` + testVideoID + `">` +
		`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + testVideoID + `","channelId":"` + otherTestChannelID + `","isLive":true,"title":"Other Stream"},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"isLiveNow":true}}}};</script>` +
		`</head><body>{"channelId":"` + testChannelID + `"}</body></html>`

	eval := evaluateLivePageBody(body, testChannelID)
	if eval.hasOwnerMatch {
		t.Fatalf("expected owner mismatch to be rejected")
	}
	if eval.isLive {
		t.Fatalf("expected not live when canonical stream owner does not match channel")
	}
}

func TestEvaluateLivePageBody_DoesNotTrustGlobalLiveStatus(t *testing.T) {
	body := `<html><head>` +
		`<link rel="canonical" href="https://www.youtube.com/watch?v=` + testVideoID + `">` +
		`</head><body>` +
		`{"channelId":"` + testChannelID + `"}` +
		`{"videoId":"` + testVideoID + `"}` +
		`... lots of unrelated data ...` +
		`{"status":"LIVE"}` +
		`</body></html>`

	eval := evaluateLivePageBody(body, testChannelID)
	if !eval.hasOwnerMatch {
		t.Fatalf("expected loose owner match to be detected")
	}
	if eval.isLive {
		t.Fatalf("expected not live when LIVE marker is only global and unanchored")
	}
}

func TestEvaluateLivePageBody_CanonicalMissingDoesNotPromoteRandomVideoID(t *testing.T) {
	body := `<html><head></head><body>` +
		`{"channelId":"` + testChannelID + `"}` +
		`{"videoId":"` + otherVideoID + `"}` +
		`{"videoId":"` + testVideoID + `"}` +
		`{"status":"LIVE"}` +
		`</body></html>`

	eval := evaluateLivePageBody(body, testChannelID)
	if eval.videoID == "" {
		t.Fatalf("expected fallback to capture some video ID for diagnostics")
	}
	if eval.isLive {
		t.Fatalf("expected not live without anchored ownership and structured payload")
	}
}

func TestEvaluateLivePageBody_ScheduledButNotLiveFromStructuredPayload(t *testing.T) {
	body := `<html><head>` +
		`<link rel="canonical" href="https://www.youtube.com/watch?v=` + testVideoID + `">` +
		`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + testVideoID + `","channelId":"` + testChannelID + `","isLive":false,"title":"Upcoming"},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"scheduledStartTime":"1999999999","isLiveNow":false}}}};</script>` +
		`</head><body></body></html>`

	eval := evaluateLivePageBody(body, testChannelID)
	if !eval.isScheduled {
		t.Fatalf("expected scheduled stream to be detected")
	}
	if eval.isLive {
		t.Fatalf("expected scheduled-but-not-live stream to be offline")
	}
}

func TestEvaluateLivePageBody_AcceptsStructuredOwnedLive(t *testing.T) {
	body := `<html><head>` +
		`<link rel="canonical" href="https://www.youtube.com/watch?v=` + testVideoID + `">` +
		`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + testVideoID + `","channelId":"` + testChannelID + `","isLive":true,"title":"Owned Live"},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"isLiveNow":true}}}};</script>` +
		`</head><body></body></html>`

	eval := evaluateLivePageBody(body, testChannelID)
	if !eval.hasOwnerMatch {
		t.Fatalf("expected owner match")
	}
	if !eval.isLive {
		t.Fatalf("expected live stream from structured payload")
	}
	if eval.videoID != testVideoID {
		t.Fatalf("expected video id %s, got %s", testVideoID, eval.videoID)
	}
	if eval.title != "Owned Live" {
		t.Fatalf("expected title from structured payload, got %q", eval.title)
	}
}

func TestCheckYouTubeViaLivePage_UsesExpectedRequestAndParsesResponse(t *testing.T) {
	logger := logging.NewLogger(&config.GlobalConfig{Timezone: "UTC"}, "YT", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channel/"+testChannelID+"/live" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Fatalf("expected User-Agent header to be set")
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "text/html") {
			t.Fatalf("expected browser-style Accept header, got %q", got)
		}
		if got := r.Header.Get("Sec-Fetch-Mode"); got != "navigate" {
			t.Fatalf("expected Sec-Fetch-Mode navigate, got %q", got)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head>`+
			`<link rel="canonical" href="https://www.youtube.com/watch?v=`+testVideoID+`">`+
			`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"`+testVideoID+`","channelId":"`+testChannelID+`","isLive":true,"title":"HTTP Live"},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"isLiveNow":true}}}};</script>`+
			`</head><body></body></html>`)
	}))
	defer server.Close()

	targetURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	client := &http.Client{
		Transport: rewritingTransport{target: targetURL, base: server.Client().Transport},
	}

	info, err := CheckYouTubeViaLivePage(context.Background(), client, testChannelID, "Test Channel", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.IsLive {
		t.Fatalf("expected live result")
	}
	if info.VideoID != testVideoID {
		t.Fatalf("expected video id %s, got %s", testVideoID, info.VideoID)
	}
	if info.Title != "HTTP Live" {
		t.Fatalf("expected title from HTTP response, got %q", info.Title)
	}
}

func TestCheckYouTubeViaLivePage_ReturnsErrorOnNonOK(t *testing.T) {
	logger := logging.NewLogger(&config.GlobalConfig{Timezone: "UTC"}, "YT", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	targetURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	client := &http.Client{
		Transport: rewritingTransport{target: targetURL, base: server.Client().Transport},
	}

	_, err = CheckYouTubeViaLivePage(context.Background(), client, testChannelID, "Test Channel", logger)
	if err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}
