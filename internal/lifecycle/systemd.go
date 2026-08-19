package lifecycle

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

func NotifyReady() error {
	_, err := daemon.SdNotify(false, "READY=1")
	return err
}

func NotifyWatchdog() error {
	_, err := daemon.SdNotify(false, "WATCHDOG=1")
	return err
}

func WatchdogInterval() (time.Duration, bool) {
	return daemon.SdWatchdogEnabled()
}

func StartWatchdog(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = NotifyWatchdog()
		}
	}
}