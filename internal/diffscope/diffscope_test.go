package diffscope

import "testing"

func TestChangedLineNumbers(t *testing.T) {
	base := "BEGIN\n  A := 1;\n  B := 2;\n  C := 3;\nEND;"
	curr := "BEGIN\n  A := 1;\n  NewLine := 99;\n  B := 2;\n  C := 4;\nEND;"

	got := ChangedLineNumbers(base, curr)
	if !got[3] || !got[5] {
		t.Fatalf("expected lines 3 and 5 changed, got %+v", got)
	}
	if got[1] || got[2] || got[4] || got[6] {
		t.Fatalf("unexpected unchanged lines marked changed: %+v", got)
	}
}
