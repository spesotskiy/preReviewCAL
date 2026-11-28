package config

import "testing"

func TestCommentsConfigDefaultsAndOverride(t *testing.T) {
	cfg := Default()
	if cfg.Comments.KeepCommentPrefix != DefaultKeepCommentPrefix {
		t.Errorf("keepCommentPrefix default = %q", cfg.Comments.KeepCommentPrefix)
	}
	marker, commented := cfg.Comments.Patterns()
	if marker == nil || commented == nil {
		t.Fatal("default patterns must compile")
	}
	if !marker.MatchString("    // LCG PSY NAV-46641 23.11.2026 <<") || !marker.MatchString("// LCG LEB STR26165 02.02.2022 >>") {
		t.Error("default codeMarkerPattern must match LCG markers")
	}
	if !marker.MatchString("// LCG IR STR14758 20.08.2020 <<") || !marker.MatchString("// LCG AL STR119 17.09.2008") {
		t.Error("default codeMarkerPattern must match 2-letter team codes and markers without direction")
	}
	if marker.MatchString("// some normal comment") {
		t.Error("normal comment must not match marker pattern")
	}
	for _, code := range []string{"OldVal := 1;", "IF Cond THEN", "DoCleanup();", "EXIT;", "Rec.FINDFIRST();"} {
		if !commented.MatchString(code) {
			t.Errorf("commentedCodePattern must match %q", code)
		}
	}
	for _, text := range []string{
		"Total per line",
		"CAL020: zero-arg calls including TODAY() get ()",
		"see manual for details",
	} {
		if commented.MatchString(text) {
			t.Errorf("commentedCodePattern must NOT match explanatory %q", text)
		}
	}

	custom, err := Parse([]byte(`{"comments":{"keepCommentPrefix":"!!","commentedCodePattern":"(?i)^TODO-CODE"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if custom.Comments.KeepCommentPrefix != "!!" {
		t.Errorf("keepCommentPrefix override = %q", custom.Comments.KeepCommentPrefix)
	}
	if custom.Comments.CodeMarkerPattern != DefaultCodeMarkerPattern {
		t.Error("unset codeMarkerPattern must keep default")
	}
	if m, c := custom.Comments.Patterns(); m == nil || c == nil {
		t.Error("overridden patterns must compile")
	}

	if _, err := Parse([]byte(`{"comments":{"codeMarkerPattern":"(["}}`)); err == nil {
		t.Error("invalid codeMarkerPattern must produce a config error")
	}
}
