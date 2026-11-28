package rules

import (
	"strings"
	"testing"

	"prereviewcal/internal/config"
)

func withTestObject(procs string) string {
	return `OBJECT Codeunit 50029 Strict With
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
` + procs + `
    BEGIN
    END.
  }
}
`
}

// Task 1.3 #4: no blind heuristics. If the schema of the WITH target table is not
// present in --symbols-dir, the WITH block must NOT be touched and a warning emitted.
func TestWithUnknownSchemaNotExpanded(t *testing.T) {
	src := withTestObject(`    PROCEDURE UseUnknown@1();
    VAR
      SomeRec@1000 : Record 99999;
      TotalL@1001 : Decimal;
    BEGIN
      WITH SomeRec DO
        TotalL += Quantity;
    END;
`)
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "WITH SomeRec") {
		t.Errorf("WITH must stay untouched when schema is unknown:\n%s", out)
	}
	if strings.Contains(out, "SomeRec") && strings.Contains(out, ".Quantity") {
		t.Errorf("blind heuristic prefixed unknown field:\n%s", out)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Rule == config.RuleNoWith && d.Severity == SeverityWarning && !d.Fixed &&
			strings.Contains(d.Message, "not found in --symbols-dir") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CAL005 warning about missing schema, got %+v", res.Diagnostics)
	}
}

// Task 1.3 #4: even with a known table, identifiers absent from the schema
// (fields/procedures) must block the expansion with a warning.
func TestWithKnownSchemaUnknownIdentifier(t *testing.T) {
	src := withTestObject(`    PROCEDURE UseMystery@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
      TotalQtyL@1001 : Decimal;
    BEGIN
      WITH ItemLedgEntryL DO BEGIN
        SETRANGE("Item No.", 'X');
        TotalQtyL += MysteryField;
      END;
    END;
`)
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "WITH ItemLedgEntryL DO BEGIN") {
		t.Errorf("WITH must stay untouched when an identifier is not in the schema:\n%s", out)
	}
	if strings.Contains(out, "ItemLedgEntryL.MysteryField") {
		t.Errorf("unknown identifier must not be blindly prefixed:\n%s", out)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Rule == config.RuleNoWith && !d.Fixed && strings.Contains(d.Message, "'MysteryField'") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CAL005 warning naming MysteryField, got %+v", res.Diagnostics)
	}
}

// Task 1.3 #3: public table procedures from the schema are qualified; the full
// expansion still works for schema-complete blocks.
func TestWithSchemaProcedureQualified(t *testing.T) {
	src := withTestObject(`    PROCEDURE ApplyAdo@1(UseAdoP@1000 : Boolean);
    VAR
      ItemLedgEntryL@1001 : Record 32;
    BEGIN
      WITH ItemLedgEntryL DO
        SetUseADO(UseAdoP);
    END;
`)
	_, out := processTestObject(t, src)

	if !strings.Contains(out, "ItemLedgEntryL.SetUseADO(UseAdoP);") {
		t.Errorf("expected public table procedure prefixed from schema, got:\n%s", out)
	}
	if strings.Contains(out, "WITH") {
		t.Errorf("WITH should be eliminated in a schema-complete block:\n%s", out)
	}
}

// Task 1.3 #2: system enum qualifiers before '::' are never prefixed.
func TestWithEnumScopeNotPrefixed(t *testing.T) {
	src := withTestObject(`    PROCEDURE LookupEntry@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
      TotalQtyL@1001 : Decimal;
    BEGIN
      WITH ItemLedgEntryL DO BEGIN
        IF FORM.RUNMODAL(0, ItemLedgEntryL) = ACTION::LookupOK THEN
          TotalQtyL := Quantity;
        SETRANGE(Open, TRUE);
      END;
    END;
`)
	_, out := processTestObject(t, src)

	if strings.Contains(out, "ItemLedgEntryL.ACTION") {
		t.Errorf("enum qualifier ACTION must never be prefixed:\n%s", out)
	}
	if !strings.Contains(out, "ACTION::LookupOK") {
		t.Errorf("expected ACTION::LookupOK intact, got:\n%s", out)
	}
	// The rest of the block IS resolvable (Quantity field, Open in field slot, FIND methods).
	if !strings.Contains(out, "TotalQtyL := ItemLedgEntryL.Quantity;") {
		t.Errorf("expected normal WITH expansion for resolvable identifiers, got:\n%s", out)
	}
}

// Task 1.3: nested WITH — if the inner block cannot be eliminated (no schema),
// the outer one must not be expanded either; both get warnings.
func TestWithNestedInnerBlocked(t *testing.T) {
	src := withTestObject(`    PROCEDURE Nested@1();
    VAR
      ItemLedgEntryL@1000 : Record 32;
      SomeRec@1001 : Record 99999;
      TotalQtyL@1002 : Decimal;
    BEGIN
      WITH ItemLedgEntryL DO BEGIN
        SETRANGE("Item No.", 'X');
        WITH SomeRec DO
          TotalQtyL += Quantity;
      END;
    END;
`)
	res, out := processTestObject(t, src)

	if !strings.Contains(out, "WITH ItemLedgEntryL DO BEGIN") || !strings.Contains(out, "WITH SomeRec") {
		t.Errorf("both WITH blocks must remain untouched:\n%s", out)
	}
	var innerWarn, outerWarn bool
	for _, d := range res.Diagnostics {
		if d.Rule != config.RuleNoWith || d.Fixed {
			continue
		}
		if strings.Contains(d.Message, "SomeRec") && strings.Contains(d.Message, "--symbols-dir") {
			innerWarn = true
		}
		if strings.Contains(d.Message, "ItemLedgEntryL") && strings.Contains(d.Message, "inner WITH") {
			outerWarn = true
		}
	}
	if !innerWarn {
		t.Errorf("expected warning for unresolvable inner WITH, got %+v", res.Diagnostics)
	}
	if !outerWarn {
		t.Errorf("expected warning for outer WITH blocked by inner, got %+v", res.Diagnostics)
	}
}
