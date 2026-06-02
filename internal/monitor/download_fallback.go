package monitor

import (
	"fmt"
	"os"
	"time"

	"streammon/internal/config"
	"streammon/internal/util/ansi"
	"streammon/internal/util/logging"
	"streammon/internal/util/text"
)

func (b *BaseMonitor) startFallbackDownload(ch config.Channel, proc *downloadProcess) bool {
	if proc.fallbackAttempted {
		return false
	}
	controller, ok := b.controller.(FallbackDownloaderController)
	if !ok {
		return false
	}

	cmd, downloaderName, enabled := controller.BuildFallbackDownloaderCmd(ch, proc.status)
	if !enabled || cmd == nil {
		return false
	}
	proc.fallbackAttempted = true

	cmd.Dir = proc.cmd.Dir
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1", "TERM=xterm-256color")
	if b.controller.GetGlobalConfig().SaveDownloadLogs || b.controller.GetGlobalConfig().YoutubeDlpVerboseDebug {
		if stdoutPipe, err := cmd.StdoutPipe(); err == nil && stdoutPipe != nil {
			go logging.ReadPipeAndLog(stdoutPipe, proc.logger, downloaderName, proc.outputCallback)
		}
		if stderrPipe, err := cmd.StderrPipe(); err == nil && stderrPipe != nil {
			go logging.ReadPipeAndLog(stderrPipe, proc.logger, downloaderName, proc.outputCallback)
		}
	}

	commandStr := cmd.Path
	if len(cmd.Args) > 1 {
		commandStr += " " + text.JoinCommandArgs(cmd.Args[1:])
	}
	proc.logger.LogRegular(fmt.Sprintf("yt-dlp failed for %s%s%s. Trying livestream_dl fallback.", ansi.ColorOrange, ch.Name, ansi.ColorReset))
	proc.logger.LogSubprocessOutput("COMMAND: "+commandStr, downloaderName)

	if err := cmd.Start(); err != nil {
		proc.logger.LogError(fmt.Sprintf("Error starting livestream_dl fallback for %s%s%s: %v", ansi.ColorOrange, ch.Name, ansi.ColorReset, err))
		return false
	}

	proc.cmd = cmd
	proc.downloaderName = downloaderName
	proc.startedAt = time.Now()
	proc.mergerDetected.Store(false)
	proc.downloadCompleted.Store(false)
	return true
}
