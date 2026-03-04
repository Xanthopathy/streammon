package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// TwitchClientID is a public, hardcoded client ID for the Twitch GQL API.
// This is a common practice for third-party tools.
const TwitchClientID = "kimne78kx3ncx6brgo4mv6wki5h1ko"

// --- GQL Request and Response Structures ---

// streamMetadataGQLRequest is the payload for the StreamMetadata GQL query.
// We send a slice of this struct as the body.
type streamMetadataGQLRequest struct {
	OperationName string `json:"operationName"`
	Variables     struct {
		ChannelLogin string `json:"channelLogin"`
		IncludeIsDJ  bool   `json:"includeIsDJ"`
	} `json:"variables"`
	Extensions struct {
		PersistedQuery struct {
			Version    int    `json:"version"`
			SHA256Hash string `json:"sha256Hash"`
		} `json:"persistedQuery"`
	} `json:"extensions"`
}

// streamMetadataGQLResponse is the structure for the data returned by the StreamMetadata query.
// The response is a slice of these objects.
type streamMetadataGQLResponse []struct {
	Data struct {
		User *struct {
			LastBroadcast *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"lastBroadcast"`
			Stream *struct {
				ID        string    `json:"id"`
				Type      string    `json:"type"`
				CreatedAt time.Time `json:"createdAt"`
			} `json:"stream"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// --- API Function ---

// CheckLiveGQL performs a lightweight check to see if a Twitch channel is live using the StreamMetadata query.
func CheckLiveGQL(httpClient *http.Client, channelLogin string, globalCfg *config.GlobalConfig) (LiveInfo, error) {
	// Construct the GQL request payload
	payload := streamMetadataGQLRequest{
		OperationName: "StreamMetadata",
	}
	payload.Variables.ChannelLogin = strings.ToLower(channelLogin)
	payload.Variables.IncludeIsDJ = true // As seen in test query
	payload.Extensions.PersistedQuery.Version = 1
	payload.Extensions.PersistedQuery.SHA256Hash = "b57f9b910f8cd1a4659d894fe7550ccc81ec9052c01e438b290fd66a040b9b93"

	// The API expects a JSON array containing the request object
	body, err := json.Marshal([]streamMetadataGQLRequest{payload})
	if err != nil {
		return LiveInfo{}, fmt.Errorf("failed to marshal GQL request: %w", err)
	}
	util.DebugLog(globalCfg, "TwitchAPI", fmt.Sprintf("Requesting for %s with payload: %s", channelLogin, string(body)))

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

	// Read the body to allow for logging and decoding
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return LiveInfo{}, fmt.Errorf("failed to read GQL response body: %w", err)
	}
	defer resp.Body.Close()

	util.DebugLog(globalCfg, "TwitchAPI", fmt.Sprintf("Raw response for %s: %s", channelLogin, string(responseBody)))

	// Decode the response
	var gqlResp streamMetadataGQLResponse
	if err := json.Unmarshal(responseBody, &gqlResp); err != nil {
		return LiveInfo{}, fmt.Errorf("failed to decode GQL response: %w", err)
	}

	// Basic validation of the response structure
	if len(gqlResp) == 0 {
		return LiveInfo{}, fmt.Errorf("GQL response was an empty array")
	}
	if len(gqlResp[0].Errors) > 0 {
		return LiveInfo{}, fmt.Errorf("GQL error: %s", gqlResp[0].Errors[0].Message)
	}
	if gqlResp[0].Data.User == nil {
		// This can happen for suspended or non-existent channels.
		return LiveInfo{IsLive: false}, nil
	}

	user := gqlResp[0].Data.User
	stream := user.Stream
	lastBroadcast := user.LastBroadcast

	// Prepare the result, starting with data that's always present
	info := LiveInfo{IsLive: false}
	if lastBroadcast != nil {
		info.Title = lastBroadcast.Title
		info.LastBroadcastID = lastBroadcast.ID
	}

	// If the stream object exists and is 'live', update the status
	if stream != nil && stream.Type == "live" {
		info.IsLive = true
		info.VideoID = stream.ID
		info.CreatedAt = stream.CreatedAt

		// Sanity check: if lastBroadcast.id and stream.id don't match, something is weird.
		// The live stream ID should take precedence.
		if lastBroadcast != nil && stream.ID != lastBroadcast.ID {
			util.DebugLog(globalCfg, "TwitchAPI", fmt.Sprintf("Stream ID (%s) and LastBroadcast ID (%s) mismatch for %s", stream.ID, lastBroadcast.ID, channelLogin))
		}
	}

	// Fallback for title if lastBroadcast was missing for some reason
	if info.Title == "" {
		info.Title = "Twitch Stream"
	}

	return info, nil
}
