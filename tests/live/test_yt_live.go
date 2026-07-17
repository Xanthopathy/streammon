package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"streammon/internal/config"
	"streammon/internal/scrapers/youtube"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0"

func main() {
	configPath := flag.String("config", "tests/live/streammon_config_yt.toml", "path to the YouTube test config file")
	outDir := flag.String("out", "tests/live/data", "directory to write collected /live artifacts")
	timeout := flag.Duration("timeout", 30*time.Second, "HTTP request timeout")
	checkLive := flag.Bool("check-live", true, "run the repo's /live detection logic and save results")
	flag.Parse()

	cfg, err := config.LoadYTConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	if len(cfg.Channels) == 0 {
		fmt.Fprintf(os.Stderr, "no channels found in %s\n", *configPath)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output dir %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: *timeout}
	globalCfg := config.GetDefaultGlobalConfig()
	globalCfg.YoutubeAPIVerboseDebug = true
	logger := logging.NewLogger(globalCfg, "LiveTest", ansi.ColorBlue)

	fmt.Printf("Loaded %d channels from %s\n", len(cfg.Channels), *configPath)
	fmt.Printf("Using /live check method for each configured channel. Output directory: %s\n", *outDir)
	fmt.Println("---------------------------------------------------")

	for _, ch := range cfg.Channels {
		fmt.Printf("Channel: %s (%s)\n", ch.Name, ch.ID)
		url := fmt.Sprintf("https://www.youtube.com/channel/%s/live", ch.ID)
		req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
		if err != nil {
			fmt.Printf("  failed to create request: %v\n", err)
			continue
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  http error: %v\n", err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("  failed to read body: %v\n", err)
			continue
		}

		rawPath := filepath.Join(*outDir, fmt.Sprintf("%s.live.html", ch.ID))
		if err := os.WriteFile(rawPath, body, 0o644); err != nil {
			fmt.Printf("  failed to write raw HTML: %v\n", err)
			continue
		}
		fmt.Printf("  saved raw HTML to %s\n", rawPath)

		if *checkLive {
			info, err := youtube.CheckYouTubeViaLivePage(context.Background(), client, ch.ID, ch.Name, logger)
			if err != nil {
				fmt.Printf("  /live detection error: %v\n", err)
			} else {
				jsonPath := filepath.Join(*outDir, fmt.Sprintf("%s.livecheck.json", ch.ID))
				data, _ := json.MarshalIndent(info, "", "  ")
				if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
					fmt.Printf("  failed to write live-check JSON: %v\n", err)
				} else {
					fmt.Printf("  saved /live detection result to %s\n", jsonPath)
				}
				fmt.Printf("  is_live=%v video_id=%q title=%q\n", info.IsLive, info.VideoID, info.Title)
				if !info.IsLive {
					fmt.Println("  -> current /live scraper would miss this channel in this state")
				}
			}
		}

		fmt.Println()
	}
}
