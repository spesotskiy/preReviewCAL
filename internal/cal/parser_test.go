package cal

import "testing"

func TestLexRoundTrip(t *testing.T) {
	src := "    BEGIN\r\n" +
		"      // line comment\r\n" +
		"      { block { nested } comment }\r\n" +
		"      \"Item No.\" := 'ABC''DEF';\r\n" +
		"      IF (Qty >= 10.5) AND (PostingDate <> 0D) THEN\r\n" +
		"        EXIT;\r\n" +
		"    END;\r\n"

	toks := Lex(src)
	if got := Render(toks); got != src {
		t.Fatalf("lex round-trip mismatch:\ngot:\n%q\nwant:\n%q", got, src)
	}
}

func TestParseCodeSection(t *testing.T) {
	code := "    VAR\r\n" +
		"      GlobalRec@1000 : TEMPORARY Record 32;\r\n" +
		"      ConfirmMsg@1001 : TextConst 'ENU=Are you sure?;RUS=Вы уверены?';\r\n" +
		"\r\n" +
		"    [EventSubscriber(Codeunit,80,OnBeforePostSalesDoc,'',false,false)]\r\n" +
		"    LOCAL PROCEDURE HandleBeforePost@1(VAR SalesHeader@1000 : Record 36;CommitIsSuppressed@1001 : Boolean) Result : Boolean;\r\n" +
		"    VAR\r\n" +
		"      LocalInt@1002 : Integer;\r\n" +
		"    BEGIN\r\n" +
		"      LocalInt := 1;\r\n" +
		"      Result := TRUE;\r\n" +
		"    END;\r\n" +
		"\r\n" +
		"    BEGIN\r\n" +
		"    {\r\n" +
		"      Docs\r\n" +
		"    }\r\n" +
		"    END.\r\n"

	cs := ParseCodeSection(code)
	if cs.GlobalVars == nil || len(cs.GlobalVars.Decls) != 2 {
		t.Fatalf("expected 2 global vars, got %+v", cs.GlobalVars)
	}
	if !cs.GlobalVars.Decls[0].IsTemporary || cs.GlobalVars.Decls[0].SubtypeID != 32 {
		t.Fatalf("unexpected GlobalRec: %+v", cs.GlobalVars.Decls[0])
	}
	if !cs.GlobalVars.Decls[1].IsTextConst {
		t.Fatalf("expected ConfirmMsg IsTextConst=true")
	}
	if len(cs.Procedures) != 1 {
		t.Fatalf("expected 1 procedure, got %d", len(cs.Procedures))
	}
	p := cs.Procedures[0]
	if !p.HasAttribute("EventSubscriber") || !p.IsLocal || p.Name != "HandleBeforePost" {
		t.Fatalf("unexpected proc: %+v", p)
	}
	if len(p.Params) != 2 || !p.Params[0].IsVarParam || p.Params[0].SubtypeID != 36 {
		t.Fatalf("unexpected params: %+v", p.Params)
	}
	if p.ReturnVar == nil || p.ReturnVar.Name != "Result" {
		t.Fatalf("expected ReturnVar Result, got %+v", p.ReturnVar)
	}
	if p.LocalVars == nil || len(p.LocalVars.Decls) != 1 || p.LocalVars.Decls[0].Name != "LocalInt" {
		t.Fatalf("unexpected local vars: %+v", p.LocalVars)
	}
	if cs.DocStart == -1 || cs.DocEnd == -1 {
		t.Fatalf("expected documentation block")
	}
}
