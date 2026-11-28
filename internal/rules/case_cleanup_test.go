package rules

import (
	"strings"
	"testing"

	"prereviewcal/internal/objectfile"
)

func runCaseCleanup(body string) (string, []Diagnostic) {
	obj := &objectfile.ObjectFile{Raw: body}
	return applyCaseCleanup(body, nil, obj, 0)
}

func TestCaseCleanupEmptyElse(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: DoOne();\r\n" +
		"  ELSE;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: DoOne();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, diags := runCaseCleanup(in)
	if got != want {
		t.Fatalf("empty ELSE;\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "removed empty ELSE section in CASE") {
		t.Errorf("expected ELSE removal diagnostic, got %+v", diags)
	}
}

func TestCaseCleanupElseBeforeEnd(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: DoOne();\r\n" +
		"  ELSE\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: DoOne();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if got != want {
		t.Fatalf("ELSE before END;\n got: %q\nwant: %q", got, want)
	}
}

func TestCaseCleanupEmptyArmsNoElse(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE \"Order Status\" OF\r\n" +
		"    \"Order Status\"::New: ;\r\n" +
		"    \"Order Status\"::Open: DoOpen();\r\n" +
		"    3: ;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  CASE \"Order Status\" OF\r\n" +
		"    \"Order Status\"::Open: DoOpen();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, diags := runCaseCleanup(in)
	if got != want {
		t.Fatalf("empty arms\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "removed empty CASE branch") {
		t.Errorf("expected arm removal diagnostics, got %+v", diags)
	}
}

func TestCaseCleanupEmptyArmsWithContentfulElseKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: ;\r\n" +
		"  ELSE\r\n" +
		"    DoElse();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if got != in {
		t.Fatalf("empty arms must stay when ELSE section has content:\n got: %q", got)
	}
}

func TestCaseCleanupEmptyElseThenArmsThenWholeCase(t *testing.T) {
	// ELSE; is removed first, so the CASE ends up with no ELSE and only empty
	// arms -> whole construct must be removed.
	in := "BEGIN\r\n" +
		"  DoBefore();\r\n" +
		"  CASE X OF\r\n" +
		"    1: ;\r\n" +
		"    2: ;\r\n" +
		"  ELSE;\r\n" +
		"  END;\r\n" +
		"  DoAfter();\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  DoBefore();\r\n" +
		"  DoAfter();\r\n" +
		"END;\r\n"
	got, diags := runCaseCleanup(in)
	if got != want {
		t.Fatalf("whole empty CASE removal\n got: %q\nwant: %q", got, want)
	}
	if !hasDiag(diags, SeverityInfo, "removed empty CASE ... END construct") {
		t.Errorf("expected whole-CASE removal diagnostic, got %+v", diags)
	}
}

func TestCaseCleanupAlreadyEmptyCase(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE Status OF\r\n" +
		"  END;\r\n" +
		"  Next();\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  Next();\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if got != want {
		t.Fatalf("no-arm CASE\n got: %q\nwant: %q", got, want)
	}
}

func TestCaseCleanupSelectorCallKeepsWithWarning(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE NextRec() OF\r\n" +
		"    1: ;\r\n" +
		"  ELSE;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, diags := runCaseCleanup(in)
	if got != in {
		t.Fatalf("CASE with callable selector must not be removed:\n got: %q", got)
	}
	if !hasDiag(diags, SeverityWarning, "may contain a call with side effects") {
		t.Errorf("expected side-effect warning, got %+v", diags)
	}
}

func TestCaseCleanupNestedCascade(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE A OF\r\n" +
		"    1: CASE B OF\r\n" +
		"         2: ;\r\n" +
		"       END;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if got != want {
		t.Fatalf("nested empty CASE cascade\n got: %q\nwant: %q", got, want)
	}
}

func TestCaseCleanupArmWithCommentOnSameLine(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: ; // keep me\r\n" +
		"    2: Do();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	// The empty arm tokens are removed but the trailing comment must NOT be
	// destroyed silently (the deletion stops before it).
	if strings.Contains(got, "1: ;") {
		t.Errorf("empty arm must be removed:\n%s", got)
	}
	if !strings.Contains(got, "// keep me") {
		t.Errorf("comment after the removed arm must survive:\n%s", got)
	}
	if !strings.Contains(got, "2: Do();") {
		t.Errorf("contentful arm must stay:\n%s", got)
	}
}

