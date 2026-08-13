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
