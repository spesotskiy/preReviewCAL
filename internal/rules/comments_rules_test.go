package rules

import (
	"strings"
	"testing"

	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

func runCommentRules(body string, changed map[int]bool) (string, []Diagnostic) {
	cfg := config.Default()
	obj := &objectfile.ObjectFile{Raw: body}
	return applyCommentRules(body, changed, cfg, obj, 0)
}

func TestCommentsStandaloneBlockRemovedInlineKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  { legacy note }\r\n" + // standalone single-line block -> removed
		"  IF FlagL THEN\r\n" +
		"    Do(FALSE {OzonOnly});\r\n" + // inline block -> kept untouched
		"  {\r\n" +
		"    multi\r\n" +
		"    line\r\n" +
		"  }\r\n" + // standalone multi-line block -> removed
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  IF FlagL THEN\r\n" +
		"    Do(FALSE {OzonOnly});\r\n" +
		"END;\r\n"
	got, diags := runCommentRules(in, nil)
	if got != want {
		t.Fatalf("block comments\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "removed 1 commented-out") && !hasDiag(diags, SeverityInfo, "standalone block comment") {
		t.Errorf("expected block removal diagnostic, got %+v", diags)
	}
}

func TestCommentsCodeLinesRemovedExplanatoryKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // OldVal := 1;\r\n" + // removed (:=)
		"  // DoCleanup();\r\n" + // removed (call line, ;)
		"  // IF Cond THEN\r\n" + // removed (keyword start)
		"  // EXIT;\r\n" + // removed (trailing ;)
		"  // Total per line\r\n" + // kept
		"  // CAL020: zero-arg calls including TODAY() get ()\r\n" + // kept
		"  Do(); // trailing commented := 1;\r\n" + // kept (not pure comment line)
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  // Total per line\r\n" +
		"  // CAL020: zero-arg calls including TODAY() get ()\r\n" +
		"  Do(); // trailing commented := 1;\r\n" +
		"END;\r\n"
	got, diags := runCommentRules(in, nil)
	if got != want {
		t.Fatalf("commented code\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "removed 4 commented-out code line(s)") {
		t.Errorf("expected count-4 diagnostic, got %+v", diags)
	}
}

func TestCommentsKeepPrefixProtection(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // - keep: Old := 1;\r\n" + // protected by keepCommentPrefix despite :=
		"  //- keep too;\r\n" +
		"  // Removed := 2;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  // - keep: Old := 1;\r\n" +
		"  //- keep too;\r\n" +
		"END;\r\n"
	got, _ := runCommentRules(in, nil)
	if got != want {
		t.Fatalf("keep prefix\n got: %q\nwant: %q", got, want)
	}
}

func TestCommentsMarkerPairEmpty(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // LCG PSY NAV-46641 23.11.2026 <<\r\n" +
		"  // OldCode := 1;\r\n" +
		"  { dead block }\r\n" +
		"  // LCG PSY NAV-46641 23.11.2026 >>\r\n" +
		"  Do();\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  Do();\r\n" +
		"END;\r\n"
	got, diags := runCommentRules(in, nil)
	if got != want {
		t.Fatalf("empty marker pair\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "empty code-marker pair") {
		t.Errorf("expected pair removal diagnostic, got %+v", diags)
	}
}

func TestCommentsMarkerPairWithCodeKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // LCG PSY NAV-46638 27.10.2026 <<\r\n" +
		"  Active();\r\n" +
		"  // LCG PSY NAV-46638 27.10.2026 >>\r\n" +
		"END;\r\n"
	got, _ := runCommentRules(in, nil)
	if got != in {
		t.Fatalf("marker pair with live code must stay:\n got: %q", got)
	}
}

func TestCommentsMarkerPairWithProtectedCommentKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // LCG IR STR14758 20.08.2020 <<\r\n" +
		"  // - why this stays\r\n" +
		"  // LCG IR STR14758 20.08.2020 >>\r\n" +
		"END;\r\n"
	got, _ := runCommentRules(in, nil)
	if got != in {
		t.Fatalf("protected comment must keep the marker pair:\n got: %q", got)
	}
}

func TestCommentsUnmatchedMarkersKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // LCG AL STR119 17.09.2008\r\n" + // no direction
		"  // LCG PSY NAV-1 01.01.2020 <<\r\n" + // never closed
		"  Run();\r\n" +
		"END;\r\n"
	got, _ := runCommentRules(in, nil)
	if got != in {
		t.Fatalf("unmatched markers must be kept:\n got: %q", got)
	}
}

