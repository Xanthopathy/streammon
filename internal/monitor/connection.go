package monitor

import (
	"fmt"
	"sync"
	"time"

	"streammon/internal/config"
	"streammon/internal/util"
)

// GlobalConnectionMonitor is a singleton that manages connection state for all monitors.
// It runs one connection checker and notifies all subscribed monitors of state changes.
var (
	globalConnMonitor *ConnectionMonitor
	connMonitorOnce   sync.Once
)

// ConnectionMonitor manages internet connectivity and notifies subscribers of state changes.
type ConnectionMonitor struct {
	mu                 sync.RWMutex
	isConnected        bool
	lastLogged         bool                 // Track last logged state to prevent duplicate logs
	subscribers        map[*sync.Cond]bool  // Map of condition variables to notify
	globalCfg          *config.GlobalConfig // Needed for logging
	sysLogger          *util.Logger
	stateChangedAt     time.Time
	consecutiveSuccess int
	consecutiveFailure int
	lastFailureLogTime time.Time
	lastSuccessLogTime time.Time
	checkTrigger       chan struct{} // Channel to trigger immediate connection checks
}

// GetGlobalConnectionMonitor returns the singleton connection monitor instance.
func GetGlobalConnectionMonitor(globalCfg *config.GlobalConfig) *ConnectionMonitor {
	connMonitorOnce.Do(func() {
		globalConnMonitor = &ConnectionMonitor{
			isConnected:    true,
			lastLogged:     true,
			subscribers:    make(map[*sync.Cond]bool),
			globalCfg:      globalCfg,
			sysLogger:      util.NewLogger(globalCfg, "System", util.ColorCyan),
			stateChangedAt: time.Now(),
			checkTrigger:   make(chan struct{}, 1),
		}
		// Start the background connection monitoring
		go globalConnMonitor.run()
	})
	return globalConnMonitor
}

// Subscribe adds a condition variable to be notified of connection state changes.
func (cm *ConnectionMonitor) Subscribe(cond *sync.Cond) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.subscribers[cond] = true
}

// Unsubscribe removes a condition variable from notifications.
func (cm *ConnectionMonitor) Unsubscribe(cond *sync.Cond) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.subscribers, cond)
}

// TriggerImmediateCheck requests an immediate connection check without waiting for the timer.
// Used by checker.go when network errors are detected.
func (cm *ConnectionMonitor) TriggerImmediateCheck() {
	select {
	case cm.checkTrigger <- struct{}{}:
	default:
		// Already triggered, skip
	}
}

// IsConnected returns the current connection state.
func (cm *ConnectionMonitor) IsConnected() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.isConnected
}

// broadcastStateChange notifies all subscribers and handles logging.
func (cm *ConnectionMonitor) broadcastStateChange() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.isConnected && !cm.lastLogged {
		// Connection just restored
		cm.sysLogger.Logf("%sConnection restored (stable).%s Resuming operations...", util.ColorGreen, util.ColorReset)
		cm.lastLogged = true
		cm.stateChangedAt = time.Now()
	} else if !cm.isConnected && cm.lastLogged {
		// Connection just lost
		cm.sysLogger.Logf("%sConnection lost (confirmed).%s Pausing monitors...", util.ColorRed, util.ColorReset)
		cm.lastLogged = false
		cm.stateChangedAt = time.Now()
	}

	// Notify all subscribers
	for cond := range cm.subscribers {
		cond.Broadcast()
	}
}

// run is the main connection monitoring loop (runs in background).
func (cm *ConnectionMonitor) run() {
	normalInterval := 10 * time.Second
	recoveryInterval := 5 * time.Second
	const threshold = 3

	timer := time.NewTimer(normalInterval)
	defer timer.Stop()

	for {
		select {
		case <-cm.checkTrigger:
			// Immediate check requested (e.g., by checker.go when network errors occur)
			// Drain the timer so we don't double-check immediately after
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			// Periodic check
		}

		connected := util.CheckInternetConnection()

		cm.mu.Lock()
		if cm.isConnected {
			if connected {
				cm.consecutiveFailure = 0 // Reset failure count
			} else {
				cm.consecutiveFailure++
				// Log a warning on the first failure so the user knows why checks might be failing
				if cm.consecutiveFailure == 1 {
					cm.sysLogger.Warn("Connection check failed. Verifying stability...")
				}
				if cm.consecutiveFailure >= threshold {
					cm.isConnected = false
					cm.consecutiveSuccess = 0 // Reset success count for recovery
					cm.broadcastStateChange()
				}
			}
		} else {
			// Currently disconnected
			if connected {
				cm.consecutiveSuccess++
				cm.sysLogger.Debug("System", fmt.Sprintf("Connection check passed (%d/%d)...", cm.consecutiveSuccess, threshold))
				if cm.consecutiveSuccess >= threshold {
					cm.isConnected = true
					cm.consecutiveFailure = 0
					cm.broadcastStateChange()
				}
			} else {
				cm.consecutiveSuccess = 0 // Connection still flaky, reset success count
			}
		}

		currentState := cm.isConnected
		cm.mu.Unlock()

		if currentState {
			timer.Reset(normalInterval)
		} else {
			// Check more frequently when offline to resume quickly
			timer.Reset(recoveryInterval)
		}
	}
}
