package logger

import "testing"

func TestCategoryColorIsStableAndDynamic(t *testing.T) {
	first := categoryColor("SYNC")
	if second := categoryColor("SYNC"); second != first {
		t.Fatalf("category color changed: %d != %d", second, first)
	}
	if first < 16 || first > 231 {
		t.Fatalf("category color outside terminal cube: %d", first)
	}
	if categoryColor("DRIVE") == first && categoryColor("STORE") == first {
		t.Fatal("unexpected three-way category color collision")
	}
}

func TestEventSinkReceivesDynamicCategoryAndFields(t *testing.T) {
	var category string
	var fields map[string]any
	SetEventSink(func(_ string, receivedCategory string, _ string, receivedFields map[string]any) {
		category = receivedCategory
		fields = receivedFields
	})
	defer SetEventSink(nil)
	InfoCategory("custom-category", "operation", "path", "file.txt")
	if category != "CUSTOM-CATEGORY" || fields["path"] != "file.txt" {
		t.Fatalf("sink category=%q fields=%v", category, fields)
	}
}
