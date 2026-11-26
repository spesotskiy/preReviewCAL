package rules

import (
	"fmt"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

// runCallParensRule implements CAL020:
// - Add () to zero-argument procedure calls, built-in zero-arg functions (including TODAY(), COMPANYNAME()),
//   and zero-arg Record/Object methods.
// - Keep EXIT without empty parentheses (EXIT() -> EXIT; EXIT(val) unchanged).
func runCallParensRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var cs *cal.CodeSection
	if obj.Code != nil {
		cs = cal.ParseCodeSection(obj.Code.Raw)
	}
	osym := buildObjectSymbols(obj, cs)

	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	// Embedded triggers.
	for _, tr := range obj.Triggers {
		us := fscope.TriggerScope(tr)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		td := cal.ParseTrigger(tr.Raw)
		if td.BodyStart < 0 || td.BodyEnd <= td.BodyStart {
			continue
		}
		scope := buildLocalScope(nil, nil, td.LocalVars, osym)
		body := tr.Raw[td.BodyStart:td.BodyEnd]
		var changedLines map[int]bool
		if us.Mode == diffscope.ModePartial {
			changedLines = us.ChangedBodyLines(body)
		}
		newBody, d := fixCallParensInBody(body, scope, osym, changedLines, obj, tr.StartOffset+td.BodyStart)
		diags = append(diags, d...)
		if newBody != body {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset + td.BodyStart,
				End:     tr.StartOffset + td.BodyEnd,
				NewText: newBody,
			})
		}
	}

	// CODE procedures.
	if obj.Code != nil && cs != nil {
		var codeEdits []textEdit
		for _, proc := range cs.Procedures {
			us := fscope.ProcScope(proc)
			if us.Mode == diffscope.ModeSkip {
				continue
			}
			if proc.BodyStart < 0 || proc.BodyEnd <= proc.BodyStart {
				continue
			}
			scope := buildLocalScope(proc.Params, proc.ReturnVar, proc.LocalVars, osym)
			body := obj.Code.Raw[proc.BodyStart:proc.BodyEnd]
			var changedLines map[int]bool
			if us.Mode == diffscope.ModePartial {
				changedLines = us.ChangedBodyLines(body)
			}
			newBody, d := fixCallParensInBody(body, scope, osym, changedLines, obj, obj.Code.StartOffset+proc.BodyStart)
			diags = append(diags, d...)
			if newBody != body {
				codeEdits = append(codeEdits, textEdit{
					start:   proc.BodyStart,
					end:     proc.BodyEnd,
					newText: newBody,
				})
			}
		}
		if len(codeEdits) > 0 {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   obj.Code.StartOffset,
				End:     obj.Code.EndOffset,
				NewText: applyTextEdits(obj.Code.Raw, codeEdits),
			})
		}
	}

	return spanEdits, diags
}

