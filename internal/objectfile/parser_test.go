package objectfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"prereviewcal/internal/config"
)

func TestParseAndRoundTripCodeunit(t *testing.T) {
	sample := "OBJECT Codeunit 50000 Sales Helper\r\n" +
		"{\r\n" +
		"  OBJECT-PROPERTIES\r\n" +
		"  {\r\n" +
		"    Date=21.11.26;\r\n" +
		"    Time=12:00:00;\r\n" +
		"    Modified=Yes;\r\n" +
		"    Version List=CUSTOM;\r\n" +
		"  }\r\n" +
		"  PROPERTIES\r\n" +
		"  {\r\n" +
		"    TableNo=36;\r\n" +
		"    OnRun=BEGIN\r\n" +
		"            MESSAGE('Hello {world}');\r\n" +
		"          END;\r\n" +
		"  }\r\n" +
		"  CODE\r\n" +
		"  {\r\n" +
		"    VAR\r\n" +
		"      GlobalVar@1000 : Integer;\r\n" +
		"\r\n" +
		"    PROCEDURE DoWork@1();\r\n" +
		"    BEGIN\r\n" +
		"      // comment with unmatched }\r\n" +
		"      GlobalVar := 1;\r\n" +
		"    END;\r\n" +
		"\r\n" +
		"    BEGIN\r\n" +
		"    {\r\n" +
		"      Documentation\r\n" +
		"    }\r\n" +
		"    END.\r\n" +
		"  }\r\n" +
		"}\r\n"

	obj, err := Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if obj.ObjectType != "Codeunit" || obj.ObjectID != 50000 || obj.ObjectName != "Sales Helper" {
		t.Fatalf("unexpected header: %+v", obj)
	}
	if obj.SourceTable != 36 {
		t.Fatalf("SourceTable = %d, want 36", obj.SourceTable)
	}
	if len(obj.Triggers) != 1 || obj.Triggers[0].Name != "OnRun" {
		t.Fatalf("expected 1 OnRun trigger, got %+v", obj.Triggers)
	}
	if obj.Triggers[0].BaseIndent != 10 {
		t.Fatalf("BaseIndent = %d, want 10", obj.Triggers[0].BaseIndent)
	}
	if obj.Code == nil {
		t.Fatalf("expected Code section")
	}

	out := obj.ApplyEdits(nil)
	if !bytes.Equal(out, []byte(sample)) {
		t.Fatalf("round-trip mismatch")
	}
}

func TestParseReportWithRDLDataAndDataset(t *testing.T) {
	sample := "OBJECT Report 50020 Test Report\r\n" +
		"{\r\n" +
		"  OBJECT-PROPERTIES\r\n" +
		"  {\r\n" +
		"    Date=21.11.26;\r\n" +
		"  }\r\n" +
		"  PROPERTIES\r\n" +
		"  {\r\n" +
		"  }\r\n" +
		"  DATASET\r\n" +
		"  {\r\n" +
		"    { 1000;    ;ItemLedgerEntry     ;DataItem;DataItemTable=Table32 }\r\n" +
		"    { 1001;1   ;TotalQtyCol         ;Column  ;TotalQtyGlobal }\r\n" +
		"  }\r\n" +
		"  CODE\r\n" +
		"  {\r\n" +
		"    VAR\r\n" +
		"      TotalQtyGlobal@1000 : Decimal;\r\n" +
		"\r\n" +
		"    BEGIN\r\n" +
		"    END.\r\n" +
		"  }\r\n" +
		"  RDLDATA\r\n" +
		"  {\r\n" +
		"    <?xml version=\"1.0\"?>\r\n" +
		"    <Report>{arbitrary braces}</Report>\r\n" +
		"    END_OF_RDLDATA\r\n" +
		"  }\r\n" +
		"}\r\n"

	obj, err := Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !obj.HasRDLData {
		t.Fatalf("expected HasRDLData=true")
	}
	if !obj.ExternalRefs["totalqtyglobal"] {
		t.Fatalf("expected totalqtyglobal in ExternalRefs, got %+v", obj.ExternalRefs)
	}
	if len(obj.DataItems) != 1 || obj.DataItems[0].TableID != 32 {
		t.Fatalf("expected DataItem Table 32, got %+v", obj.DataItems)
	}
	if !bytes.Equal(obj.ApplyEdits(nil), []byte(sample)) {
		t.Fatalf("round-trip mismatch for report with RDLDATA")
	}
}

func TestWorkspaceIndexFindsConfiguredTableFilename(t *testing.T) {
	dir := t.TempDir()
	tableSrc := "OBJECT Table 32 Item Ledger Entry\r\n" +
		"{\r\n" +
		"  OBJECT-PROPERTIES\r\n" +
		"  {\r\n" +
		"  }\r\n" +
		"  PROPERTIES\r\n" +
		"  {\r\n" +
		"  }\r\n" +
		"  FIELDS\r\n" +
		"  {\r\n" +
		"    { 1   ;   ;Entry No.           ;Integer        }\r\n" +
		"    { 2   ;   ;Item No.            ;Code20         }\r\n" +
		"    { 13  ;   ;Quantity            ;Decimal        }\r\n" +
		"  }\r\n" +
		"  CODE\r\n" +
		"  {\r\n" +
		"    BEGIN\r\n" +
		"    END.\r\n" +
		"  }\r\n" +
		"}\r\n"
	filePath := filepath.Join(dir, "t32 - Item Ledger Entry.txt")
	if err := os.WriteFile(filePath, []byte(tableSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := NewWorkspaceIndex(dir, config.Default().FileNaming)
	schema := idx.LookupTable(32)
	if schema == nil {
		t.Fatalf("expected schema for table 32")
	}
	if !schema.HasField("Entry No.") || !schema.HasField("quantity") || !schema.HasField("Item No.") {
		t.Fatalf("missing fields in schema: %+v", schema.Fields)
	}
}
