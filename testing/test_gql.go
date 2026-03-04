package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
)

func main() {
	url := "https://gql.twitch.tv/gql"
	// This is the "Flattened" JSON payload you found
	payload := []byte(`[{"operationName":"StreamMetadata","variables":{"channelLogin":"pippa","includeIsDJ":true},"extensions":{"persistedQuery":{"version":1,"sha256Hash":"b57f9b910f8cd1a4659d894fe7550ccc81ec9052c01e438b290fd66a040b9b93"}}}]`)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	
	// Set the headers exactly like the browser/curl
	req.Header.Set("Client-Id", "kimne78kx3ncx6brgo4mv6wki5h1ko")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// Direct pipe to Stdout so you can see the raw JSON
	io.Copy(os.Stdout, resp.Body)
}