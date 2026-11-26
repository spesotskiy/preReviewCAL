package rules

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

const legacyCodeunitBase = `OBJECT Codeunit 50200 Legacy Helper
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
    Time=12:00:00;
    Modified=Yes;
    Version List=LEGACY;
  }
  PROPERTIES
  {
  }
  CODE
  {
    VAR
      LegacyUnusedGlobal@1000 : Integer;
      ActiveGlobal@1001 : Integer;

    procedure UntouchedLegacyProc@1(OldArg@1000 : Integer);
    var
      DirtyVar@1001 : Integer;
      UnusedOld@1002 : Text[30];
      ItemLedg@1003 : Record 32;
    begin
      // Must stay 100% untouched in diff mode!
      dirtyvar:=oldarg+1;;
      with itemledg do begin
          setrange("Item No.",'A');
      end;
    end;

    procedure SmallOldProc@2(Val@1000 : Integer);
    var
      Temp@1001 : Integer;
    begin
      temp:=val+1;
      ActiveGlobal:=temp;
    end;

    procedure LargeOldProc@3(OldParam@1000 : Integer);
    var
      OldUnused@1001 : Integer;
      OldCounter@1002 : Integer;
      OldRec@1003 : Record 32;
    begin
      oldcounter:=oldparam;
      with oldrec do begin
        setrange("Item No.",'OLD');
        if findfirst then
          oldcounter:=oldcounter+Quantity;
      end;
          oldcounter:=oldcounter+1;
          oldcounter:=oldcounter+2;
          oldcounter:=oldcounter+3;
          oldcounter:=oldcounter+4;
          oldcounter:=oldcounter+5;
      ActiveGlobal:=oldcounter;
    end;

    BEGIN
    END.
  }
}
`

const legacyCodeunitModified = `OBJECT Codeunit 50200 Legacy Helper
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
    Time=12:00:00;
    Modified=Yes;
    Version List=LEGACY;
  }
  PROPERTIES
  {
  }
  CODE
  {
    VAR
      LegacyUnusedGlobal@1000 : Integer;
      ActiveGlobal@1001 : Integer;

    procedure UntouchedLegacyProc@1(OldArg@1000 : Integer);
    var
      DirtyVar@1001 : Integer;
      UnusedOld@1002 : Text[30];
      ItemLedg@1003 : Record 32;
    begin
      // Must stay 100% untouched in diff mode!
      dirtyvar:=oldarg+1;;
      with itemledg do begin
          setrange("Item No.",'A');
      end;
    end;

    procedure SmallOldProc@2(Val@1000 : Integer);
    var
      Temp@1001 : Integer;
    begin
      temp:=val+2;
      ActiveGlobal:=temp;
    end;

    procedure LargeOldProc@3(OldParam@1000 : Integer);
    var
      OldUnused@1001 : Integer;
      OldCounter@1002 : Integer;
      OldRec@1003 : Record 32;
      NewFlag@1004 : Boolean;
      NewUnused@1005 : Integer;
      NewMsg@1006 : TextConst 'ENU=Done';
      NewRec@1007 : Record 32;
    begin
      oldcounter:=oldparam;
      with oldrec do begin
        setrange("Item No.",'OLD');
        if findfirst then
          oldcounter:=oldcounter+Quantity;
        // CAL020, CAL032, CAL033: new line inside old WITH (old WITH must NOT be removed!)
        newflag:=(today<>0D);
      end;
          oldcounter:=oldcounter+1;
          oldcounter:=oldcounter+2;
      // CAL005, CAL010, CAL030 (Variant A anchored to 10 spaces), CAL031, CAL032, CAL033
      if (newflag) then
      begin
        with newrec do begin
          setrange("Item No.",'NEW');
          if findfirst then
            message(newmsg);
        end;
      end;
          oldcounter:=oldcounter+3;
          oldcounter:=oldcounter+4;
          oldcounter:=oldcounter+5;
      ActiveGlobal:=oldcounter;
    end;

    procedure AddedNewProc@4(CodeVal@1000 : Code[20]);
    var
      NewLocal@1001 : Date;
    begin
      // Newly added proc: must be fully cleaned
      newlocal:=today;;
      if (codeval='') then
        exit();
      message('%1 %2',codeval,newlocal);
    end;

    BEGIN
    END.
  }
}
`

