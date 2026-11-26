package rules

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/rules -> repo root
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func TestGoldenCases(t *testing.T) {
	root := repoRoot(t)
	cfg := config.Default()
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)

	casesDir := filepath.Join(root, "testdata", "cases")
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatalf("read testdata/cases: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no golden test cases found")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		caseName := entry.Name()
		t.Run(caseName, func(t *testing.T) {
			inputDir := filepath.Join(casesDir, caseName, "input")
			expectedDir := filepath.Join(casesDir, caseName, "expected")

			inFiles, err := os.ReadDir(inputDir)
			if err != nil {
				t.Fatalf("read input dir: %v", err)
			}
			for _, f := range inFiles {
				if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".txt") {
					continue
				}
				inBytes, err := os.ReadFile(filepath.Join(inputDir, f.Name()))
				if err != nil {
					t.Fatal(err)
				}
				expBytes, err := os.ReadFile(filepath.Join(expectedDir, f.Name()))
				if err != nil {
					t.Fatal(err)
				}

				// 1. Validate filename matches configured naming template.
				obj, err := objectfile.Parse(inBytes)
				if err != nil {
					t.Fatalf("Parse(%s): %v", f.Name(), err)
				}
				wantFileName := cfg.FileNaming.FormatFilename(obj.ObjectType, obj.ObjectID, obj.ObjectName)
				if f.Name() != wantFileName {
					t.Errorf("test file %q does not match configured naming rule %q", f.Name(), wantFileName)
				}

				// 2. Run all enabled rules and compare byte-for-byte with expected.
				res, err := Process(inBytes, cfg, ws)
				if err != nil {
					t.Fatalf("Process(%s): %v", f.Name(), err)
				}
				if !bytes.Equal(res.Output, expBytes) {
					t.Fatalf("golden mismatch for %s/%s:\n--- GOT ---\n%s\n--- WANT ---\n%s",
						caseName, f.Name(), string(res.Output), string(expBytes))
				}

				// 3. Idempotency: running again on expected must produce 0 byte changes.
				res2, err := Process(expBytes, cfg, ws)
				if err != nil {
					t.Fatalf("idempotency Process(%s): %v", f.Name(), err)
				}
				if res2.Changed || !bytes.Equal(res2.Output, expBytes) {
					t.Fatalf("not idempotent for %s/%s:\n--- SECOND RUN ---\n%s",
						caseName, f.Name(), string(res2.Output))
				}

				// 4. Lossless round-trip: running with all rules disabled produces 0 byte changes.
				resOff, err := Process(inBytes, config.AllDisabled(), ws)
				if err != nil {
					t.Fatalf("round-trip Process(%s): %v", f.Name(), err)
				}
				if resOff.Changed || !bytes.Equal(resOff.Output, inBytes) {
					t.Fatalf("all-disabled round-trip modified %s/%s", caseName, f.Name())
				}

				// 5. CRLF preservation test: convert input & expected to CRLF and verify exact match!
				inCRLF := bytes.ReplaceAll(bytes.ReplaceAll(inBytes, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
				expCRLF := bytes.ReplaceAll(bytes.ReplaceAll(expBytes, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
				resCRLF, err := Process(inCRLF, cfg, ws)
				if err != nil {
					t.Fatalf("CRLF Process(%s): %v", f.Name(), err)
				}
				if !bytes.Equal(resCRLF.Output, expCRLF) {
					t.Fatalf("CRLF mismatch for %s/%s", caseName, f.Name())
				}
			}
		})
	}
}

func TestDiagnosticWarnings(t *testing.T) {
	root := repoRoot(t)
	cfg := config.Default()
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)

	// CAL003: unused parameter emits warning without editing.
	t.Run("CAL003_warning", func(t *testing.T) {
		p := filepath.Join(root, "testdata", "cases", "cal003_unused_param", "input", "c50003 - Unused Param.txt")
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Process(data, cfg, ws)
		if err != nil {
			t.Fatal(err)
		}
		if res.Changed {
			t.Fatalf("CAL003 should not modify file")
		}
		if !hasUnfixedDiagnostic(res.Diagnostics, config.RuleUnusedParam) {
			t.Fatalf("expected CAL003 warning, got %+v", res.Diagnostics)
		}
	})

	// CAL011: EventSubscriber parameter emits warning without auto-fixing.
	t.Run("CAL011_EventSubscriber_warning", func(t *testing.T) {
		p := filepath.Join(root, "testdata", "cases", "cal011_param_suffix", "input", "c50011 - Param Suffix And EventSub.txt")
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Process(data, cfg, ws)
		if err != nil {
			t.Fatal(err)
		}
		if !hasUnfixedDiagnostic(res.Diagnostics, config.RuleParamSuffix) {
			t.Fatalf("expected CAL011 warning for EventSubscriber, got %+v", res.Diagnostics)
		}
	})

	// CAL010: >30 char variable emits warning without truncating/renaming.
	t.Run("CAL010_max_30_char_warning", func(t *testing.T) {
		p := filepath.Join(root, "testdata", "cases", "cal010_local_suffix", "input", "c50010 - Local Suffix.txt")
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Process(data, cfg, ws)
		if err != nil {
			t.Fatal(err)
		}
		if !hasUnfixedDiagnostic(res.Diagnostics, config.RuleLocalVarSuffix) {
			t.Fatalf("expected CAL010 warning for 30-char limit, got %+v", res.Diagnostics)
		}
	})
}

func TestConfigToggleDisablesSpecificRules(t *testing.T) {
	root := repoRoot(t)
	p := filepath.Join(root, "testdata", "cases", "all_rules_codeunit", "input", "c50100 - All Rules Combined.txt")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// Disable CAL005 (no-with), CAL010 (local L suffix), CAL020 (call parens).
	cfg, err := config.Parse([]byte(`{
		"rules": {
			"CAL005": { "enabled": false },
			"CAL010": { "enabled": false },
			"CAL020": { "enabled": false }
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)
	res, err := Process(data, cfg, ws)
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Output)

	// WITH must still be present because CAL005 is disabled.
	if !strings.Contains(out, "WITH ItemLedgEntry DO BEGIN") {
		t.Errorf("expected WITH preserved when CAL005 disabled, got:\n%s", out)
	}
	// Local vars must NOT have L suffix because CAL010 is disabled.
	if strings.Contains(out, "TotalQtyL") {
		t.Errorf("expected TotalQty not renamed to TotalQtyL when CAL010 disabled")
	}
	// TODAY must NOT have () added because CAL020 is disabled.
	if strings.Contains(out, "TODAY()") {
		t.Errorf("expected TODAY without () when CAL020 disabled")
	}
	// Enabled rules like CAL004 (TextConst -> global) and CAL011 (ItemNo -> ItemNoP) MUST still apply!
	if !strings.Contains(out, "ItemNoP@1000") {
		t.Errorf("expected enabled CAL011 to rename ItemNo -> ItemNoP")
	}
	if !strings.Contains(out, "ConfirmMsg@1002") {
		t.Errorf("expected enabled CAL004 to promote ConfirmMsg to global VAR")
	}
}

func hasUnfixedDiagnostic(diags []Diagnostic, rule string) bool {
	for _, d := range diags {
		if d.Rule == rule && !d.Fixed {
			return true
		}
	}
	return false
}