func TestCommentsChangedLinesGate(t *testing.T) {
	in := "BEGIN\r\n" +
		"  // Old := 1;\r\n" + // line 2 unchanged -> kept
		"  // New := 2;\r\n" + // line 3 changed -> removed
		"END;\r\n"
	changed := map[int]bool{1: true, 3: true, 4: true}
	got, _ := runCommentRules(in, changed)
	if !strings.Contains(got, "// Old := 1;") {
		t.Errorf("unchanged old comment must stay:\n%s", got)
	}
	if strings.Contains(got, "// New := 2;") {
		t.Errorf("changed commented code must be removed:\n%s", got)
	}
}

func TestCommentsIdempotent(t *testing.T) {
	in := "BEGIN\r\n" +
		"  { note }\r\n" +
		"  // Gone := 1;\r\n" +
		"  // LCG PSY NAV-46641 23.11.2026 <<\r\n" +
		"  // Doomed();\r\n" +
		"  // LCG PSY NAV-46641 23.11.2026 >>\r\n" +
		"  Keep();\r\n" +
		"END;\r\n"
	first, _ := runCommentRules(in, nil)
	second, diags2 := runCommentRules(first, nil)
	if second != first {
		t.Fatalf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
	if len(diags2) != 0 {
		t.Errorf("second run must be silent, got %+v", diags2)
	}
}

// Task 6.3 rule 1: content inside {...} is never re-indented / re-spaced,
// even when the block shares lines with code (inline) or CAL035 is off.
func TestBlockContentNeverRetouched(t *testing.T) {
	body := "BEGIN\r\n" +
		"  X := 1 { inline block\r\n" +
		"        with   := messy;   spacing\r\n" +
		"        };\r\n" +
		"      Do();\r\n" +
		"END;\r\n"

	indented, _ := applyIndentation(body, 0, 2, true)
	for _, frag := range []string{"        with   := messy;   spacing", "        };"} {
		if !strings.Contains(indented, frag) {
			t.Errorf("block interior was re-indented, missing %q in:\n%s", frag, indented)
		}
	}
	spaced, _ := applySpacing(body, nil)
	if !strings.Contains(spaced, "with   := messy;   spacing") {
		t.Errorf("block interior was re-spaced:\n%s", spaced)
	}
	blanks, _ := applyBlankLinesAndSemicolons(body, nil)
	if !strings.Contains(blanks, "with   := messy;   spacing") {
		t.Errorf("block interior damaged by blank-line pass:\n%s", blanks)
	}
}

func TestCommentsDisabledKeepsEverything(t *testing.T) {
	cfg, err := config.Parse([]byte(`{"rules":{"CAL035":{"enabled":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Process([]byte(commentFixtureObject), cfg, testWorkspace(t))
	if err != nil {
		t.Fatal(err)
	}
	o := string(res.Output)
	if !strings.Contains(o, "// Deprecated := 1;") || !strings.Contains(o, "{ old vendor note }") {
		t.Errorf("with CAL035 disabled comments must survive:\n%s", o)
	}
}

const commentFixtureObject = `OBJECT Codeunit 50039 Comments
{
  OBJECT-PROPERTIES
  {
    Date=28.11.26;
    Time=12:00:00;
    Modified=Yes;
    Version List=TEST;
  }
  PROPERTIES
  {
  }
  CODE
  {
    PROCEDURE Run@1(CodeP@1000 : Integer);
    VAR
      TotalL@1001 : Integer;
    BEGIN
      { old vendor note }
      // Deprecated := 1;
      // - keep this explanation
      // LCG PSY NAV-46641 23.11.2026 <<
      // RemovedCall();
      // LCG PSY NAV-46641 23.11.2026 >>
      IF CodeP > 0 { inline } THEN
        TotalL := CodeP;
      MESSAGE('%1', TotalL);
    END;

    BEGIN
    END.
  }
}
`

func TestCommentsEndToEnd(t *testing.T) {
	_, out := processTestObject(t, commentFixtureObject)

	if strings.Contains(out, "old vendor note") {
		t.Errorf("standalone block must be removed:\n%s", out)
	}
	if strings.Contains(out, "Deprecated :=") || strings.Contains(out, "RemovedCall") {
		t.Errorf("commented code must be removed:\n%s", out)
	}
	if strings.Contains(out, "NAV-46641") {
		t.Errorf("empty marker pair must be removed:\n%s", out)
	}
	if !strings.Contains(out, "// - keep this explanation") {
		t.Errorf("protected comment must survive:\n%s", out)
	}
	if !strings.Contains(out, "CodeP > 0 { inline }") {
		t.Errorf("inline block must survive untouched:\n%s", out)
	}
	// Pipeline still formats everything else.
	if !strings.Contains(out, "IF CodeP > 0 { inline } THEN") {
		t.Errorf("unexpected formatting:\n%s", out)
	}
}
