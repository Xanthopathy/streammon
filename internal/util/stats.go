package util

import (
	"fmt"
	"sync"
)

// FallbackStats tracks channels that required a fallback method.
// It provides a thread-safe way to accumulate failures and log them.
type FallbackStats struct {
	mu             sync.Mutex
	failedChannels []string
}

// Add records a channel that failed its primary check and swapped to a fallback.
func (s *FallbackStats) Add(channelName, fallbackMethod string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedChannels = append(s.failedChannels, fmt.Sprintf("%s (->%s)", channelName, fallbackMethod))
}

// LogAndReset prints the stats to the provided logger and clears the internal list.
func (s *FallbackStats) LogAndReset(logger *Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.failedChannels) > 0 {
		logger.LogRegular(fmt.Sprintf("%sFallback Report:%s %d channels failed primary check and swapped methods: %v",
			ColorYellow, ColorReset, len(s.failedChannels), s.failedChannels))

		// Reset stats for next loop
		s.failedChannels = nil
	}
}