func fixCallParensInBody(body string, scope *localScope, osym *objectSymbols, changedLines map[int]bool, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
	tokens := cal.Lex(body)
	var edits []textEdit
	var diags []Diagnostic

	for i, tok := range tokens {
		if tok.Kind != cal.TokIdent && tok.Kind != cal.TokQuotedIdent {
			continue
		}
		if changedLines != nil && !changedLines[tok.Line] {
			continue
		}

		// 1. EXIT() with empty parens -> EXIT (user rule 3: only EXIT should be without parens).
		if tok.IsWord("EXIT") && !isMemberAccess(tokens, i) {
			next := cal.NextCode(tokens, i+1)
			if next != -1 && tokens[next].Kind == cal.TokLParen {
				afterLP := cal.NextCode(tokens, next+1)
				if afterLP != -1 && tokens[afterLP].Kind == cal.TokRParen {
					edits = append(edits, textEdit{
						start:   tok.End,
						end:     tokens[afterLP].End,
						newText: "",
					})
					diags = append(diags, Diagnostic{
						Rule:     config.RuleCallParens,
						Severity: SeverityInfo,
						Line:     obj.LineOfOffset(fileBaseOffset + tok.Start),
						Message:  "removed empty parentheses from EXIT()",
						Fixed:    true,
					})
				}
			}
			continue
		}

		// Skip statement keywords.
		if tok.Kind == cal.TokIdent && cal.IsStatementKeyword(tok.Text) && !isMemberAccess(tokens, i) {
			continue
		}

		next := cal.NextCode(tokens, i+1)
		// Already followed by '(', assignment, member chain, scope, indexing, or CASE label ':' -> skip.
		if next != -1 {
			switch tokens[next].Kind {
			case cal.TokLParen, cal.TokAssign, cal.TokPlusAssign, cal.TokMinusAssign,
				cal.TokMulAssign, cal.TokDivAssign, cal.TokDot, cal.TokScope, cal.TokLBracket, cal.TokColon:
				continue
			}
		}

		prev := cal.PrevCode(tokens, i-1)
		// Never add () after '::' (Option value).
		if prev != -1 && tokens[prev].Kind == cal.TokScope {
			continue
		}

		name := tok.IdentName()
		lower := strings.ToLower(name)
		needsParens := false

		if prev != -1 && tokens[prev].Kind == cal.TokDot {
			// Qualified member access: Receiver.Name
			recvIdx := cal.PrevCode(tokens, prev-1)
			var recvVar *cal.VarDecl
			recvName := ""
			if recvIdx != -1 && (tokens[recvIdx].Kind == cal.TokIdent || tokens[recvIdx].Kind == cal.TokQuotedIdent) {
				recvName = tokens[recvIdx].IdentName()
				recvVar = scope.vars[strings.ToLower(recvName)]
			}

			if cal.IsRecordZeroArgMethod(name) || cal.IsObjectZeroArgMethod(name) {
				needsParens = true
			} else if recvVar != nil && strings.EqualFold(recvVar.BaseType, "Codeunit") {
				// Codeunits have no fields; any member reference without '(' is a zero-arg call.
				needsParens = true
			} else if strings.EqualFold(recvName, "Rec") {
				if p, ok := osym.procs[lower]; ok && len(p.Params) == 0 {
					needsParens = true
				}
			} else if isStandaloneStatementCall(tokens, recvIdx, i) {
				// In C/AL a bare field access cannot be a standalone statement; it must be a 0-arg method call.
				needsParens = true
			}
		} else {
			// Unqualified identifier.
			// Must NOT be a variable, param, return var, global var, or DataItem.
			if _, isVar := scope.vars[lower]; isVar {
				continue
			}
			if _, isDI := osym.dataItems[lower]; isDI {
				continue
			}
			if p, ok := osym.procs[lower]; ok && len(p.Params) == 0 {
				needsParens = true
			} else if cal.IsZeroArgGlobalBuiltin(name) {
				needsParens = true
			} else if osym.hasOwnRec && cal.IsRecordZeroArgMethod(name) {
				// Implicit Rec method call in Table/Page/Codeunit with TableNo.
				if _, isField := osym.ownFields[lower]; !isField {
					needsParens = true
				}
			}
		}

		if needsParens {
			edits = append(edits, textEdit{
				start:   tok.End,
				end:     tok.End,
				newText: "()",
			})
			diags = append(diags, Diagnostic{
				Rule:     config.RuleCallParens,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset + tok.Start),
				Message:  fmt.Sprintf("added () to zero-argument call '%s'", name),
				Fixed:    true,
			})
		}
	}

	return applyTextEdits(body, edits), diags
}

// isStandaloneStatementCall checks whether the expression ending at identIdx (starting at receiver)
// occupies a standalone statement slot (e.g. "Obj.DoSomething;").
func isStandaloneStatementCall(tokens []cal.Token, recvIdx, identIdx int) bool {
	if recvIdx < 0 {
		return false
	}
	// Walk backward across chained dots (e.g. A.B.C) or brackets (A[1].C) to start of chain.
	start := recvIdx
	for start > 0 {
		p := cal.PrevCode(tokens, start-1)
		if p == -1 {
			break
		}
		if tokens[p].Kind == cal.TokDot {
			prevRec := cal.PrevCode(tokens, p-1)
			if prevRec != -1 {
				start = prevRec
				continue
			}
		}
		break
	}
	before := cal.PrevCode(tokens, start-1)
	after := cal.NextCode(tokens, identIdx+1)

	beforeOK := before == -1 ||
		tokens[before].Kind == cal.TokSemi ||
		tokens[before].Kind == cal.TokColon ||
		tokens[before].IsWord("BEGIN") ||
		tokens[before].IsWord("THEN") ||
		tokens[before].IsWord("ELSE") ||
		tokens[before].IsWord("DO") ||
		tokens[before].IsWord("REPEAT")

	afterOK := after == -1 ||
		tokens[after].Kind == cal.TokSemi ||
		tokens[after].IsWord("END") ||
		tokens[after].IsWord("ELSE") ||
		tokens[after].IsWord("UNTIL")

	return beforeOK && afterOK
}

