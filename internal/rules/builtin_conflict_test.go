package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

func conflictScope() *localScope {
	return &localScope{vars: map[string]*cal.VarDecl{
		"time":      {Name: "Time", BaseType: "Time"},
		"counter":   {Name: "Counter", BaseType: "Integer"},
		"strsubstno": {Name: "StrSubstNo", BaseType: "Text"},
	}}
}

// Task 1.5 #1: CAL033 must never uppercase declared variables (Time vs TIME()),
// and must never uppercase the left side of ':='.
func TestKeywordCasingDeclaredNameGuard(t *testing.T) {
	scope := conflictScope()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "declared Time on LHS stays as-is",
			in:   "Time := DT2TIME(ValueP);",
			want: "Time := DT2TIME(ValueP);",
		},
		{
			name: "declared Time as RHS reference stays as-is",
			in:   "MESSAGE('%1', Time);",
			want: "MESSAGE('%1', Time);",
		},
		{
			name: "declared StrSubstNo (builtin name) stays as-is",
			in:   "Counter := StrSubstNo('%1', 1);",
			want: "Counter := StrSubstNo('%1', 1);",
		},
		{
			name: "assignment LHS without scope is not uppercased",
			in:   "time := 5;",
			want: "time := 5;",
		},
		{
			name: "undeclared zero-arg builtin still uppercased",
			in:   "X := today;",
			want: "X := TODAY;",
		},
		{
			name: "undeclared builtin call still uppercased",
			in:   "X := userid;",
			want: "X := USERID;",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := applyKeywordCasing(tc.in, nil, scope)
			if got != tc.want {
				t.Fatalf("applyKeywordCasing():\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
	// Without scope the declared-var guard must not apply (only the ':=' guard).
	got, _ := applyKeywordCasing("MESSAGE('%1', Time);", nil, nil)
	if got != "MESSAGE('%1', TIME);" {
		t.Errorf("without scope, undeclared Time is still treated as builtin, got %q", got)
	}
}

func processTestObject(t *testing.T, src string) (*Result, string) {
	t.Helper()
	root := repoRoot(t)
	cfg := config.Default()
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)
	res, err := Process([]byte(src), cfg, ws)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	return res, string(res.Output)
}

func hasWarningMentioning(diags []Diagnostic, substr string) bool {
	for _, d := range diags {
		if d.Severity == SeverityWarning && strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

// Task 1.5 #2: Return Value named Time collides with TIME() -> renamed TimeL,
// with a warning; the reported bug "Time := DT2TIME(...)" -> "TIME := ..." must not happen.
func TestReturnVarBuiltinConflictRename(t *testing.T) {
	src := `OBJECT Codeunit 50025 Return Conflict
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
    PROCEDURE GetValueAsTime@1(Value@1000 : DateTime) Time : Time;
    BEGIN
      Time := DT2TIME(Value);
      MESSAGE('%1', Time);
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if !strings.Contains(out, ") TimeL : Time;") {
		t.Errorf("expected Return Value renamed to TimeL (type Time untouched), got:\n%s", out)
	}
	if !strings.Contains(out, "TimeL := DT2TIME(ValueP);") {
		t.Errorf("expected body reference renamed, got:\n%s", out)
	}
	if strings.Contains(out, "TIME :=") {
		t.Errorf("regression: assignment was uppercased to TIME :=\n%s", out)
	}
	if !hasWarningMentioning(res.Diagnostics, "Return Value 'Time'") {
		t.Errorf("expected warning about Return Value rename, got %+v", res.Diagnostics)
	}
}

// Task 1.5 #2: a global variable colliding with a system function is renamed with 'G'
// in the declaration and in all procedure bodies, with a warning.
func TestGlobalBuiltinConflictRename(t *testing.T) {
	src := `OBJECT Codeunit 50026 Global Conflict
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
    VAR
      WorkDate@1000 : Date;

    PROCEDURE TouchGlobal@1(ValueP@1000 : Code[10]);
    BEGIN
      IF WorkDate = 0D THEN
        WorkDate := TODAY();
      MESSAGE('%1 %2', WorkDate, ValueP);
    END;

    PROCEDURE ReadGlobal@2();
    BEGIN
      EXIT(WorkDate <> 0D);
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "WorkDateG@1000 : Date;") {
		t.Errorf("expected global renamed to WorkDateG, got:\n%s", out)
	}
	if strings.Contains(out, "WorkDate ") || strings.Contains(out, "WorkDate;") || strings.Contains(out, "WorkDate =") {
		t.Errorf("expected no old global references left, got:\n%s", out)
	}
	if !strings.Contains(out, "IF WorkDateG = 0D THEN") {
		t.Errorf("expected renamed reference in body 1, got:\n%s", out)
	}
	if !strings.Contains(out, "EXIT(WorkDateG <> 0D);") {
		t.Errorf("expected renamed reference in body 2, got:\n%s", out)
	}
	if !hasWarningMentioning(res.Diagnostics, "renamed global variable 'WorkDate' to 'WorkDateG'") {
		t.Errorf("expected warning about global rename, got %+v", res.Diagnostics)
	}
}

// Task 1.5: when the colliding global is shadowed by a local/param, it must NOT be
// renamed automatically — warning only.
func TestGlobalBuiltinConflictShadowedWarnsOnly(t *testing.T) {
	src := `OBJECT Codeunit 50027 Shadowed Conflict
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
    VAR
      WorkDate@1000 : Date;

    PROCEDURE ShadowedByLocal@1();
    VAR
      WorkDate@1000 : Date;
    BEGIN
      WorkDate := TODAY();
    END;

    PROCEDURE ReadGlobal@2();
    BEGIN
      MESSAGE('%1', WorkDate);
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if strings.Contains(out, "WorkDateG") {
		t.Errorf("shadowed global must not be auto-renamed, got:\n%s", out)
	}
	if !strings.Contains(out, "WorkDateL@1000 : Date;") {
		t.Errorf("expected shadowing local renamed to WorkDateL by CAL010, got:\n%s", out)
	}
	if !hasWarningMentioning(res.Diagnostics, "shadowed by a procedure or a local/parameter declaration") {
		t.Errorf("expected warning about shadowed global, got %+v", res.Diagnostics)
	}
}

// Task 1.5 #3: when the collision-safe new name for a colliding Return Value is
// already occupied, no rename happens — warning only, and CAL033 must still
// protect the usages from being uppercased.
func TestReturnVarBuiltinConflictCollisionWarnsOnly(t *testing.T) {
	src := `OBJECT Codeunit 50028 Return Collision
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
    PROCEDURE GetValueAsTime@1(Value@1000 : DateTime) Time : Time;
    VAR
      TimeL@1001 : Integer;
    BEGIN
      Time := DT2TIME(Value);
      TimeL := 1;
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "Time : Time;") {
		t.Errorf("Return Value 'Time' must stay (TimeL occupied by local), got:\n%s", out)
	}
	if strings.Contains(out, "TIME :=") {
		t.Errorf("CAL033 must not uppercase declared Return Value, got:\n%s", out)
	}
	if !hasWarningMentioning(res.Diagnostics, "'TimeL' is already in use") {
		t.Errorf("expected collision warning for Return Value rename, got %+v", res.Diagnostics)
	}
	// The local TimeL must remain intact.
	if !strings.Contains(out, "TimeL@1001 : Integer;") || !strings.Contains(out, "TimeL := 1;") {
		t.Errorf("local TimeL must be untouched, got:\n%s", out)
	}
}
