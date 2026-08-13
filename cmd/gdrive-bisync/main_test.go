package main

import "testing"

func TestShouldOpenTUIForInteractiveCommandWithoutFlags(t *testing.T) {
	if !shouldOpenTUI(0, true) {
		t.Fatal("interactive invocation without flags should open TUI")
	}
	if shouldOpenTUI(1, true) {
		t.Fatal("explicit CLI flag should retain CLI behavior")
	}
	if shouldOpenTUI(0, false) {
		t.Fatal("non-interactive service invocation should run daemon")
	}
}