// runRedundantParensRule implements CAL021: safely remove redundant parentheses
// while strictly preserving required parentheses for C/AL operator precedence (e.g. (A = 1) AND (B = 2)).
func runRedundantParensRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	// Embedded triggers.
	for _, tr := range obj.Triggers {
		us := fscope.TriggerScope(tr)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		td := cal.ParseTrigger(tr.Raw)
		if td.BodyStart < 0 || td.BodyEnd <= td.BodyStart {
			continue
		}
		body := tr.Raw[td.BodyStart:td.BodyEnd]
		newBody, d := fixRedundantParensInBody(body, us, obj, tr.StartOffset+td.BodyStart)
		diags = append(diags, d...)
		if newBody != body {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset + td.BodyStart,
				End:     tr.StartOffset + td.BodyEnd,
				NewText: newBody,
			})
		}
	}

	// CODE procedures.
	if obj.Code != nil {
		cs := cal.ParseCodeSection(obj.Code.Raw)
		var codeEdits []textEdit
		for _, proc := range cs.Procedures {
			us := fscope.ProcScope(proc)
			if us.Mode == diffscope.ModeSkip {
				continue
			}
			if proc.BodyStart < 0 || proc.BodyEnd <= proc.BodyStart {
				continue
			}
			body := obj.Code.Raw[proc.BodyStart:proc.BodyEnd]
			newBody, d := fixRedundantParensInBody(body, us, obj, obj.Code.StartOffset+proc.BodyStart)
			diags = append(diags, d...)
			if newBody != body {
				codeEdits = append(codeEdits, textEdit{
					start:   proc.BodyStart,
					end:     proc.BodyEnd,
					newText: newBody,
				})
			}
		}
		if len(codeEdits) > 0 {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   obj.Code.StartOffset,
				End:     obj.Code.EndOffset,
				NewText: applyTextEdits(obj.Code.Raw, codeEdits),
			})
		}
	}

	return spanEdits, diags
}

func fixRedundantParensInBody(body string, us *diffscope.UnitScope, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
	var diags []Diagnostic
	cur := body

	for pass := 0; pass < 10; pass++ {
		tokens := cal.Lex(cur)
		var changedLines map[int]bool
		if us != nil && us.Mode == diffscope.ModePartial {
			changedLines = us.ChangedBodyLines(cur)
		}
		lpIdx, rpIdx, reason := findRedundantParenPair(tokens, changedLines)
		if lpIdx == -1 {
			break
		}
		lineNum := obj.LineOfOffset(fileBaseOffset + tokens[lpIdx].Start)
		diags = append(diags, Diagnostic{
			Rule:     config.RuleRedundantParens,
			Severity: SeverityInfo,
			Line:     lineNum,
			Message:  reason,
			Fixed:    true,
		})

		// Remove '(' at lpIdx and ')' at rpIdx.
		edits := []textEdit{
			{start: tokens[lpIdx].Start, end: tokens[lpIdx].End, newText: ""},
			{start: tokens[rpIdx].Start, end: tokens[rpIdx].End, newText: ""},
		}
		cur = applyTextEdits(cur, edits)
	}

	return cur, diags
}

func findRedundantParenPair(tokens []cal.Token, changedLines map[int]bool) (int, int, string) {
	for i, tok := range tokens {
		if tok.Kind != cal.TokLParen {
			continue
		}
		rp := matchParen(tokens, i)
		if rp <= i {
			continue
		}
		if changedLines != nil && (!changedLines[tok.Line] || !changedLines[tokens[rp].Line]) {
			continue
		}

		// Case 1: Double parentheses ((expr)) -> strip the INNER redundant pair so Func((x)) -> Func(x) and ((x)) -> (x).
		innerFirst := cal.NextCode(tokens, i+1)
		if innerFirst != -1 && innerFirst < rp && tokens[innerFirst].Kind == cal.TokLParen {
			innerRP := matchParen(tokens, innerFirst)
			if innerRP > innerFirst && innerRP < rp {
				afterInner := cal.NextCode(tokens, innerRP+1)
				if afterInner == rp {
					return innerFirst, innerRP, "removed duplicate parentheses"
				}
			}
		}

		prev := cal.PrevCode(tokens, i-1)
		next := cal.NextCode(tokens, rp+1)

		// Never strip parens belonging to a function call or EXIT(val).
		if isCallLParen(tokens, prev) {
			continue
		}

		// Case 2: Whole condition wrapped in parens: IF (expr) THEN, WHILE (expr) DO, UNTIL (expr);
		if prev != -1 && next != -1 {
			if tokens[prev].IsWord("IF") && tokens[next].IsWord("THEN") {
				return i, rp, "removed redundant outer parentheses in IF condition"
			}
			if tokens[prev].IsWord("WHILE") && tokens[next].IsWord("DO") {
				return i, rp, "removed redundant outer parentheses in WHILE condition"
			}
			if tokens[prev].IsWord("UNTIL") && (tokens[next].Kind == cal.TokSemi || tokens[next].IsWord("END") || tokens[next].IsWord("ELSE")) {
				return i, rp, "removed redundant outer parentheses in UNTIL condition"
			}
		}

		// Case 3: Single atomic expression wrapped in parens: (FlagL), (123), (Rec.Field), (TODAY()).
		if isSingleAtom(tokens, i+1, rp) {
			return i, rp, "removed redundant parentheses around atomic expression"
		}
	}
	return -1, -1, ""
}