func TestCaseCleanupCommentOnlyArmKept(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1:\r\n" +
		"      { dead? still a comment - keep }\r\n" +
		"      ;\r\n" +
		"    2: Do();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if !strings.Contains(got, "dead? still a comment") {
		t.Errorf("arm with a comment between ':' and ';' must not be removed:\n%s", got)
	}
}

func TestCaseCleanupMultipleCasesAndBEGINArms(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: BEGIN\r\n" +
		"         DoA();\r\n" +
		"       END;\r\n" +
		"    2: ;\r\n" +
		"  END;\r\n" +
		"  CASE Y OF\r\n" +
		"    3: ;\r\n" +
		"    4: ;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	want := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: BEGIN\r\n" +
		"         DoA();\r\n" +
		"       END;\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, _ := runCaseCleanup(in)
	if got != want {
		t.Fatalf("mixed CASEs\n got: %q\nwant: %q", got, want)
	}
}

func TestCaseCleanupChangedLinesGate(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: ;\r\n" + // line 3 - unchanged old
		"    2: ;\r\n" + // line 4 - changed/new
		"    3: Do();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	changed := map[int]bool{4: true}
	obj := &objectfile.ObjectFile{Raw: in}
	got, _ := applyCaseCleanup(in, changed, obj, 0)
	if !strings.Contains(got, "1: ;") {
		t.Errorf("unchanged old arm must stay:\n%s", got)
	}
	if strings.Contains(got, "2: ;") {
		t.Errorf("changed arm must be removed:\n%s", got)
	}
}

func TestCaseCleanupNoFalsePositives(t *testing.T) {
	in := "BEGIN\r\n" +
		"  CASE X OF\r\n" +
		"    1: DoOne();\r\n" +
		"    2: BEGIN\r\n" +
		"         DoTwo();\r\n" +
		"       END;\r\n" +
		"  ELSE\r\n" +
		"    DoElse();\r\n" +
		"  END;\r\n" +
		"  CASE Codeunit.RUN(CODEUNIT::\"My CU\", Arg) OF\r\n" +
		"    1: OK();\r\n" +
		"  END;\r\n" +
		"END;\r\n"
	got, diags := runCaseCleanup(in)
	if got != in {
		t.Fatalf("valid CASEs must not be touched:\n got: %q\nwant: %q", got, in)
	}
	if len(diags) != 0 {
		t.Errorf("no diagnostics expected, got %+v", diags)
	}
}

// End-to-end through the whole pipeline (formatting runs after cleanup).
func TestCaseCleanupEndToEnd(t *testing.T) {
	src := `OBJECT Codeunit 50034 Case Cleanup
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
    PROCEDURE Process@1(DocType@1000 : Integer);
    VAR
      TotalL@1001 : Integer;
    BEGIN
      CASE DocType OF
        1: ;
        2:
          BEGIN
            TotalL := TotalL + 1;
          END;
      ELSE;
      END;

      CASE 1 OF
      END;
      MESSAGE('%1', TotalL);
    END;

    BEGIN
    END.
  }
}
`
	_, out := processTestObject(t, src)

	if strings.Contains(out, "1: ;") {
		t.Errorf("empty arm must be removed:\n%s", out)
	}
	if strings.Contains(out, "ELSE;") || strings.Contains(out, "ELSE") {
		t.Errorf("empty ELSE must be removed:\n%s", out)
	}
	if strings.Contains(out, "CASE 1 OF") {
		t.Errorf("fully empty CASE must be removed:\n%s", out)
	}
	if !strings.Contains(out, "2:") || !strings.Contains(out, "TotalL := TotalL + 1;") {
		t.Errorf("contentful arm must stay:\n%s", out)
	}
}

func hasDiag(diags []Diagnostic, sev Severity, substr string) bool {
	for _, d := range diags {
		if d.Severity == sev && strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}
