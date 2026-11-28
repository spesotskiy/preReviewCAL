package objectfile

import (
	"os"
	"path/filepath"
	"testing"

	"prereviewcal/internal/config"
)

// Task 1.3: WorkspaceIndex must index not only table fields but also public
// procedures of tables AND other objects.
func TestWorkspaceIndexObjectProcedures(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()

	tableSrc := `OBJECT Table 32 Item Ledger Entry
{
  FIELDS
  {
    { 1   ;   ;Entry No.           ;Integer        }
    { 2   ;   ;Item No.            ;Code20         }
  }
  CODE
  {
    PROCEDURE SetUseADO@100(UseADO@1000 : Boolean);
    BEGIN
    END;

    LOCAL PROCEDURE HiddenHelper@101();
    BEGIN
    END;

    BEGIN
    END.
  }
}
`
	cuSrc := `OBJECT Codeunit 50050 Schema Helper
{
  CODE
  {
    PROCEDURE DoWork@1(X@1000 : Integer);
    BEGIN
    END;

    LOCAL PROCEDURE Private@2();
    BEGIN
    END;

    BEGIN
    END.
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "t32 - Item Ledger Entry.txt"), []byte(tableSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c50050 - Schema Helper.txt"), []byte(cuSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewWorkspaceIndex(dir, cfg.FileNaming)

	ts := idx.LookupTable(32)
	if ts == nil {
		t.Fatal("LookupTable(32) = nil")
	}
	if !ts.HasField("item no.") {
		t.Errorf("expected field 'Item No.' indexed")
	}
	if !ts.HasProc("SetUseADO") {
		t.Errorf("expected public table procedure 'SetUseADO' indexed")
	}
	if ts.HasProc("HiddenHelper") {
		t.Errorf("LOCAL procedure must not be indexed as public")
	}

	// "Record" base type maps to the Table schema.
	if ts2 := idx.LookupObject("Record", 32); ts2 != ts {
		t.Errorf("LookupObject(Record,32) must resolve to the Table 32 schema")
	}

	cu := idx.LookupObject("Codeunit", 50050)
	if cu == nil {
		t.Fatal("LookupObject(Codeunit,50050) = nil (non-table files must be indexed too)")
	}
	if !cu.HasProc("DoWork") {
		t.Errorf("expected public codeunit procedure 'DoWork' indexed")
	}
	if cu.HasProc("Private") {
		t.Errorf("LOCAL codeunit procedure must not be indexed")
	}

	// RegisterTable still guards against non-table objects.
	idx2 := NewWorkspaceIndex("", cfg.FileNaming)
	idx2.RegisterTable(&ObjectFile{ObjectType: "Codeunit", ObjectID: 1})
	if idx2.LookupTable(1) != nil {
		t.Errorf("RegisterTable must ignore non-Table objects")
	}
}
