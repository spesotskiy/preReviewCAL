package rules

import (
	"strings"
	"testing"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

// Review fix 1 (CAL005): Option fields of the WITH target MUST still be prefixed
// before '::' — only true enum qualifiers (not schema fields) are protected.
func TestWithOptionFieldBeforeScopeIsPrefixed(t *testing.T) {
	src := withTestObject(`    PROCEDURE CheckType@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
      FoundL@1001 : Boolean;
    BEGIN
      WITH ItemLedgEntryL DO BEGIN
        IF "Entry Type" = "Entry Type"::Sale THEN
          FoundL := FINDFIRST();
      END;
    END;
`)
	_, out := processTestObject(t, src)

	if !strings.Contains(out, `IF ItemLedgEntryL."Entry Type" = ItemLedgEntryL."Entry Type"::Sale THEN`) {
		t.Errorf("Option field before '::' must be prefixed, got:\n%s", out)
	}
	if strings.Contains(out, "WITH") {
		t.Errorf("WITH must be expanded here, got:\n%s", out)
	}
	// Enum qualifier that is NOT a schema field stays untouched (regression both ways).
	src2 := withTestObject(`    PROCEDURE LookupEntry@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
      FoundL@1001 : Boolean;
    BEGIN
      WITH ItemLedgEntryL DO
        FoundL := FORM.RUNMODAL(0, ItemLedgEntryL) = ACTION::LookupOK;
    END;
`)
	_, out2 := processTestObject(t, src2)
	if strings.Contains(out2, "ItemLedgEntryL.ACTION") {
		t.Errorf("ACTION enum qualifier must never be prefixed, got:\n%s", out2)
	}
}

// Review fix 2 (task 1.5): applyIdentifierRenames must rename exact-UPPERCASE
// references on the LHS of ':='; builtin calls and bare RHS 0-arg references stay.
func TestRenameUppercaseAssignmentLHS(t *testing.T) {
	src := `OBJECT Codeunit 50030 Uppercase LHS
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

    PROCEDURE GetValueAsTime@1(Value@1000 : DateTime) Time : Time;
    BEGIN
      TIME := DT2TIME(Value);
      WORKDATE := TODAY();
      MESSAGE('%1', WORKDATE());
    END;

    BEGIN
    END.
  }
}
`
	_, out := processTestObject(t, src)

	if !strings.Contains(out, "TimeL := DT2TIME(ValueP);") {
		t.Errorf("uppercase LHS TIME must be renamed to TimeL, got:\n%s", out)
	}
	if !strings.Contains(out, "WorkDateG := TODAY();") {
		t.Errorf("uppercase LHS WORKDATE must be renamed to WorkDateG, got:\n%s", out)
	}
	if strings.Contains(out, "WorkDateG()") || strings.Contains(out, "TimeL(") {
		t.Errorf("uppercase builtin call reference must NOT be renamed, got:\n%s", out)
	}
	if !strings.Contains(out, "MESSAGE('%1', WORKDATE());") {
		t.Errorf("WORKDATE() call must stay intact, got:\n%s", out)
	}
}

// Review fix 3 (task 1.5): global conflict rename must be blocked when the name is
// referenced from non-code sections (ExternalRefs).
func TestGlobalRenameBlockedByExternalRefs(t *testing.T) {
	src := `OBJECT Page 50031 External Ref Conflict
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
  CONTROLS
  {
    { 1   ;1   ;Text0               ;Field;SourceExpr=WorkDate }
  }
  CODE
  {
    VAR
      WorkDate@1000 : Date;

    PROCEDURE Touch@1();
    BEGIN
      WorkDate := TODAY();
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "SourceExpr=WorkDate") {
		t.Fatalf("fixture sanity: SourceExpr must be present:\n%s", out)
	}
	if strings.Contains(out, "WorkDateG") {
		t.Errorf("global with external references must not be auto-renamed, got:\n%s", out)
	}
	found := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "referenced from non-code sections") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about non-code references, got %+v", res.Diagnostics)
	}
}

// Review fix 4: fixed warnings must be rendered with a distinguishable tag.
func TestDiagnosticStringFixedWarning(t *testing.T) {
	d := Diagnostic{Rule: config.RuleLocalVarSuffix, Severity: SeverityWarning, Line: 1, Message: "x", Fixed: true}
	if !strings.Contains(d.String(), "(fixed, warn)") {
		t.Errorf("Diagnostic.String() = %q, want tag (fixed, warn)", d.String())
	}
	dFixed := Diagnostic{Rule: config.RuleCallParens, Severity: SeverityInfo, Line: 1, Message: "x", Fixed: true}
	if !strings.Contains(dFixed.String(), "(fixed)") || strings.Contains(dFixed.String(), "warn") {
		t.Errorf("fixed info must stay (fixed), got %q", dFixed.String())
	}
	dWarn := Diagnostic{Rule: config.RuleUnusedParam, Severity: SeverityWarning, Line: 1, Message: "x", Fixed: false}
	if !strings.Contains(dWarn.String(), "(warn)") {
		t.Errorf("unfixed warning rendering changed: %q", dWarn.String())
	}
	// HasWarnings() must ignore fixed warnings (check exit code stability).
	r := &Result{Diagnostics: []Diagnostic{d}}
	if r.HasWarnings() {
		t.Errorf("HasWarnings must be false for fixed-only warnings")
	}
}

// Review fix 5: own-table fields inside a WITH block must not block elimination.
func TestWithOwnFieldsDoNotBlock(t *testing.T) {
	src := `OBJECT Table 50032 Own Fields With
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
    TableNo=50032;
  }
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        }
    { 5   ;   ;Order No.           ;Code20         }
  }
  CODE
  {
    PROCEDURE Fill@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
    BEGIN
      WITH ItemLedgEntryL DO
        SETRANGE("Item No.","Order No.");
    END;

    BEGIN
    END.
  }
}
`
	res, out := processTestObject(t, src)

	if strings.Contains(out, "WITH ItemLedgEntryL DO") {
		t.Errorf("own-table field 'Order No.' must not block WITH elimination:\n%s", out)
	}
	if !strings.Contains(out, `ItemLedgEntryL.SETRANGE("Item No.", "Order No.");`) {
		t.Errorf("expected expansion keeping own field bare, got:\n%s", out)
	}
	for _, d := range res.Diagnostics {
		if d.Rule == config.RuleNoWith && !d.Fixed {
			t.Errorf("unexpected CAL005 warning: %+v", d)
		}
	}
}

// Review fix 5: non-Table target must not prefix Record-only methods; unknown
// identifiers are deduplicated in the warning message.
func TestWithNonTableSchemaAndUnknownDedup(t *testing.T) {
	osym := &objectSymbols{
		globals:   map[string]*cal.VarDecl{},
		procs:     map[string]*cal.ProcDecl{},
		dataItems: map[string]int{},
		ownFields: map[string]string{},
	}
	scope := buildLocalScope(nil, nil, nil, osym)
	cuSchema := &objectfile.TableSchema{
		TableID:    50050,
		ObjectType: "Codeunit",
		Procs:      map[string]string{"dowork": "DoWork"},
	}
	// FINDSET is a Record method: it must NOT be blindly prefixed for a Codeunit target.
	out, unknowns := prefixWithIdentifiers("FINDSET;\nMystery;\nMystery;\nDoWork;", "MyCU", scope, osym, cuSchema)
	if strings.Contains(out, "MyCU.FINDSET") {
		t.Errorf("Record method must not be prefixed for Codeunit target: %q", out)
	}
	if !strings.Contains(out, "MyCU.DoWork") {
		t.Errorf("schema procedure must be prefixed: %q", out)
	}
	if len(unknowns) != 2 || unknowns[0] != "FINDSET" || unknowns[1] != "Mystery" {
		t.Errorf("unknowns (FINDSET unresolvable for CU, Mystery deduplicated) expected, got %v", unknowns)
	}
	if strings.Count(strings.Join(unknowns, ","), "Mystery") != 1 {
		t.Errorf("Mystery must be deduplicated, got %v", unknowns)
	}
}

// Review fix 7: CAL033 must not UPPERCASE own-table fields that collide with
// builtin names (field "Time" on the table).
func TestKeywordCasingOwnFieldGuard(t *testing.T) {
	osym := &objectSymbols{
		globals:   map[string]*cal.VarDecl{},
		procs:     map[string]*cal.ProcDecl{},
		dataItems: map[string]int{},
		ownFields: map[string]string{"time": "Time", "userid": "UserID"},
		hasOwnRec: true,
	}
	scope := buildLocalScope(nil, nil, nil, osym)
	in := "Total := Time + Duration(UserID);"
	got, _ := applyKeywordCasing(in, nil, scope)
	if got != in {
		t.Errorf("own fields Time/UserID must keep declared casing, got %q", got)
	}
	// Unknown (not declared) colliding names are still treated as builtins.
	got2, _ := applyKeywordCasing("X := today;", nil, scope)
	if got2 != "X := TODAY;" {
		t.Errorf("undeclared builtin must still be uppercased, got %q", got2)
	}
}

// Review fix 6: relative "symbolsDir" from the config file works end-to-end when
// running the CLI from another directory (resolved in internal/cli).
