package appstate

import (
	"path/filepath"
	"testing"
)

func TestAcquireInstanceLockRejectsSecondOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer first.Close()

	second, err := AcquireInstanceLock(path)
	if err == nil {
		second.Close()
		t.Fatal("expected second lock acquisition to fail")
	}
}

func TestAcquireInstanceLockCanBeReacquiredAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first lock: %v", err)
	}

	second, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	second.Close()
}
