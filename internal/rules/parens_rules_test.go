package rules

import (
	"testing"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/objectfile"
)

func lexCodes(s string) []cal.Token {
	toks := cal.Lex(s)
	var codes []cal.Token
	for _, tok := range toks {
		if !tok.IsTrivia() && tok.Kind != cal.TokEOF {
			codes = append(codes, tok)
		}
	}
	return codes
}

func isWordTok(t cal.Token) bool {
	return t.Kind == cal.TokIdent || t.Kind == cal.TokNumber
}

func runRedundantParens(t *testing.T, body string) string {
	t.Helper()
	obj := &objectfile.ObjectFile{Raw: body}
	got, _ := fixRedundantParensInBody(body, nil, obj, 0)
	return got
}

// Task 1.1 (CAL021): removing a redundant bracket must never glue two
// identifiers/keywords/numbers into one token.
func TestFixRedundantParensNoTokenGluing(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "NOT( glued to following identifier",
			in:   "IF NOT(_SalesHeaderExt.GET(TEXT('Doc No.'))) THEN EXIT(FALSE);",
			want: "IF NOT _SalesHeaderExt.GET(TEXT('Doc No.')) THEN EXIT(FALSE);",
		},
		{
			name: ")THEN glued to preceding identifier",
			in:   "IF (FlagL)THEN EXIT(FALSE);",
			want: "IF FlagL THEN EXIT(FALSE);",
		},
		{
			name: "number glued to AND keyword",
			in:   "Result := (1)AND (2);",
			want: "Result := 1 AND 2;",
		},
		{
			name: "already separated by space stays single-spaced",
			in:   "IF (FlagL) THEN TotalL := (QtyP) + (1);",
			want: "IF FlagL THEN TotalL := QtyP + 1;",
		},
		{
			name: "duplicate parens keep call and glue rules intact",
			in:   "Result := ((FlagL));",
			want: "Result := FlagL;",
		},
		{
			name: "unicode identifier glued to keyword",
			in:   "IF NOT(Документ) THEN EXIT(FALSE);",
			want: "IF NOT Документ THEN EXIT(FALSE);",
		},
		{
			name: "quoted ident needs no space",
			in:   "Result := (\"Document No.\");",
			want: "Result := \"Document No.\";",
		},
		{
			name: "paren after operator needs no space",
			in:   "TotalL := (QtyP);",
			want: "TotalL := QtyP;",
		},
		{
			name: "call parens are never touched",
			in:   "IF NOT(SalesHeader.GET()) THEN EXIT(TRUE);",
			want: "IF NOT SalesHeader.GET() THEN EXIT(TRUE);",
		},
		{
			name: "required comparison parens preserved",
			in:   "IF (QtyP = 0) AND (TotalL = 1) THEN EXIT(TRUE);",
			want: "IF (QtyP = 0) AND (TotalL = 1) THEN EXIT(TRUE);",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runRedundantParens(t, tc.in)
			if got != tc.want {
				t.Fatalf("fixRedundantParensInBody():\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// Resulting text after bracket removal must lex back into the same token sequence
// (no identifier glued with a keyword/identifier).
func TestFixRedundantParensOutputLexesSeparately(t *testing.T) {
	inputs := []string{
		"IF NOT(_SalesHeaderExt.GET(TEXT('Doc No.')))THEN EXIT(FALSE);",
		"IF (FlagL)THEN EXIT(FALSE);",
		"Result := (1)AND (2);",
		"IF NOT(Документ.ПолучитьНомер())THEN EXIT(FALSE);",
	}
	for _, in := range inputs {
		out := runRedundantParens(t, in)
		toks := lexCodes(out)
		for i := 1; i < len(toks); i++ {
			if isWordTok(toks[i-1]) && isWordTok(toks[i]) &&
				toks[i-1].End == toks[i].Start {
				t.Fatalf("glued tokens %q + %q in output %q", toks[i-1].Text, toks[i].Text, out)
			}
		}
	}
}
