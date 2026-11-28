package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFixCheckAndDiff(t *testing.T) {
	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))

	// Copy symbols Table 32 and a dirty Codeunit into tmp.
	tabBytes, err := os.ReadFile(filepath.Join(root, "testdata", "symbols", "t32 - Item Ledger Entry.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "t32 - Item Ledger Entry.txt"), tabBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	inBytes, err := os.ReadFile(filepath.Join(root, "testdata", "cases", "cal005_no_with", "input", "c50005 - No With.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expBytes, err := os.ReadFile(filepath.Join(root, "testdata", "cases", "cal005_no_with", "expected", "c50005 - No With.txt"))
	if err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(tmp, "c50005 - No With.txt")
	if err := os.WriteFile(targetFile, inBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Config with CAL005 disabled: --symbols-dir must not be required.
	noWithOffCfg := filepath.Join(tmp, "no-with-off.json")
	if err := os.WriteFile(noWithOffCfg, []byte(`{"rules":{"CAL005":{"enabled":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// 0. Task 1.3: without --symbols-dir (and without config symbolsDir) CAL005 must refuse to run.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"check", targetFile}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("check without --symbols-dir exit code = %d, want 2 (stderr=%s)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--symbols-dir is required") {
		t.Fatalf("expected clear --symbols-dir error, got stderr=%s", stderr.String())
	}

	// 1. check returns exit code 1 on dirty file.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"check", "--symbols-dir", tmp, targetFile}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("check exit code = %d, want 1 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
	}

	// 2. fix --diff prints unified diff without modifying file.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"fix", "--symbols-dir", tmp, "--diff", targetFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("fix --diff exit code = %d, want 0 (stderr=%s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "-      WITH ItemLedgEntryL DO BEGIN") {
		t.Fatalf("expected diff output, got:\n%s", stdout.String())
	}
	afterDryRun, _ := os.ReadFile(targetFile)
	if !bytes.Equal(afterDryRun, inBytes) {
		t.Fatalf("fix --diff must not modify file on disk")
	}

	// 3. fix updates file on disk to match expected.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"fix", "--symbols-dir", tmp, targetFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("fix exit code = %d, want 0 (stderr=%s)", code, stderr.String())
	}
	afterFix, _ := os.ReadFile(targetFile)
	if !bytes.Equal(afterFix, expBytes) {
		t.Fatalf("fixed file mismatch:\n--- GOT ---\n%s\n--- WANT ---\n%s", string(afterFix), string(expBytes))
	}

	// 4. check now returns exit code 0!
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"check", "--symbols-dir", tmp, targetFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("check after fix exit code = %d, want 0 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
	}

	// 5. CAL005 disabled in config: running WITHOUT --symbols-dir is allowed again.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"check", "--config", noWithOffCfg, targetFile}, &stdout, &stderr)
	if code == 2 {
		t.Fatalf("with CAL005 disabled, missing --symbols-dir must not abort (stderr=%s)", stderr.String())
	}
}
