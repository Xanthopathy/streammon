package monitor

import (
	"net/http"
	"sync"
	"time"

	"streammon/internal/util"
)

// BaseMonitor provides the generic, shared functionality for monitoring any platform.
type BaseMonitor struct {
	logger                    *util.Logger
	controller                MonitorController
	httpClient                *http.Client
	statusMutex               sync.RWMutex
	downloadMutex             sync.Mutex
	liveStatus                map[string]LiveInfo         // map[channelID]LiveInfo
	activeDownloads           map[string]*downloadProcess // map[channelID]*downloadProcess
	downloadedVideos          map[string]map[string]bool  // map[channelID]map[videoID]bool - in-memory cache of downloaded videos
	downloadedVidMu           sync.RWMutex                // protects downloadedVideos
	queuedVideosLogged        map[string]bool             // map[videoID]bool - tracks which queued videos have logged the "already queued" message
	queuedVideosLoggedMutex   sync.Mutex                  // protects queuedVideosLogged
	downloadedVidsLogged      map[string]bool             // map[videoID]bool - tracks which downloaded videos have logged the "already downloaded" message
	downloadedVidsLoggedMutex sync.Mutex                  // protects downloadedVidsLogged
	archivedVideos            map[string]bool             // map[videoID]bool - loaded from archive.txt
	archivedVidMu             sync.RWMutex                // protects archivedVideos
}

// NewBaseMonitor creates a new generic monitor.
func NewBaseMonitor(controller MonitorController) *BaseMonitor {
	return &BaseMonitor{
		logger:               util.NewLogger(controller.GetGlobalConfig(), controller.GetLogPrefix(), controller.GetLogColor()),
		controller:           controller,
		httpClient:           &http.Client{Timeout: 30 * time.Second},
		liveStatus:           make(map[string]LiveInfo),
		activeDownloads:      make(map[string]*downloadProcess),
		downloadedVideos:     make(map[string]map[string]bool),
		queuedVideosLogged:   make(map[string]bool),
		downloadedVidsLogged: make(map[string]bool),
		archivedVideos:       make(map[string]bool),
	}
}
