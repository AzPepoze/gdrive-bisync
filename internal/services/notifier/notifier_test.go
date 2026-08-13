package notifier

import (
	"context"
	"testing"
	"time"
)

type sentNotification struct{ urgency, title, message string }

func TestCriticalDeduplicatesDuringCooldownAndRecoverySendsOnce(t *testing.T) {
	now := time.Unix(100, 0)
	var sent []sentNotification
	n := newWithSender(true, 30*time.Minute, func(_ context.Context, urgency, title, message string) error {
		sent = append(sent, sentNotification{urgency, title, message})
		return nil
	}, func() time.Time { return now })

	n.Critical("sync:network", "Sync failed", "network down")
	n.Critical("sync:network", "Sync failed", "network down")
	if len(sent) != 1 {
		t.Fatalf("expected one deduplicated notification, got %d", len(sent))
	}
	n.Recovered("sync:network", "Sync is healthy again.")
	n.Recovered("sync:network", "Sync is healthy again.")
	if len(sent) != 2 || sent[1].urgency != "normal" {
		t.Fatalf("expected one recovery notification, got %#v", sent)
	}
}

func TestCriticalAllowsDifferentKeysAndExpiredCooldown(t *testing.T) {
	now := time.Unix(100, 0)
	count := 0
	n := newWithSender(true, time.Minute, func(context.Context, string, string, string) error { count++; return nil }, func() time.Time { return now })
	n.Critical("auth", "Auth", "failed")
	n.Critical("database", "Database", "failed")
	now = now.Add(time.Minute)
	n.Critical("auth", "Auth", "failed")
	if count != 3 {
		t.Fatalf("expected three notifications, got %d", count)
	}
}

func TestDisabledNotifierDoesNothing(t *testing.T) {
	count := 0
	n := newWithSender(false, 0, func(context.Context, string, string, string) error { count++; return nil }, time.Now)
	n.Critical("error", "Error", "message")
	n.Recovered("error", "healthy")
	if count != 0 {
		t.Fatalf("disabled notifier sent %d notifications", count)
	}
}

func TestRecoveryOnlyClearsMatchingFailureCategory(t *testing.T) {
	count := 0
	n := newWithSender(true, time.Hour, func(context.Context, string, string, string) error { count++; return nil }, time.Now)
	n.Critical("watcher", "Watcher", "failed")
	n.Recovered("sync", "sync healthy")
	if count != 1 {
		t.Fatalf("unrelated recovery sent a notification; count=%d", count)
	}
	n.Recovered("watcher", "watcher healthy")
	if count != 2 {
		t.Fatalf("matching recovery was not sent; count=%d", count)
	}
}