const legacyCodeunitExpectedLimit10 = `OBJECT Codeunit 50200 Legacy Helper
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
    Time=12:00:00;
    Modified=Yes;
    Version List=LEGACY;
  }
  PROPERTIES
  {
  }
  CODE
  {
    VAR
      ActiveGlobal@1001 : Integer;
      NewMsg@1002 : TextConst 'ENU=Done';

    procedure UntouchedLegacyProc@1(OldArg@1000 : Integer);
    var
      DirtyVar@1001 : Integer;
      UnusedOld@1002 : Text[30];
      ItemLedg@1003 : Record 32;
    begin
      // Must stay 100% untouched in diff mode!
      dirtyvar:=oldarg+1;;
      with itemledg do begin
          setrange("Item No.",'A');
      end;
    end;

    PROCEDURE SmallOldProc@2(ValP@1000 : Integer);
    VAR
      TempL@1001 : Integer;
    BEGIN
      TempL := ValP + 2;
      ActiveGlobal := TempL;
    END;

    procedure LargeOldProc@3(OldParam@1000 : Integer);
    var
      OldCounter@1002 : Integer;
      OldRec@1003 : Record 32;
      NewFlagL@1004 : Boolean;
      NewRecL@1007 : Record 32;
    begin
      oldcounter:=oldparam;
      with oldrec do begin
        setrange("Item No.",'OLD');
        if findfirst then
          oldcounter:=oldcounter+Quantity;
        // CAL020, CAL032, CAL033: new line inside old WITH (old WITH must NOT be removed!)
        NewFlagL := (TODAY() <> 0D);
      end;
          oldcounter:=oldcounter+1;
          oldcounter:=oldcounter+2;
          // CAL005, CAL010, CAL030 (Variant A anchored to 10 spaces), CAL031, CAL032, CAL033
          IF NewFlagL THEN BEGIN
            NewRecL.SETRANGE("Item No.", 'NEW');
            IF NewRecL.FINDFIRST() THEN
              MESSAGE(NewMsg);
          END;
          oldcounter:=oldcounter+3;
          oldcounter:=oldcounter+4;
          oldcounter:=oldcounter+5;
      ActiveGlobal:=oldcounter;
    end;

    PROCEDURE AddedNewProc@4(CodeValP@1000 : Code[20]);
    VAR
      NewLocalL@1001 : Date;
    BEGIN
      // Newly added proc: must be fully cleaned
      NewLocalL := TODAY();
      IF CodeValP = '' THEN
        EXIT;
      MESSAGE('%1 %2', CodeValP, NewLocalL);
    END;

    BEGIN
    END.
  }
}
`

func TestDiffModeLimitAndVariantAIndentation(t *testing.T) {
	root := repoRoot(t)
	cfg := config.Default()
	cfg.Diff.MaxOldFunctionLines = 10 // SmallOldProc (4 lines) <= 10 -> Full; LargeOldProc (14 lines) > 10 -> Partial
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)

	res, err := ProcessDiff([]byte(legacyCodeunitBase), []byte(legacyCodeunitModified), cfg, ws)
	if err != nil {
		t.Fatalf("ProcessDiff failed: %v", err)
	}
	if !bytes.Equal(res.Output, []byte(legacyCodeunitExpectedLimit10)) {
		t.Fatalf("diff-mode output mismatch:\n--- GOT ---\n%s\n--- WANT ---\n%s",
			string(res.Output), legacyCodeunitExpectedLimit10)
	}

	// Verify idempotency in diff mode: running ProcessDiff(base, res.Output) must yield Changed == false!
	res2, err := ProcessDiff([]byte(legacyCodeunitBase), res.Output, cfg, ws)
	if err != nil {
		t.Fatalf("second ProcessDiff failed: %v", err)
	}
	if res2.Changed {
		t.Fatalf("diff mode not idempotent!\n--- SECOND RUN ---\n%s", string(res2.Output))
	}
}

func TestDiffModeUnlimitedZeroTransformsLargeOldFunctionWholly(t *testing.T) {
	root := repoRoot(t)
	cfg := config.Default()
	cfg.Diff.MaxOldFunctionLines = 0 // 0 = transform any touched old function wholly!
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)

	res, err := ProcessDiff([]byte(legacyCodeunitBase), []byte(legacyCodeunitModified), cfg, ws)
	if err != nil {
		t.Fatalf("ProcessDiff(max=0) failed: %v", err)
	}
	out := string(res.Output)

	// UntouchedLegacyProc MUST STILL stay untouched!
	if !strings.Contains(out, "procedure UntouchedLegacyProc@1(OldArg@1000 : Integer);") {
		t.Errorf("expected UntouchedLegacyProc to stay untouched even when maxOldFunctionLines=0")
	}
	if !strings.Contains(out, "dirtyvar:=oldarg+1;;") {
		t.Errorf("expected UntouchedLegacyProc body to stay untouched")
	}

	// LargeOldProc MUST be wholly transformed (param OldParamP, OldUnused removed, old WITH removed, OldCounterL renamed)!
	if !strings.Contains(out, "PROCEDURE LargeOldProc@3(OldParamP@1000 : Integer);") {
		t.Errorf("expected LargeOldProc signature transformed when maxOldFunctionLines=0, got:\n%s", out)
	}
	if strings.Contains(out, "OldUnused@1001") {
		t.Errorf("expected OldUnused removed when maxOldFunctionLines=0")
	}
	if !strings.Contains(out, "OldRecL.SETRANGE(\"Item No.\", 'OLD');") {
		t.Errorf("expected old WITH eliminated in LargeOldProc when maxOldFunctionLines=0, got:\n%s", out)
	}
}