func matchParen(tokens []cal.Token, lpIdx int) int {
	depth := 0
	for i := lpIdx; i < len(tokens); i++ {
		if tokens[i].Kind == cal.TokLParen {
			depth++
		} else if tokens[i].Kind == cal.TokRParen {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isCallLParen(tokens []cal.Token, prevIdx int) bool {
	if prevIdx < 0 {
		return false
	}
	prev := tokens[prevIdx]
	if prev.Kind == cal.TokQuotedIdent || prev.Kind == cal.TokRBracket || prev.Kind == cal.TokRParen {
		return true
	}
	if prev.Kind == cal.TokIdent {
		if prev.IsWord("EXIT") {
			return true
		}
		// Keywords like IF, AND, OR, NOT, WHILE, UNTIL, CASE, IN, DIV, MOD, XOR are NOT function calls.
		if !cal.IsStatementKeyword(prev.Text) {
			return true
		}
	}
	return false
}

// isSingleAtom reports whether tokens[start:end] is a single atomic primary expression
// (literal, TRUE/FALSE, identifier, member chain A.B, option A::B, array index A[i], or call F(...))
// with NO top-level unary/binary operators or commas.
func isSingleAtom(tokens []cal.Token, start, end int) bool {
	first := cal.NextCode(tokens, start)
	if first == -1 || first >= end {
		return false
	}

	// Must start with literal, TRUE/FALSE, or non-keyword identifier.
	ft := tokens[first]
	switch ft.Kind {
	case cal.TokNumber, cal.TokString, cal.TokQuotedIdent:
		// OK
	case cal.TokIdent:
		if cal.IsStatementKeyword(ft.Text) && !ft.IsWord("TRUE") && !ft.IsWord("FALSE") {
			return false
		}
	default:
		return false
	}

	// Walk remaining tokens at paren/bracket depth 0; allow only '.', '::', '(...)' call args, '[...]' indexers,
	// and identifiers immediately following '.' or '::'.
	pDepth := 0
	bDepth := 0
	prevCode := first
	for i := first + 1; i < end; i++ {
		tok := tokens[i]
		if tok.IsTrivia() {
			continue
		}
		if pDepth > 0 {
			if tok.Kind == cal.TokLParen {
				pDepth++
			} else if tok.Kind == cal.TokRParen {
				pDepth--
			}
			prevCode = i
			continue
		}
		if bDepth > 0 {
			if tok.Kind == cal.TokLBracket {
				bDepth++
			} else if tok.Kind == cal.TokRBracket {
				bDepth--
			}
			prevCode = i
			continue
		}

		switch tok.Kind {
		case cal.TokDot, cal.TokScope:
			// OK
		case cal.TokLParen:
			// Only allowed as call argument list after ident/quoted-ident/']'/')'
			if tokens[prevCode].Kind != cal.TokIdent && tokens[prevCode].Kind != cal.TokQuotedIdent && tokens[prevCode].Kind != cal.TokRBracket {
				return false
			}
			pDepth = 1
		case cal.TokLBracket:
			bDepth = 1
		case cal.TokIdent, cal.TokQuotedIdent:
			if tokens[prevCode].Kind != cal.TokDot && tokens[prevCode].Kind != cal.TokScope {
				return false
			}
		default:
			return false
		}
		prevCode = i
	}

	return pDepth == 0 && bDepth == 0
}
