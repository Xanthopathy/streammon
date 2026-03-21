package main

import (
	"fmt"
	"net/http"
	"time"

	"streammon/internal/config"
	"streammon/internal/monitor"
	"streammon/internal/util"
)

func main() {
	// 1. Setup dependencies
	// We use a longer timeout because the /live page can sometimes take a moment to redirect and load scripts
	client := &http.Client{Timeout: 30 * time.Second}

	// Use default config to setup the logger
	globalCfg := config.GetDefaultGlobalConfig()
	// Force enable API debug to see the logs generated inside CheckYouTubeViaLivePage
	globalCfg.YoutubeAPIVerboseDebug = true

	logger := util.NewLogger(globalCfg, "Test", util.ColorBlue)

	// 2. Define a channel to test
	// Replace this ID with a channel you know is currently LIVE to verify it works.
	// Example: Lofi Girl (24/7 stream) -> UCSJ4gkVC6NrvII8umztf0Ow
	testChannelID := "UCZLZ8Jjx_RN2CXloOmgTHVg" // "UCSJ4gkVC6NrvII8umztf0Ow"
	testChannelName := "Kaela"                  // "Lofi Girl"

	fmt.Printf("Testing CheckYouTubeViaLivePage for %s (%s)...\n", testChannelName, testChannelID)
	fmt.Println("---------------------------------------------------")

	// 3. Call the function directly
	info, err := monitor.CheckYouTubeViaLivePage(client, testChannelID, testChannelName, logger)

	fmt.Println("---------------------------------------------------")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Result:\n")
	fmt.Printf("  IsLive:    %v\n", info.IsLive)
	fmt.Printf("  VideoID:   %s\n", info.VideoID)
	fmt.Printf("  Title:     %s\n", info.Title)
}