func TestDiffModeTableTriggersRespectMaxOldFunctionLines(t *testing.T) {
	baseTable := `OBJECT Table 50201 Legacy Table
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
    OnInsert=VAR
               UntouchedVar@1000 : Integer;
             BEGIN
               // Untouched trigger must not be changed
               untouchedvar:=1;;
             END;
  }
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        }
    { 2   ;   ;Small Field         ;Integer        ;OnValidate=VAR
                                                                 SmallVar@1000 : Integer;
                                                               BEGIN
                                                                 smallvar:=1;
                                                               END;
                                                    CaptionML=ENU=Small Field }
    { 3   ;   ;Large Field         ;Integer        ;OnValidate=VAR
                                                                 OldTrigVar@1000 : Integer;
                                                               BEGIN
                                                                 oldtrigvar:=1;
                                                                 oldtrigvar:=2;
                                                                 oldtrigvar:=3;
                                                                 oldtrigvar:=4;
                                                                 oldtrigvar:=5;
                                                                 oldtrigvar:=6;
                                                               END;
                                                    CaptionML=ENU=Large Field }
  }
  CODE
  {
    BEGIN
    END.
  }
}
`
	modTable := `OBJECT Table 50201 Legacy Table
{
  OBJECT-PROPERTIES
  {
    Date=21.11.26;
  }
  PROPERTIES
  {
    OnInsert=VAR
               UntouchedVar@1000 : Integer;
             BEGIN
               // Untouched trigger must not be changed
               untouchedvar:=1;;
             END;
  }
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        }
    { 2   ;   ;Small Field         ;Integer        ;OnValidate=VAR
                                                                 SmallVar@1000 : Integer;
                                                               BEGIN
                                                                 smallvar:=2;
                                                               END;
                                                    CaptionML=ENU=Small Field }
    { 3   ;   ;Large Field         ;Integer        ;OnValidate=VAR
                                                                 OldTrigVar@1000 : Integer;
                                                                 NewTrigVar@1001 : Date;
                                                               BEGIN
                                                                 oldtrigvar:=1;
                                                                 oldtrigvar:=2;
                                                                 oldtrigvar:=3;
                                                                 newtrigvar:=today;
                                                                 oldtrigvar:=4;
                                                                 oldtrigvar:=5;
                                                                 oldtrigvar:=6;
                                                               END;
                                                    CaptionML=ENU=Large Field }
  }
  CODE
  {
    BEGIN
    END.
  }
}
`

	cfg := config.Default()
	cfg.Diff.MaxOldFunctionLines = 5 // Small Field trigger (3 lines) <= 5 -> Full; Large Field trigger (8 lines) > 5 -> Partial

	res, err := ProcessDiff([]byte(baseTable), []byte(modTable), cfg, nil)
	if err != nil {
		t.Fatalf("ProcessDiff table failed: %v", err)
	}
	out := string(res.Output)

	// 1. Untouched OnInsert stays dirty verbatim.
	if !strings.Contains(out, "untouchedvar:=1;;") {
		t.Errorf("untouched OnInsert trigger was modified:\n%s", out)
	}
	// 2. Small Field OnValidate (<=5 lines) is wholly cleaned (SmallVarL).
	if !strings.Contains(out, "SmallVarL@1000 : Integer;") || !strings.Contains(out, "SmallVarL := 2;") {
		t.Errorf("small modified trigger was not wholly cleaned:\n%s", out)
	}
	// 3. Large Field OnValidate (>5 lines) keeps OldTrigVar untouched, cleans NewTrigVar -> NewTrigVarL and TODAY().
	if !strings.Contains(out, "OldTrigVar@1000 : Integer;") || !strings.Contains(out, "oldtrigvar:=1;") {
		t.Errorf("old parts of large trigger were unexpectedly modified:\n%s", out)
	}
	if !strings.Contains(out, "NewTrigVarL@1001 : Date;") || !strings.Contains(out, "NewTrigVarL := TODAY();") {
		t.Errorf("new parts of large trigger were not cleaned:\n%s", out)
	}
}
