package notifier

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"gdrive-bisync/internal/services/logger"
)

type Sender func(ctx context.Context, urgency, title, message string) error

type Notifier struct {
	enabled  bool
	cooldown time.Duration
	send     Sender
	now      func() time.Time

	mu           sync.Mutex
	lastSent     map[string]time.Time
	activeErrors map[string]bool
}

func New(enabled bool, cooldown time.Duration) *Notifier {
	return newWithSender(enabled, cooldown, sendDesktopNotification, time.Now)
}

func newWithSender(enabled bool, cooldown time.Duration, sender Sender, now func() time.Time) *Notifier {
	return &Notifier{enabled: enabled, cooldown: cooldown, send: sender, now: now, lastSent: make(map[string]time.Time), activeErrors: make(map[string]bool)}
}

func (notifier *Notifier) Critical(key, title, message string) {
	if notifier == nil || !notifier.enabled {
		return
	}
	notifier.mu.Lock()
	now := notifier.now()
	last, seen := notifier.lastSent[key]
	if seen && now.Sub(last) < notifier.cooldown {
		notifier.activeErrors[key] = true
		notifier.mu.Unlock()
		return
	}
	notifier.lastSent[key] = now
	notifier.activeErrors[key] = true
	notifier.mu.Unlock()
	notifier.deliver("critical", title, message)
}

func (notifier *Notifier) Recovered(key, message string) {
	if notifier == nil || !notifier.enabled {
		return
	}
	notifier.mu.Lock()
	if !notifier.activeErrors[key] {
		notifier.mu.Unlock()
		return
	}
	delete(notifier.activeErrors, key)
	delete(notifier.lastSent, key)
	notifier.mu.Unlock()
	notifier.deliver("normal", "Google Drive sync recovered", message)
}

func (notifier *Notifier) deliver(urgency, title, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := notifier.send(ctx, urgency, title, message); err != nil {
		logger.Debug("Desktop notification unavailable; error remains in logs and status", "error", err)
	}
}

func sendDesktopNotification(ctx context.Context, urgency, title, message string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("desktop notifications are unsupported on %s", runtime.GOOS)
	}
	return exec.CommandContext(ctx, "notify-send", "--app-name=gdrive-bisync", "--urgency="+urgency, title, message).Run()
}
