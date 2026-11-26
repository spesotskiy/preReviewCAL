package config

import "testing"

func TestFileNamingFormatAndParse(t *testing.T) {
	cfg := Default()

	cases := []struct {
		objType  string
		id       int
		name     string
		expected string
	}{
		{"Table", 32, "Item Ledger Entry", "t32 - Item Ledger Entry.txt"},
		{"Page", 38, "Item Ledger Entries", "p38 - Item Ledger Entries.txt"},
		{"XMLport", 50000, "Export Items", "x50000 - Export Items.txt"},
		{"Codeunit", 50010, "Sales Helper", "c50010 - Sales Helper.txt"},
		{"Report", 50020, "Inventory Valuation", "r50020 - Inventory Valuation.txt"},
		{"Query", 50030, "Open Entries", "q50030 - Open Entries.txt"},
		{"MenuSuite", 1090, "Company", "m1090 - Company.txt"},
	}

	for _, tc := range cases {
		got := cfg.FileNaming.FormatFilename(tc.objType, tc.id, tc.name)
		if got != tc.expected {
			t.Fatalf("FormatFilename(%s, %d, %s) = %q, want %q", tc.objType, tc.id, tc.name, got, tc.expected)
		}
		typ, id, name, ok := cfg.FileNaming.ParseFilename(got)
		if !ok {
			t.Fatalf("ParseFilename(%q) failed", got)
		}
		if typ != tc.objType || id != tc.id || name != tc.name {
			t.Fatalf("ParseFilename(%q) = (%q, %d, %q), want (%q, %d, %q)", got, typ, id, name, tc.objType, tc.id, tc.name)
		}
	}
}

func TestConfigPartialOverride(t *testing.T) {
	raw := []byte(`{
		"fileNaming": {
			"template": "{prefix}_{id}_{name}.txt",
			"typePrefixes": { "Table": "TAB" }
		},
		"rules": {
			"CAL005": { "enabled": false },
			"CAL010": { "suffix": "Loc" }
		}
	}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.IsRuleEnabled(RuleNoWith) {
		t.Fatalf("expected CAL005 disabled")
	}
	if !cfg.IsRuleEnabled(RuleUnusedLocalVar) {
		t.Fatalf("expected CAL001 enabled by default")
	}
	if cfg.LocalVarSuffix() != "Loc" {
		t.Fatalf("expected suffix Loc, got %q", cfg.LocalVarSuffix())
	}
	got := cfg.FileNaming.FormatFilename("Table", 32, "Item Ledger Entry")
	if got != "TAB_32_Item Ledger Entry.txt" {
		t.Fatalf("got %q", got)
	}
	if cfg.Diff.MaxOldFunctionLines != 50 {
		t.Fatalf("expected default MaxOldFunctionLines=50, got %d", cfg.Diff.MaxOldFunctionLines)
	}
}

func TestDiffConfigMaxOldFunctionLinesZero(t *testing.T) {
	cfg, err := Parse([]byte(`{ "diff": { "maxOldFunctionLines": 0 } }`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Diff.MaxOldFunctionLines != 0 {
		t.Fatalf("expected explicit 0 to override default, got %d", cfg.Diff.MaxOldFunctionLines)
	}
}
