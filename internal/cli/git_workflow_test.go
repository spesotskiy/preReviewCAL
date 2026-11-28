package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGitWorkflowsThreeModes(t *testing.T) {
	repo := t.TempDir()

	// Initialize temporary git repo on 'main'.
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "dev@example.com")
	runGitCmd(t, repo, "config", "user.name", "NAV Dev")
	runGitCmd(t, repo, "config", "core.autocrlf", "false")

	// Config with maxOldFunctionLines = 8 for clear small vs large function separation.
	writeFile(t, filepath.Join(repo, ".prereviewcal.json"), `{
  "diff": { "maxOldFunctionLines": 8 }
}
`)

	// Commit 1 (main): Table 32 + legacy Codeunit with untouched, small, and large procedures.
	t32Path := filepath.Join(repo, "t32 - Item Ledger Entry.txt")
	writeFile(t, t32Path, `OBJECT Table 32 Item Ledger Entry
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
  }
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        }
    { 2   ;   ;Item No.            ;Code20         }
    { 13  ;   ;Quantity            ;Decimal        }
  }
  CODE
  {
    BEGIN
    END.
  }
}
`)

	c50200Path := filepath.Join(repo, "c50200 - Legacy Service.txt")
	writeFile(t, c50200Path, `OBJECT Codeunit 50200 Legacy Service
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
  }
  CODE
  {
    VAR
      StaleGlobal@1000 : Integer;
      UsedGlobal@1001 : Integer;

    procedure UntouchedDirty@1(Arg@1000 : Integer);
    var
      LegacyVar@1001 : Integer;
    begin
      legacyvar:=arg+1;;
    end;

    procedure SmallLegacy@2(Qty@1000 : Integer);
    begin
      UsedGlobal:=qty;
    end;

    procedure LargeLegacy@3(BaseVal@1000 : Integer);
    var
      Acc@1001 : Integer;
    begin
      acc:=baseval;
      acc:=acc+1;
      acc:=acc+2;
      acc:=acc+3;
      acc:=acc+4;
      acc:=acc+5;
      acc:=acc+6;
      UsedGlobal:=acc;
    end;

    BEGIN
    END.
  }
}
`)

	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "Commit 1: initial legacy objects on main")

	// Start feature branch from main.
	runGitCmd(t, repo, "checkout", "-b", "feature/clean-nav")

	// =========================================================================
	// WORKFLOW 1: During development — compare staged changes against HEAD
	// =========================================================================
	t.Run("Workflow1_StagedPreCommit", func(t *testing.T) {
		// Modify LargeLegacy (add 1 dirty line + 1 new local var) and stage it.
		writeFile(t, c50200Path, `OBJECT Codeunit 50200 Legacy Service
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
  }
  CODE
  {
    VAR
      StaleGlobal@1000 : Integer;
      UsedGlobal@1001 : Integer;

    procedure UntouchedDirty@1(Arg@1000 : Integer);
    var
      LegacyVar@1001 : Integer;
    begin
      legacyvar:=arg+1;;
    end;

    procedure SmallLegacy@2(Qty@1000 : Integer);
    begin
      UsedGlobal:=qty;
    end;

    procedure LargeLegacy@3(BaseVal@1000 : Integer);
    var
      Acc@1001 : Integer;
      StepDate@1002 : Date;
    begin
      acc:=baseval;
      acc:=acc+1;
      acc:=acc+2;
      stepdate:=today;
      acc:=acc+3;
      acc:=acc+4;
      acc:=acc+5;
      acc:=acc+6;
      UsedGlobal:=acc;
    end;

    BEGIN
    END.
  }
}
`)
		runGitCmd(t, repo, "add", c50200Path)

		// 1a. check --staged detects dirty staged code and returns exit code 1.
		var stdout, stderr bytes.Buffer
		code := Run([]string{"check", "--symbols-dir", repo, "--staged", "--repo", repo}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("check --staged exit=%d want 1 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
		}

		// 1b. fix --staged cleans ONLY the staged modifications (+ global VAR trash) on disk.
		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"fix", "--symbols-dir", repo, "--staged", "--repo", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("fix --staged exit=%d want 0 (stderr=%s)", code, stderr.String())
		}

		got := readFile(t, c50200Path)
		if strings.Contains(got, "StaleGlobal") {
			t.Errorf("expected CAL002 to remove StaleGlobal")
		}
		if !strings.Contains(got, "legacyvar:=arg+1;;") {
			t.Errorf("expected UntouchedDirty to remain untouched:\n%s", got)
		}
		if !strings.Contains(got, "Acc@1001 : Integer;") || !strings.Contains(got, "acc:=baseval;") {
			t.Errorf("expected old lines of LargeLegacy to stay untouched:\n%s", got)
		}
		if !strings.Contains(got, "StepDateL@1002 : Date;") || !strings.Contains(got, "StepDateL := TODAY();") {
			t.Errorf("expected new var & line in LargeLegacy to be cleaned:\n%s", got)
		}

		// Re-stage cleaned file -> check --staged returns 0!
		runGitCmd(t, repo, "add", c50200Path)
		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"check", "--symbols-dir", repo, "--staged", "--repo", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("check --staged after fix exit=%d want 0 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
		}

		runGitCmd(t, repo, "commit", "-m", "Commit 2: cleaned partial change in LargeLegacy")
	})

	// =========================================================================
	// WORKFLOW 2: Pre-review polish — polish entire branch vs main
	// =========================================================================
	t.Run("Workflow2_PreReviewBranchPolish", func(t *testing.T) {
		// Commit 3: developer commits dirty changes in SmallLegacy + adds NewHelper + creates brand-new file c50202.
		curr := readFile(t, c50200Path)
		curr = strings.Replace(curr,
			"      UsedGlobal:=qty;\n    end;",
			"      UsedGlobal:=qty+1;\n    end;\n\n    procedure NewHelper@4(ItemNo@1000 : Code[20]);\n    var\n      Ledg@1001 : Record 32;\n    begin\n      with ledg do begin\n        setrange(\"Item No.\",itemno);\n        if findfirst then\n          message('%1',Quantity);\n      end;\n    end;",
			1)
		writeFile(t, c50200Path, curr)

		c50202Path := filepath.Join(repo, "c50202 - Brand New.txt")
		writeFile(t, c50202Path, `OBJECT Codeunit 50202 Brand New
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
  }
  CODE
  {
    procedure RunNew@1(Input@1000 : Integer);
    var
      Val@1001 : Integer;
      Unused@1002 : Text[10];
    begin
      val:=input+1;;
      message('%1',val);
    end;

    BEGIN
    END.
  }
}
`)
		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-m", "Commit 3: dirty branch commits before pre-review")

		// Polish entire branch against main in one command!
		var stdout, stderr bytes.Buffer
		code := Run([]string{"fix", "--symbols-dir", repo, "--git-base", "main", "--repo", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("fix --git-base main exit=%d want 0 (stderr=%s)", code, stderr.String())
		}

		got50200 := readFile(t, c50200Path)
		// UntouchedDirty STILL untouched!
		if !strings.Contains(got50200, "legacyvar:=arg+1;;") {
			t.Errorf("UntouchedDirty modified during branch polish:\n%s", got50200)
		}
		// SmallLegacy (<=8 lines) was touched -> wholly cleaned!
		if !strings.Contains(got50200, "PROCEDURE SmallLegacy@2(QtyP@1000 : Integer);") || !strings.Contains(got50200, "UsedGlobal := QtyP + 1;") {
			t.Errorf("SmallLegacy not wholly cleaned:\n%s", got50200)
		}
		// NewHelper was added -> wholly cleaned including WITH replacement!
		if !strings.Contains(got50200, "LedgL.SETRANGE(\"Item No.\", ItemNoP);") || !strings.Contains(got50200, "IF LedgL.FINDFIRST() THEN") {
			t.Errorf("NewHelper not wholly cleaned:\n%s", got50200)
		}

		// Brand-new file c50202 was completely cleaned!
		got50202 := readFile(t, c50202Path)
		if strings.Contains(got50202, "Unused@1002") || !strings.Contains(got50202, "PROCEDURE RunNew@1(InputP@1000 : Integer);") || !strings.Contains(got50202, "ValL := InputP + 1;") {
			t.Errorf("brand new file c50202 not fully cleaned:\n%s", got50202)
		}

		// Verify check --git-base main is now 100% green (exit code 0)!
		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"check", "--symbols-dir", repo, "--git-base", "main", "--repo", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("check --git-base main after polish exit=%d want 0 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
		}

		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-m", "Commit 4: polished branch ready for MR")
	})

	// =========================================================================
	// WORKFLOW 3: CI/CD Merge Request — check two commits without touching disk
	// =========================================================================
	t.Run("Workflow3_CICD_MergeRequestCheck", func(t *testing.T) {
		// 3a. Clean MR branch (feature/clean-nav vs main) passes CI check with exit 0!
		var stdout, stderr bytes.Buffer
		code := Run([]string{"check", "--symbols-dir", repo, "--git-base", "main", "--git-head", "feature/clean-nav", "--repo", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("CI check on clean MR exit=%d want 0 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
		}

		// 3b. Create a separate dirty MR branch from main and run CI check against it.
		runGitCmd(t, repo, "checkout", "-b", "feature/mr-dirty", "main")
		dirtyMR := readFile(t, c50200Path)
		dirtyMR = strings.Replace(dirtyMR,
			"    BEGIN\n    END.",
			"    procedure DirtyMRFunc@9(X@1000 : Integer);\n    var\n      Y@1001 : Integer;\n    begin\n      y:=x+1;;\n      message('%1',y);\n    end;\n\n    BEGIN\n    END.",
			1)
		writeFile(t, c50200Path, dirtyMR)
		runGitCmd(t, repo, "add", c50200Path)
		runGitCmd(t, repo, "commit", "-m", "Dirty MR commit")

		// Switch back to main so working tree doesn't even have the MR commits!
		runGitCmd(t, repo, "checkout", "main")
		mainDiskBefore := readFile(t, c50200Path)

		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"check", "--symbols-dir", repo, "--git-base", "main", "--git-head", "feature/mr-dirty", "--diff", "--repo", repo}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("CI check on dirty MR exit=%d want 1 (stdout=%s stderr=%s)", code, stdout.String(), stderr.String())
		}
		// Diff output shows fixes for DirtyMRFunc, NOT for UntouchedDirty!
		outStr := stdout.String()
		if !strings.Contains(outStr, "+    PROCEDURE DirtyMRFunc@9(XP@1000 : Integer);") {
			t.Errorf("expected CI diff to report DirtyMRFunc fix, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "-    procedure UntouchedDirty") || strings.Contains(outStr, "LegacyVar") {
			t.Errorf("CI check must NOT report untouched legacy procedure:\n%s", outStr)
		}
		// Working tree on disk must remain 100% untouched!
		if readFile(t, c50200Path) != mainDiskBefore {
			t.Fatalf("CI check modified working tree on disk!")
		}
	})
}
