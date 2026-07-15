package youtube

import "testing"

const (
	testChannelID      = "UCaaaaaaaaaaaaaaaaaaaaaa"
	otherTestChannelID = "UCbbbbbbbbbbbbbbbbbbbbbb"
	testVideoID        = "WSrznNCR8LA"
	otherVideoID       = "XSrznNCR8LB"
)

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
