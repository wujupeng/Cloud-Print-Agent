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
	d, err := daemon.SdWatchdogEnabled(false)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
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