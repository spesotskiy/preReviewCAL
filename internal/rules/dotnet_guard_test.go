package rules

import (
	"testing"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/objectfile"
)

// Task 1.2: DotNet / Automation / OCX members are case-sensitive and may be
// properties: CAL033 must not change their casing, CAL020 must not add ().

func interopScope() *localScope {
	return &localScope{vars: map[string]*cal.VarDecl{
		"_serverdocumentfile":     {Name: "_ServerDocumentFile", BaseType: "DotNet"},
		"_serverdocumentfilename": {Name: "_ServerDocumentFileName", BaseType: "Text"},
		"_xmldoc":                 {Name: "_XmlDoc", BaseType: "DotNet"},
		"_outlookapp":             {Name: "_OutlookApp", BaseType: "Automation"},
		"_listview":               {Name: "_ListView", BaseType: "OCX"},
		"itemledgentryl":          {Name: "ItemLedgEntryL", BaseType: "Record", SubtypeID: 32},
		"salesheader":             {Name: "SalesHeader", BaseType: "Record", SubtypeID: 36},
	}}
}

func TestKeywordCasingDotNetGuard(t *testing.T) {
	scope := interopScope()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "DotNet method .delete stays as-is",
			in:   "_ServerDocumentFile.delete(_ServerDocumentFileName);",
			want: "_ServerDocumentFile.delete(_ServerDocumentFileName);",
		},
		{
			name: "Automation method .quit stays as-is",
			in:   "IF NOT(_OutlookApp.quit(FALSE)) THEN",
			want: "IF NOT(_OutlookApp.quit(FALSE)) THEN",
		},
		{
			name: "OCX member .items stays as-is",
			in:   "_ListView.items.Item(0);",
			want: "_ListView.items.Item(0);",
		},
		{
			name: "DotNet chain via property stays as-is",
			in:   "_XmlDoc.DocumentElement.delete();",
			want: "_XmlDoc.DocumentElement.delete();",
		},
		{
			name: "DotNet chain via method result stays as-is",
			in:   "_XmlDoc.SelectNodes('//x').delete();",
			want: "_XmlDoc.SelectNodes('//x').delete();",
		},
		{
			name: "statement keywords still uppercased on DotNet lines",
			in:   "if _XmlDoc.Count > 0 then\n  exit(_ServerDocumentFile.exists());",
			want: "IF _XmlDoc.Count > 0 THEN\n  EXIT(_ServerDocumentFile.exists());",
		},
		{
			name: "record method member still uppercased",
			in:   "ItemLedgEntryL.deleteall(FALSE);",
			want: "ItemLedgEntryL.DELETEALL(FALSE);",
		},
		{
			name: "record zero-arg method with parens still uppercased",
			in:   "SalesHeader.getfilters();",
			want: "SalesHeader.GETFILTERS();",
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
}

func TestCallParensDotNetGuard(t *testing.T) {
	scope := interopScope()
	osym := &objectSymbols{
		globals:   map[string]*cal.VarDecl{},
		procs:     map[string]*cal.ProcDecl{},
		dataItems: map[string]int{},
		ownFields: map[string]string{},
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: ".NET property .Count must not get ()",
			in:   "IF _XmlDoc.Count > 0 THEN\n  EXIT(TRUE);",
			want: "IF _XmlDoc.Count > 0 THEN\n  EXIT(TRUE);",
		},
		{
			name: ".NET chained property must not get ()",
			in:   "Total := _XmlDoc.DocumentElement.Count;",
			want: "Total := _XmlDoc.DocumentElement.Count;",
		},
		{
			name: ".NET member in standalone statement must not get ()",
			in:   "BEGIN\n  _ServerDocumentFile.Attributes;\nEND;",
			want: "BEGIN\n  _ServerDocumentFile.Attributes;\nEND;",
		},
		{
			name: "Automation member in statement position must not get ()",
			in:   "BEGIN\n  _OutlookApp.Quit;\nEND;",
			want: "BEGIN\n  _OutlookApp.Quit;\nEND;",
		},
		{
			name: "OCX member after indexer must not get ()",
			in:   "Total := _ListView.ListItems.Item(1).Count;",
			want: "Total := _ListView.ListItems.Item(1).Count;",
		},
		{
			name: "record zero-arg method still gets ()",
			in:   "BEGIN\n  ItemLedgEntryL.Reset;\nEND;",
			want: "BEGIN\n  ItemLedgEntryL.Reset();\nEND;",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &objectfile.ObjectFile{Raw: tc.in}
			got, _ := fixCallParensInBody(tc.in, scope, osym, nil, obj, 0)
			if got != tc.want {
				t.Fatalf("fixCallParensInBody():\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestChainRootVarAndInterop(t *testing.T) {
	scope := interopScope()
	body := "BEGIN\n  _XmlDoc.SelectNodes('//x').Item(0).ChildNodes.Count;\n  _List[2].Items;\nEND;"
	toks := cal.Lex(body)

	findIdx := func(word string) int {
		for i, tk := range toks {
			if tk.Kind == cal.TokIdent && tk.Text == word {
				return i
			}
		}
		return -1
	}
	// recvBefore returns the token index directly before the '.' that precedes word.
	recvBefore := func(word string) int {
		i := findIdx(word)
		if i < 0 {
			return -1
		}
		d := cal.PrevCode(toks, i-1)
		if d < 0 {
			return -1
		}
		return cal.PrevCode(toks, d-1)
	}

	if vd := chainRootVar(toks, recvBefore("Item"), scope); vd == nil || vd.BaseType != "DotNet" {
		t.Errorf("chain root for Item: expected DotNet _XmlDoc, got %+v", vd)
	}
	if vd := chainRootVar(toks, recvBefore("ChildNodes"), scope); vd == nil || !isInteropVarDecl(vd) {
		t.Errorf("chain root for ChildNodes: expected interop _XmlDoc, got %+v", vd)
	}
	if vd := chainRootVar(toks, recvBefore("Count"), scope); vd == nil || !isInteropVarDecl(vd) {
		t.Errorf("chain root for Count: expected interop _XmlDoc, got %+v", vd)
	}
	if vd := chainRootVar(toks, recvBefore("Items"), scope); vd != nil {
		t.Errorf("chain root via undeclared _List: expected nil, got %+v", vd)
	}
	if !isInteropVarDecl(&cal.VarDecl{BaseType: "dotnet"}) || !isInteropVarDecl(&cal.VarDecl{BaseType: "OCX"}) ||
		!isInteropVarDecl(&cal.VarDecl{BaseType: "Automation"}) {
		t.Errorf("isInteropVarDecl must be case-insensitive for DotNet/Automation/OCX")
	}
	if isInteropVarDecl(&cal.VarDecl{BaseType: "Record"}) || isInteropVarDecl(nil) {
		t.Errorf("isInteropVarDecl must be false for Record and nil")
	}
}
