package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// TwitchClientID is a public, hardcoded client ID for the Twitch GQL API.
// This is a common practice for third-party tools.
const TwitchClientID = "kimne78kx3ncx6brgo4mv6wki5h1ko"

// --- GQL Request and Response Structures ---

type gqlRequest struct {
	OperationName string `json:"operationName"`
	Variables     struct {
		Login string `json:"channelLogin"`
	} `json:"variables"`
	Extensions struct {
		PersistedQuery struct {
			Version    int    `json:"version"`
			SHA256Hash string `json:"sha256Hash"`
		} `json:"persistedQuery"`
	} `json:"extensions"`
}

type gqlResponse struct {
	Data struct {
		User struct {
			Stream *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"stream"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// --- API Function ---

// CheckLiveGQL performs a lightweight check to see if a Twitch channel is live.
func CheckLiveGQL(httpClient *http.Client, channelLogin string) (LiveInfo, error) {
	// Construct the GQL request payload
	payload := gqlRequest{
		OperationName: "UseLive",
	}
	payload.Variables.Login = strings.ToLower(channelLogin)
	payload.Extensions.PersistedQuery.Version = 1
	payload.Extensions.PersistedQuery.SHA256Hash = "639d5f11bfb8bf3053b424d9ef650d04c4ebb7d94711d644afb08fe9a0fad5d9"

	body, err := json.Marshal(payload)
	if err != nil {
		return LiveInfo{}, fmt.Errorf("failed to marshal GQL request: %w", err)
	}

	// Create and send the HTTP request
	req, err := http.NewRequest("POST", "https://gql.twitch.tv/gql", bytes.NewBuffer(body))
	if err != nil {
		return LiveInfo{}, fmt.Errorf("failed to create GQL request: %w", err)
	}
	req.Header.Set("Client-ID", TwitchClientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return LiveInfo{}, fmt.Errorf("GQL request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LiveInfo{}, fmt.Errorf("GQL request returned non-200 status: %s", resp.Status)
	}

	// Decode the response
	var gqlResp gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return LiveInfo{}, fmt.Errorf("failed to decode GQL response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return LiveInfo{}, fmt.Errorf("GQL error: %s", gqlResp.Errors[0].Message)
	}

	// Check if the stream is live
	if gqlResp.Data.User.Stream != nil && gqlResp.Data.User.Stream.Type == "live" {
		return LiveInfo{
			IsLive:  true,
			VideoID: gqlResp.Data.User.Stream.ID,
			Title:   gqlResp.Data.User.Stream.Title,
		}, nil
	}

	return LiveInfo{IsLive: false}, nil
}
