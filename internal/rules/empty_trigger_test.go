package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

func testWorkspace(t *testing.T) *objectfile.WorkspaceIndex {
	t.Helper()
	return objectfile.NewWorkspaceIndex(filepath.Join(repoRoot(t), "testdata", "symbols"), config.Default().FileNaming)
}

const emptyTriggerTable = `OBJECT Table 50036 Empty Trig Table
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
    OnDelete=BEGIN
             END;

    OnInsert=BEGIN
               TotalL := 1;
             END;

    OnRename=BEGIN
               // explanatory comment stays
             END;

    OnLookup=BEGIN
             END;
  }
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        ;OnValidate=VAR
                                                                  UnusedL@1000 : Integer;
                                                                BEGIN
                                                                END;
                                                    CaptionML=ENU=Entry No. }
    { 2   ;   ;Code Field          ;Code20         ;OnLookup=BEGIN
                                                               END;
                                             }
    { 3   ;   ;Keep Field          ;Integer        ;OnNewRecord=BEGIN
                                                                  TotalL := 2;
                                                                END;
                                             }
  }
  CODE
  {
    VAR
      TotalL@1000 : Integer;

    BEGIN
    END.
  }
}
`

func TestEmptyTriggerTableRemoval(t *testing.T) {
	_, out := processTestObject(t, emptyTriggerTable)

	// 1. Empty PROPERTIES trigger removed entirely (name, body, trailing blank line).
	if strings.Contains(out, "OnDelete=") {
		t.Errorf("empty OnDelete trigger must be removed:\n%s", out)
	}
	// 2. Non-empty triggers stay.
	if !strings.Contains(out, "OnInsert=BEGIN") || !strings.Contains(out, "TotalL := 1;") {
		t.Errorf("contentful OnInsert must stay:\n%s", out)
	}
	// 3. Trigger with only a comment stays.
	if !strings.Contains(out, "OnRename=") || !strings.Contains(out, "explanatory comment stays") {
		t.Errorf("comment-only trigger must stay:\n%s", out)
	}
	// 4. OnLookup exception (PROPERTIES and FIELDS).
	if !strings.Contains(out, "OnLookup=") || strings.Count(out, "OnLookup=") < 2 {
		t.Errorf("OnLookup triggers must never be removed:\n%s", out)
	}
	// 5. Field-level trigger empty after CAL001 removed its unused var -> whole property gone,
	//    field and the following CaptionML property stay glued with ';'.
	if strings.Contains(out, "OnValidate=") {
		t.Errorf("OnValidate property emptied via CAL001 must be removed:\n%s", out)
	}
	if !strings.Contains(out, "Entry No.") {
		t.Errorf("field definition must stay:\n%s", out)
	}
	if !strings.Contains(out, ";CaptionML=ENU=Entry No.") {
		t.Errorf("next field property must be glued back onto the field line:\n%s", out)
	}
	// 6. Contentful field trigger untouched.
	if !strings.Contains(out, "OnNewRecord=") {
		t.Errorf("contentful field trigger must stay:\n%s", out)
	}
}

func TestEmptyTriggerCodeunitOnRunKept(t *testing.T) {
	src := `OBJECT Codeunit 50037 Empty CU
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
    PROCEDURE Noop@1();
    BEGIN
    END;

    BEGIN
    END.
  }
}
`
	_, out := processTestObject(t, src)
	if !strings.Contains(out, "BEGIN\r\n    END.") && !strings.Contains(out, "BEGIN\n    END.") {
		t.Errorf("codeunit implicit OnRun must stay:\n%s", out)
	}
}

func TestEmptyTriggerXMLportElements(t *testing.T) {
	src := `OBJECT XMLport 50038 Empty Trig XMLport
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
    OnPreXMLport=BEGIN
                 END;

  }
  ELEMENTS
  {
    { Element@1 ;Item            ;Element;Type=Node;SourceTable=36;SourceExpr=Header;OnPostXMLport=BEGIN
                                                                                   END;
                                     SourceField=No. }
  }
  CODE
  {
    VAR
      Header@1000 : Record 36;

    BEGIN
    END.
  }
}
`
	_, out := processTestObject(t, src)

	if strings.Contains(out, "OnPreXMLport=") {
		t.Errorf("empty PROPERTIES trigger must be removed:\n%s", out)
	}
	if strings.Contains(out, "OnPostXMLport=") {
		t.Errorf("empty ELEMENTS trigger must be removed:\n%s", out)
	}
	if !strings.Contains(out, "SourceField=No.") || !strings.Contains(out, "SourceExpr=Header;SourceField=No.") {
		t.Errorf("element attributes must stay glued after the removed trigger:\n%s", out)
	}
}

func TestEmptyTriggerDiffModeKeepsUntouched(t *testing.T) {
	base := strings.Replace(emptyTriggerTable, "TotalL := 1;", "TotalL := 9;", 1)
	res, err := ProcessDiff([]byte(base), []byte(emptyTriggerTable), config.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Output)
	// OnInsert was modified (one line), but the old empty OnDelete trigger is NOT on a
	// changed line -> in diff mode empty-trigger removal is skipped for it (conservative).
	if !strings.Contains(out, "OnDelete=") {
		t.Errorf("diff mode must not remove old untouched empty triggers:\n%s", out)
	}
}

func TestEmptyTriggerRuleDisabledWithCAL001(t *testing.T) {
	cfg, err := config.Parse([]byte(`{"rules":{"CAL001":{"enabled":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	ws := testWorkspace(t)
	res, err := Process([]byte(emptyTriggerTable), cfg, ws)
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Output)
	if !strings.Contains(out, "OnDelete=BEGIN") {
		t.Errorf("with CAL001 disabled the empty-trigger pass must not run:\n%s", out)
	}
}
