package rules

import (
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

// runFormatRules executes CAL033 (keyword-casing), CAL034 (blank-lines-and-semicolons),
// CAL032 (spacing), CAL031 (begin-placement), and CAL030 (indentation).
func runFormatRules(obj *objectfile.ObjectFile, cfg *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	var cs *cal.CodeSection
	if obj.Code != nil {
		cs = cal.ParseCodeSection(obj.Code.Raw)
	}
	osym := buildObjectSymbols(obj, cs)

	// Embedded triggers.
	for _, tr := range obj.Triggers {
		us := fscope.TriggerScope(tr)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		newRaw, d := formatTrigger(tr, cfg, obj, us, osym)
		diags = append(diags, d...)
		if newRaw != tr.Raw {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset,
				End:     tr.EndOffset,
				NewText: newRaw,
			})
		}
	}

	// CODE section.
	if obj.Code != nil {
		newCode, d := formatCodeSection(obj.Code, cfg, obj, fscope, cs, osym)
		diags = append(diags, d...)
		if newCode != obj.Code.Raw {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   obj.Code.StartOffset,
				End:     obj.Code.EndOffset,
				NewText: newCode,
			})
		}
	}

	return spanEdits, diags
}

func formatTrigger(tr objectfile.TriggerBlock, cfg *config.Config, obj *objectfile.ObjectFile, us *diffscope.UnitScope, osym *objectSymbols) (string, []Diagnostic) {
	cur := tr.Raw
	var diags []Diagnostic
	eol := obj.LineEnding

	td := cal.ParseTrigger(cur)
	if td.BodyStart < 0 || td.BodyEnd <= td.BodyStart {
		return cur, nil
	}
	scope := buildLocalScope(nil, nil, td.LocalVars, osym)

	var edits []textEdit

	// Format local VAR section (in ModePartial, only format newly added local var declarations).
	if td.LocalVars != nil {
		if us.Mode == diffscope.ModeFull {
			varRaw := cur[td.LocalVars.StartOffset:td.LocalVars.EndOffset]
			newVar := formatVarSection(varRaw, tr.BaseIndent, true, cfg, eol)
			if newVar != varRaw {
				edits = append(edits, textEdit{start: td.LocalVars.StartOffset, end: td.LocalVars.EndOffset, newText: newVar})
			}
		} else if us.Mode == diffscope.ModePartial {
			edits = append(edits, formatPartialVarDecls(cur, td.LocalVars, tr.BaseIndent, cfg, us)...)
		}
	}

	bodySliceStart := td.BodyStart
	bodyFirstLineNoIndent := td.LocalVars == nil
	if td.LocalVars != nil {
		if ls := lineStartPos(cur, td.BodyStart); ls >= td.LocalVars.EndOffset {
			bodySliceStart = ls
		}
	}
	body := cur[bodySliceStart:td.BodyEnd]
	newBody, bodyDiags := formatCodeBody(body, tr.BaseIndent, bodyFirstLineNoIndent, cfg, obj, us, tr.StartOffset+bodySliceStart, scope)
	diags = append(diags, bodyDiags...)
	if newBody != body {
		edits = append(edits, textEdit{start: bodySliceStart, end: td.BodyEnd, newText: newBody})
	}

	return applyTextEdits(cur, edits), diags
}

func formatCodeSection(cb *objectfile.CodeBlock, cfg *config.Config, obj *objectfile.ObjectFile, fscope *diffscope.FileScope, cs *cal.CodeSection, osym *objectSymbols) (string, []Diagnostic) {
	var edits []textEdit
	var diags []Diagnostic
	eol := obj.LineEnding

	// Global VAR section (only reformat in full-file mode; in diff mode don't reindent untouched old globals).
	if cs.GlobalVars != nil && (fscope == nil || !fscope.Enabled) {
		varRaw := cb.Raw[cs.GlobalVars.StartOffset:cs.GlobalVars.EndOffset]
		newVar := formatVarSection(varRaw, 4, false, cfg, eol)
		if newVar != varRaw {
			edits = append(edits, textEdit{start: cs.GlobalVars.StartOffset, end: cs.GlobalVars.EndOffset, newText: newVar})
			diags = append(diags, Diagnostic{
				Rule:     config.RuleIndentation,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(cb.StartOffset + cs.GlobalVars.StartOffset),
				Message:  "formatted global VAR section",
				Fixed:    true,
			})
		}
	}

	for _, proc := range cs.Procedures {
		us := fscope.ProcScope(proc)
		if us.Mode == diffscope.ModeSkip {
			continue
		}

		// Procedure header keyword casing (CAL033) - only in ModeFull.
		if us.Mode == diffscope.ModeFull && cfg.IsRuleEnabled(config.RuleKeywordCasing) && proc.HeaderEnd > proc.HeaderStart {
			hdr := cb.Raw[proc.HeaderStart:proc.HeaderEnd]
			newHdr := uppercaseDeclKeywords(hdr)
			if newHdr != hdr {
				edits = append(edits, textEdit{start: proc.HeaderStart, end: proc.HeaderEnd, newText: newHdr})
				diags = append(diags, Diagnostic{
					Rule:     config.RuleKeywordCasing,
					Severity: SeverityInfo,
					Line:     obj.LineOfOffset(cb.StartOffset + proc.HeaderStart),
					Message:  "uppercased C/AL keywords in procedure header",
					Fixed:    true,
				})
			}
		}

		// Local VAR section.
		if proc.LocalVars != nil {
			if us.Mode == diffscope.ModeFull {
				varRaw := cb.Raw[proc.LocalVars.StartOffset:proc.LocalVars.EndOffset]
				newVar := formatVarSection(varRaw, 4, false, cfg, eol)
				if newVar != varRaw {
					edits = append(edits, textEdit{start: proc.LocalVars.StartOffset, end: proc.LocalVars.EndOffset, newText: newVar})
				}
			} else if us.Mode == diffscope.ModePartial {
				edits = append(edits, formatPartialVarDecls(cb.Raw, proc.LocalVars, 4, cfg, us)...)
			}
		}

		// Procedure body (Documentation block at cs.DocStart..cs.DocEnd is untouched!).
		if proc.BodyStart >= 0 && proc.BodyEnd > proc.BodyStart {
			scope := buildLocalScope(proc.Params, proc.ReturnVar, proc.LocalVars, osym)
			bodyLineStart := lineStartPos(cb.Raw, proc.BodyStart)
			body := cb.Raw[bodyLineStart:proc.BodyEnd]
			newBody, bodyDiags := formatCodeBody(body, 4, false, cfg, obj, us, cb.StartOffset+bodyLineStart, scope)
			diags = append(diags, bodyDiags...)
			if newBody != body {
				edits = append(edits, textEdit{start: bodyLineStart, end: proc.BodyEnd, newText: newBody})
			}
		}
	}

	return applyTextEdits(cb.Raw, edits), diags
}

func formatPartialVarDecls(raw string, vs *cal.VarSection, baseIndent int, cfg *config.Config, us *diffscope.UnitScope) []textEdit {
	var edits []textEdit
	indentSize := cfg.IndentSize()
	for _, vd := range vs.Decls {
		if !us.IsNewLocalVar(vd.Name) {
			continue
		}
		orig := raw[vd.StartOffset:vd.EndOffset]
		formatted := orig
		if cfg.IsRuleEnabled(config.RuleKeywordCasing) {
			formatted = uppercaseDeclKeywords(formatted)
		}
		if cfg.IsRuleEnabled(config.RuleSpacing) {
			text, le := splitLineEOL(formatted)
			formatted = strings.TrimRight(text, " \t") + le
		}
		if cfg.IsRuleEnabled(config.RuleIndentation) {
			text, le := splitLineEOL(formatted)
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				formatted = strings.Repeat(" ", baseIndent+indentSize) + trimmed + le
			}
		}
		if formatted != orig {
			edits = append(edits, textEdit{start: vd.StartOffset, end: vd.EndOffset, newText: formatted})
		}
	}
	return edits
}

func formatVarSection(raw string, baseIndent int, firstLineNoIndent bool, cfg *config.Config, eol string) string {
	cur := raw
	protected := blockCommentLineSet(cur)
	if cfg.IsRuleEnabled(config.RuleKeywordCasing) {
		cur = uppercaseDeclKeywords(cur)
	}
	if cfg.IsRuleEnabled(config.RuleSpacing) {
		lines := splitLinesKeepEOL(cur)
		for i, l := range lines {
			if protected[i] {
				continue
			}
			text, le := splitLineEOL(l)
			lines[i] = strings.TrimRight(text, " \t") + le
		}
		cur = strings.Join(lines, "")
	}
	if cfg.IsRuleEnabled(config.RuleIndentation) {
		indentSize := cfg.IndentSize()
		lines := splitLinesKeepEOL(cur)
		for i, l := range lines {
			if protected[i] {
				continue
			}
			text, le := splitLineEOL(l)
			trimmed := strings.TrimSpace(text)
			if trimmed == "" {
				lines[i] = le
				continue
			}
			if i == 0 && firstLineNoIndent {
				lines[i] = trimmed + le
				continue
			}
			if strings.EqualFold(trimmed, "VAR") {
				lines[i] = strings.Repeat(" ", baseIndent) + trimmed + le
			} else if strings.HasPrefix(trimmed, "'") {
				// Continuation line of wrapped TextConst string
				lines[i] = strings.Repeat(" ", baseIndent+indentSize*2) + trimmed + le
			} else {
				lines[i] = strings.Repeat(" ", baseIndent+indentSize) + trimmed + le
			}
		}
		cur = strings.Join(lines, "")
	}
	_ = eol
	return cur
}

func formatCodeBody(body string, baseIndent int, firstLineNoIndent bool, cfg *config.Config, obj *objectfile.ObjectFile, us *diffscope.UnitScope, fileBaseOffset int, scope *localScope) (string, []Diagnostic) {
	cur := body
	var diags []Diagnostic
	eol := obj.LineEnding
	isPartial := us != nil && us.Mode == diffscope.ModePartial

	getChanged := func(text string) map[int]bool {
		if !isPartial {
			return nil
		}
		return us.ChangedBodyLines(text)
	}

	// 0. CAL035 (task 6.3): remove standalone {...} block comments and
	// commented-out "//" code lines; clean empty << / >> marker pairs.
	if cfg.IsRuleEnabled(config.RuleCommentedCode) {
		next, d := applyCommentRules(cur, getChanged(cur), cfg, obj, fileBaseOffset)
		diags = append(diags, d...)
		if next != cur {
			cur = next
		}
	}

	// 0b. Task 6.1 (under CAL034): remove empty CASE arms/ELSE sections and fully
	// empty CASE ... END; constructs BEFORE the formatting passes.
	if cfg.IsRuleEnabled(config.RuleBlankLinesAndSemicols) {
		next, d := applyCaseCleanup(cur, getChanged(cur), obj, fileBaseOffset)
		diags = append(diags, d...)
		if next != cur {
			cur = next
		}
	}

	// 1. CAL033: Keyword casing (UPPERCASE).
	if cfg.IsRuleEnabled(config.RuleKeywordCasing) {
		next, changed := applyKeywordCasing(cur, getChanged(cur), scope)
		if changed {
			cur = next
			diags = append(diags, Diagnostic{
				Rule:     config.RuleKeywordCasing,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset),
				Message:  "uppercased C/AL keywords",
				Fixed:    true,
			})
		}
	}

	// 2. CAL034: Duplicate semicolons & excessive blank lines.
	if cfg.IsRuleEnabled(config.RuleBlankLinesAndSemicols) {
		next, changed := applyBlankLinesAndSemicolons(cur, getChanged(cur))
		if changed {
			cur = next
			diags = append(diags, Diagnostic{
				Rule:     config.RuleBlankLinesAndSemicols,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset),
				Message:  "cleaned duplicate semicolons / extra blank lines",
				Fixed:    true,
			})
		}
	}

	// 3. CAL031: BEGIN placement (same line for single-line IF/WHILE/FOR/ELSE, next line directly under IF for multi-line conditions).
	if cfg.IsRuleEnabled(config.RuleBeginPlacement) {
		next, changed := applyBeginPlacement(cur, eol, getChanged(cur))
		if changed {
			cur = next
			diags = append(diags, Diagnostic{
				Rule:     config.RuleBeginPlacement,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset),
				Message:  "adjusted BEGIN placement",
				Fixed:    true,
			})
		}
	}

	// 4. CAL032: Spacing around operators, commas, parens, brackets, trailing spaces.
	if cfg.IsRuleEnabled(config.RuleSpacing) {
		next, changed := applySpacing(cur, getChanged(cur))
		if changed {
			cur = next
			diags = append(diags, Diagnostic{
				Rule:     config.RuleSpacing,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset),
				Message:  "normalized spacing",
				Fixed:    true,
			})
		}
	}

	// 5. CAL030: Structural indentation (full mode) or anchored Variant A indentation (partial large-function mode).
	if cfg.IsRuleEnabled(config.RuleIndentation) {
		var next string
		var changed bool
		if isPartial {
			next, changed = applyAnchoredIndentation(cur, baseIndent, cfg.IndentSize(), firstLineNoIndent, getChanged(cur))
		} else {
			next, changed = applyIndentation(cur, baseIndent, cfg.IndentSize(), firstLineNoIndent)
		}
		if changed {
			cur = next
			diags = append(diags, Diagnostic{
				Rule:     config.RuleIndentation,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset),
				Message:  "fixed code indentation",
				Fixed:    true,
			})
		}
	}

	return cur, diags
}

// --- CAL033: Keyword Casing ---

var pascalCaseBuiltinObjects = map[string]bool{
	"REC":         true,
	"XREC":        true,
	"CURRPAGE":    true,
	"CURRREPORT":  true,
	"CURRXMLPORT": true,
	"DATABASE":    true,
	"SYSTEM":      true,
}

func applyKeywordCasing(body string, changedLines map[int]bool, scope *localScope) (string, bool) {
	tokens := cal.Lex(body)
	var edits []textEdit
	for i, tok := range tokens {
		if tok.Kind != cal.TokIdent {
			continue
		}
		if changedLines != nil && !changedLines[tok.Line] {
			continue
		}
		prev := cal.PrevCode(tokens, i-1)
		next := cal.NextCode(tokens, i+1)

		if prev != -1 && tokens[prev].Kind == cal.TokScope {
			continue
		}
		if prev != -1 && tokens[prev].Kind == cal.TokDot {
			// DotNet/Automation/OCX members are case-sensitive: never touch them (task 1.2).
			if isInteropVarDecl(chainRootVar(tokens, cal.PrevCode(tokens, prev-1), scope)) {
				continue
			}
			// Uppercase built-in Record/Object method calls when invoked with '('.
			if next != -1 && tokens[next].Kind == cal.TokLParen && (cal.IsRecordMethod(tok.Text) || cal.IsObjectZeroArgMethod(tok.Text)) {
				upper := strings.ToUpper(tok.Text)
				if tok.Text != upper {
					edits = append(edits, textEdit{start: tok.Start, end: tok.End, newText: upper})
				}
			}
			continue
		}

		upper := strings.ToUpper(tok.Text)
		if cal.IsStatementKeyword(tok.Text) {
			if tok.Text != upper {
				edits = append(edits, textEdit{start: tok.Start, end: tok.End, newText: upper})
			}
			continue
		}
		// Task 1.5: the left side of ':='/'+='/'-='/'*='/'/=' is an assignment
		// target, never a builtin call reference; do not uppercase it.
		if next != -1 {
			switch tokens[next].Kind {
			case cal.TokAssign, cal.TokPlusAssign, cal.TokMinusAssign, cal.TokMulAssign, cal.TokDivAssign:
				continue
			}
		}
		// Task 1.5 (review fix 7): never uppercase declared variables/parameters/
		// Return Value, own-table fields, object procedures or DataItems
		// (e.g. field "Time"/UserID on the current Table must stay as declared).
		if scope.knownName(strings.ToLower(tok.IdentName())) {
			continue
		}
		if !pascalCaseBuiltinObjects[upper] && cal.IsGlobalBuiltin(tok.Text) {
			if (next != -1 && tokens[next].Kind == cal.TokLParen) || cal.IsZeroArgGlobalBuiltin(tok.Text) {
				if tok.Text != upper {
					edits = append(edits, textEdit{start: tok.Start, end: tok.End, newText: upper})
				}
			}
		}
	}
	if len(edits) == 0 {
		return body, false
	}
	return applyTextEdits(body, edits), true
}

func uppercaseDeclKeywords(src string) string {
	tokens := cal.Lex(src)
	var edits []textEdit
	for i, tok := range tokens {
		if tok.Kind != cal.TokIdent {
			continue
		}
		if isMemberAccess(tokens, i) {
			continue
		}
		// Only uppercase declaration keywords when they appear in keyword positions (not variable/proc names before '@').
		next := cal.NextCode(tokens, i+1)
		if next != -1 && tokens[next].Kind == cal.TokAt {
			continue
		}
		if cal.IsDeclKeyword(tok.Text) {
			upper := strings.ToUpper(tok.Text)
			if tok.Text != upper {
				edits = append(edits, textEdit{start: tok.Start, end: tok.End, newText: upper})
			}
		}
	}
	return applyTextEdits(src, edits)
}

// --- CAL034: Duplicate Semicolons & Blank Lines ---

func applyBlankLinesAndSemicolons(body string, changedLines map[int]bool) (string, bool) {
	cur := body

	// 1. Collapse duplicate semicolons (e.g. ";;" or "; ;") outside strings/comments.
	tokens := cal.Lex(cur)
	var edits []textEdit
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != cal.TokSemi {
			continue
		}
		if changedLines != nil && !changedLines[tokens[i].Line] {
			continue
		}
		next := cal.NextCode(tokens, i+1)
		if next != -1 && tokens[next].Kind == cal.TokSemi {
			if changedLines != nil && !changedLines[tokens[next].Line] {
				continue
			}
			// Check no comment between i and next.
			hasComment := false
			for k := i + 1; k < next; k++ {
				if tokens[k].IsComment() {
					hasComment = true
					break
				}
			}
			if !hasComment {
				edits = append(edits, textEdit{start: tokens[i].End, end: tokens[next].End, newText: ""})
			}
		}
	}
	if len(edits) > 0 {
		cur = applyTextEdits(cur, edits)
	}

	// 2. Collapse consecutive blank lines (2+ blank lines -> 1 blank line).
	lines := splitLinesKeepEOL(cur)
	protected := blockCommentLineSet(cur)
	var out []string
	blankRun := 0
	for idx, l := range lines {
		text, le := splitLineEOL(l)
		if !protected[idx] && strings.TrimSpace(text) == "" {
			blankRun++
			if blankRun <= 1 {
				out = append(out, le)
			} else if changedLines != nil && !changedLines[idx+1] {
				out = append(out, l)
			}
		} else {
			blankRun = 0
			out = append(out, l)
		}
	}
	res := strings.Join(out, "")
	return res, res != body
}

// --- CAL031: BEGIN Placement ---

func applyBeginPlacement(body string, eol string, changedLines map[int]bool) (string, bool) {
	tokens := cal.Lex(body)
	var edits []textEdit

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.IsTrivia() || isMemberAccess(tokens, i) {
			continue
		}
		if changedLines != nil && !changedLines[tok.Line] {
			continue
		}

		if tok.IsWord("IF") || tok.IsWord("WHILE") {
			closerWord := "THEN"
			if tok.IsWord("WHILE") {
				closerWord = "DO"
			}
			closerIdx := findConditionCloser(tokens, i+1, closerWord)
			if closerIdx == -1 {
				continue
			}
			beginIdx := cal.NextCode(tokens, closerIdx+1)
			if beginIdx == -1 || !tokens[beginIdx].IsWord("BEGIN") {
				continue
			}

			isMultilineCond := hasNewlineBetween(tokens, i, closerIdx)
			hasNewlineBeforeBegin := hasNewlineBetween(tokens, closerIdx, beginIdx)
			hasCommentBeforeBegin := hasCommentBetween(tokens, closerIdx, beginIdx)

			if isMultilineCond {
				// User rule 2: BEGIN must be on a new line directly under IF (not on the same line as THEN).
				if !hasNewlineBeforeBegin && !hasCommentBeforeBegin {
					ifIndent := leadingSpacesOfOffset(body, tok.Start)
					edits = append(edits, textEdit{
						start:   tokens[closerIdx].End,
						end:     tokens[beginIdx].Start,
						newText: eol + ifIndent,
					})
				}
			} else {
				// Single-line condition: IF Cond THEN BEGIN on the same line.
				if hasNewlineBeforeBegin && !hasCommentBeforeBegin {
					edits = append(edits, textEdit{
						start:   tokens[closerIdx].End,
						end:     tokens[beginIdx].Start,
						newText: " ",
					})
				}
			}
		} else if tok.IsWord("FOR") {
			doIdx := findConditionCloser(tokens, i+1, "DO")
			if doIdx == -1 {
				continue
			}
			beginIdx := cal.NextCode(tokens, doIdx+1)
			if beginIdx == -1 || !tokens[beginIdx].IsWord("BEGIN") {
				continue
			}
			if !hasNewlineBetween(tokens, i, doIdx) && hasNewlineBetween(tokens, doIdx, beginIdx) && !hasCommentBetween(tokens, doIdx, beginIdx) {
				edits = append(edits, textEdit{
					start:   tokens[doIdx].End,
					end:     tokens[beginIdx].Start,
					newText: " ",
				})
			}
		} else if tok.IsWord("ELSE") {
			beginIdx := cal.NextCode(tokens, i+1)
			if beginIdx != -1 && tokens[beginIdx].IsWord("BEGIN") {
				if hasNewlineBetween(tokens, i, beginIdx) && !hasCommentBetween(tokens, i, beginIdx) {
					edits = append(edits, textEdit{
						start:   tok.End,
						end:     tokens[beginIdx].Start,
						newText: " ",
					})
				}
			}
		}
	}

	if len(edits) == 0 {
		return body, false
	}
	return applyTextEdits(body, edits), true
}

func findConditionCloser(tokens []cal.Token, start int, word string) int {
	pDepth := 0
	bDepth := 0
	for i := start; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.IsTrivia() {
			continue
		}
		switch tok.Kind {
		case cal.TokLParen:
			pDepth++
		case cal.TokRParen:
			if pDepth > 0 {
				pDepth--
			}
		case cal.TokLBracket:
			bDepth++
		case cal.TokRBracket:
			if bDepth > 0 {
				bDepth--
			}
		case cal.TokSemi:
			if pDepth == 0 && bDepth == 0 {
				return -1
			}
		}
		if pDepth == 0 && bDepth == 0 && tok.IsWord(word) && !isMemberAccess(tokens, i) {
			return i
		}
	}
	return -1
}

func hasNewlineBetween(tokens []cal.Token, a, b int) bool {
	for i := a + 1; i < b; i++ {
		if tokens[i].Kind == cal.TokNewline {
			return true
		}
	}
	return false
}

func hasCommentBetween(tokens []cal.Token, a, b int) bool {
	for i := a + 1; i < b; i++ {
		if tokens[i].IsComment() {
			return true
		}
	}
	return false
}

func leadingSpacesOfOffset(s string, offset int) string {
	ls := lineStartPos(s, offset)
	i := ls
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[ls:i]
}

// --- CAL032: Spacing ---

func applySpacing(body string, changedLines map[int]bool) (string, bool) {
	lines := splitLinesKeepEOL(body)
	protected := blockCommentLineSet(body)
	var out []string
	changed := false

	for idx, line := range lines {
		if protected[idx] {
			// Never re-space content inside {...} block comments (task 6.3).
			out = append(out, line)
			continue
		}
		if changedLines != nil && !changedLines[idx+1] {
			out = append(out, line)
			continue
		}
		text, le := splitLineEOL(line)
		formatted := formatLineSpacing(text)
		if formatted != text {
			changed = true
		}
		out = append(out, formatted+le)
	}
	return strings.Join(out, ""), changed
}

func formatLineSpacing(line string) string {
	// Keep leading indentation untouched (CAL030 owns indentation).
	leadEnd := 0
	for leadEnd < len(line) && (line[leadEnd] == ' ' || line[leadEnd] == '\t') {
		leadEnd++
	}
	lead := line[:leadEnd]
	rest := strings.TrimRight(line[leadEnd:], " \t")
	if rest == "" {
		return ""
	}

	tokens := cal.Lex(rest)
	// Strip pure whitespace tokens; keep comments and code tokens.
	var items []cal.Token
	for _, t := range tokens {
		if t.Kind == cal.TokWhitespace || t.Kind == cal.TokNewline {
			continue
		}
		items = append(items, t)
	}
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(lead)
	for i, tok := range items {
		if i > 0 {
			prev := items[i-1]
			var prevPrev *cal.Token
			if i >= 2 {
				prevPrev = &items[i-2]
			}
			if needsSpaceBetween(prevPrev, prev, tok) {
				b.WriteByte(' ')
			}
		}
		b.WriteString(tok.Text)
	}
	return b.String()
}

func needsSpaceBetween(prevPrev *cal.Token, prev, curr cal.Token) bool {
	// Always keep a space before/after inline comments.
	if prev.IsComment() || curr.IsComment() {
		return true
	}

	// Never space before: , ; ) ] . .. ::
	switch curr.Kind {
	case cal.TokComma, cal.TokSemi, cal.TokRParen, cal.TokRBracket, cal.TokDot, cal.TokRange, cal.TokScope, cal.TokColon:
		return false
	}

	// Never space after: ( [ . .. ::
	switch prev.Kind {
	case cal.TokLParen, cal.TokLBracket, cal.TokDot, cal.TokRange, cal.TokScope:
		return false
	}

	// Unary + or -: no space between unary sign and its operand!
	if (prev.Kind == cal.TokMinus || prev.Kind == cal.TokPlus) && isUnarySign(prevPrev) {
		return false
	}

	// Before '(': no space for function calls or EXIT(...), space after keywords/operators.
	if curr.Kind == cal.TokLParen {
		if prev.Kind == cal.TokQuotedIdent || prev.Kind == cal.TokRBracket || prev.Kind == cal.TokRParen {
			return false
		}
		if prev.Kind == cal.TokIdent {
			if prev.IsWord("EXIT") || !cal.IsStatementKeyword(prev.Text) {
				return false
			}
			return true
		}
		if prev.Kind == cal.TokComma || isBinaryOpToken(prev.Kind) {
			return true
		}
		return false
	}

	// Before '[': no space for indexing Arr[1], space after IN / ARRAY / operators / keywords.
	if curr.Kind == cal.TokLBracket {
		if prev.Kind == cal.TokIdent && (prev.IsWord("IN") || prev.IsWord("ARRAY") || cal.IsStatementKeyword(prev.Text)) {
			return true
		}
		if isBinaryOpToken(prev.Kind) || prev.Kind == cal.TokComma {
			return true
		}
		return false
	}

	// Space after comma or colon (e.g. CASE label "1: DoOne();")
	if prev.Kind == cal.TokComma || prev.Kind == cal.TokColon {
		return true
	}

	// Space around binary operators (including :=, =, <>, +, -, *, /, etc.)
	if isBinaryOpToken(prev.Kind) || isBinaryOpToken(curr.Kind) {
		return true
	}

	// Space between adjacent words/numbers/strings/closing parens.
	return true
}

func isUnarySign(beforeSign *cal.Token) bool {
	if beforeSign == nil {
		return true
	}
	switch beforeSign.Kind {
	case cal.TokLParen, cal.TokLBracket, cal.TokComma, cal.TokRange, cal.TokColon,
		cal.TokAssign, cal.TokPlusAssign, cal.TokMinusAssign, cal.TokMulAssign, cal.TokDivAssign,
		cal.TokEQ, cal.TokNE, cal.TokLT, cal.TokLE, cal.TokGT, cal.TokGE,
		cal.TokPlus, cal.TokMinus, cal.TokStar, cal.TokSlash:
		return true
	case cal.TokIdent:
		return cal.IsStatementKeyword(beforeSign.Text) && !beforeSign.IsWord("TRUE") && !beforeSign.IsWord("FALSE")
	}
	return false
}

func isBinaryOpToken(k cal.TokenKind) bool {
	switch k {
	case cal.TokAssign, cal.TokPlusAssign, cal.TokMinusAssign, cal.TokMulAssign, cal.TokDivAssign,
		cal.TokEQ, cal.TokNE, cal.TokLT, cal.TokLE, cal.TokGT, cal.TokGE,
		cal.TokPlus, cal.TokMinus, cal.TokStar, cal.TokSlash:
		return true
	}
	return false
}

// --- CAL030: Structural Indentation ---

type indentFrameKind int

const (
	frameBegin indentFrameKind = iota
	frameRepeat
	frameCase
	frameSingleStmt
	frameMultilineCond
)

type indentFrame struct {
	kind             indentFrameKind
	indent           int
	bodyIndent       int
	fromMultilineIf  bool
	caseHasActiveArm bool
}

// applyAnchoredIndentation implements Variant A indentation for large existing functions (ModePartial):
// untouched old lines stay at their exact original indentation, while each contiguous hunk of new/changed lines
// anchors onto the nearest preceding old line's actual indentation (adjusted by +indentSize if that old line
// opens a block) and formats internal nesting using indentSize.
func applyAnchoredIndentation(body string, baseIndent int, indentSize int, firstLineNoIndent bool, changedLines map[int]bool) (string, bool) {
	if indentSize <= 0 {
		indentSize = 2
	}
	lines := splitLinesKeepEOL(body)
	protected := blockCommentLineSet(body)
	var out []string
	changed := false

	lastOldIndent := baseIndent
	lastOldOpensBlock := false
	lastOldMultilineIf := false
	// Stack of opener indents for pending old single-statement IF/ELSE/FOR/WHILE/CASE-label lines
	// so that once the single statement ends with ';', subsequent new lines anchor back to the opener's level!
	var pendingOldSingleStmtIndents []int

	i := 0
	for i < len(lines) {
		if protected[i] {
			// Inside a {...} block comment: emit verbatim, do not touch anchors.
			out = append(out, lines[i])
			i++
			continue
		}
		if !changedLines[i+1] {
			// Unchanged old line: preserve verbatim and update anchor state.
			out = append(out, lines[i])
			text, _ := splitLineEOL(lines[i])
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				sp := countLeadingSpaces(text)
				codeToks := nonTriviaTokens(trimmed)
				if len(codeToks) > 0 {
					first := codeToks[0]
					last := codeToks[len(codeToks)-1]

					isSingleStmtOpener := last.IsWord("THEN") || last.IsWord("ELSE") || last.IsWord("DO") || last.Kind == cal.TokColon
					opensBlock := last.IsWord("BEGIN") || last.IsWord("REPEAT") || last.IsWord("OF") || isSingleStmtOpener
					multilineIf := first.IsWord("IF") && !containsWord(codeToks, "THEN")

					if isSingleStmtOpener {
						pendingOldSingleStmtIndents = append(pendingOldSingleStmtIndents, sp)
						lastOldIndent = sp
					} else if last.Kind == cal.TokSemi && len(pendingOldSingleStmtIndents) > 0 {
						// This old statement closed one or more pending single-statement openers (e.g. IF ... THEN stmt;).
						// Subsequent lines belong at the outermost closed opener's indent!
						lastOldIndent = pendingOldSingleStmtIndents[0]
						pendingOldSingleStmtIndents = nil
					} else {
						if first.IsWord("END") || first.IsWord("UNTIL") || first.IsWord("BEGIN") {
							pendingOldSingleStmtIndents = nil
						}
						lastOldIndent = sp
					}

					lastOldOpensBlock = opensBlock
					lastOldMultilineIf = multilineIf
				}
			}
			i++
			continue
		}

		// Contiguous hunk of changed/added lines [i, j).
		j := i
		for j < len(lines) && changedLines[j+1] {
			j++
		}

		anchor := lastOldIndent
		if lastOldOpensBlock {
			anchor = lastOldIndent + indentSize
		} else if lastOldMultilineIf {
			anchor = lastOldIndent + 3
		}

		// If the first non-empty changed line in this hunk closes an enclosing old block (END/UNTIL/ELSE),
		// align it one indent level back unless the immediately preceding old line opened that block.
		if !lastOldOpensBlock {
			for k := i; k < j; k++ {
				text, _ := splitLineEOL(lines[k])
				codeToks := nonTriviaTokens(strings.TrimSpace(text))
				if len(codeToks) > 0 {
					if codeToks[0].IsWord("END") || codeToks[0].IsWord("UNTIL") || codeToks[0].IsWord("ELSE") {
						if anchor >= baseIndent+indentSize {
							anchor -= indentSize
						}
					}
					break
				}
			}
		}

		hunk := strings.Join(lines[i:j], "")
		formattedHunk, hunkChanged := applyIndentation(hunk, anchor, indentSize, i == 0 && firstLineNoIndent)
		if hunkChanged {
			changed = true
		}
		out = append(out, formattedHunk)
		i = j
	}

	return strings.Join(out, ""), changed
}

func countLeadingSpaces(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

func applyIndentation(body string, baseIndent int, indentSize int, firstLineNoIndent bool) (string, bool) {
	if indentSize <= 0 {
		indentSize = 2
	}

	lines := splitLinesKeepEOL(body)
	protected := blockCommentLineSet(body)
	var out []string
	var stack []indentFrame
	changed := false

	currentIndent := func() int {
		if len(stack) == 0 {
			return baseIndent
		}
		return stack[len(stack)-1].bodyIndent
	}

	popSingleStmts := func() {
		for len(stack) > 0 && stack[len(stack)-1].kind == frameSingleStmt {
			stack = stack[:len(stack)-1]
		}
	}

	for lineIdx, rawLine := range lines {
		if protected[lineIdx] {
			// Never re-indent content inside {...} block comments (task 6.3).
			out = append(out, rawLine)
			continue
		}
		text, le := splitLineEOL(rawLine)
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			if text != "" {
				changed = true
			}
			out = append(out, le)
			continue
		}

		codeToks := nonTriviaTokens(trimmed)

		// Comment-only line: use current indent.
		if len(codeToks) == 0 {
			ind := currentIndent()
			var rebuilt string
			if lineIdx == 0 && firstLineNoIndent {
				rebuilt = trimmed
			} else {
				rebuilt = strings.Repeat(" ", ind) + trimmed
			}
			if rebuilt != text {
				changed = true
			}
			out = append(out, rebuilt+le)
			continue
		}

		first := codeToks[0]
		last := codeToks[len(codeToks)-1]

		// Determine this line's indentation based on leading keyword/structure.
		lineIndent := currentIndent()

		// 1. Handle leading closing/continuation keywords BEFORE computing lineIndent.
		if first.IsWord("END") {
			popSingleStmts()
			if len(stack) > 0 && (stack[len(stack)-1].kind == frameBegin || stack[len(stack)-1].kind == frameCase) {
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				lineIndent = frame.indent
			} else {
				lineIndent = baseIndent
			}
		} else if first.IsWord("UNTIL") {
			popSingleStmts()
			if len(stack) > 0 && stack[len(stack)-1].kind == frameRepeat {
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				lineIndent = frame.indent
			}
		} else if first.IsWord("ELSE") {
			// Pop pending single-statement IF arm if any, or align ELSE inside CASE.
			if len(stack) > 0 && stack[len(stack)-1].kind == frameSingleStmt {
				frame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				lineIndent = frame.indent
			} else if len(stack) > 0 && stack[len(stack)-1].kind == frameCase {
				// ELSE branch of CASE aligns directly with CASE.
				stack[len(stack)-1].caseHasActiveArm = false
				lineIndent = stack[len(stack)-1].indent
			} else {
				lineIndent = currentIndent()
			}
		} else if first.IsWord("THEN") && len(stack) > 0 && stack[len(stack)-1].kind == frameMultilineCond {
			frame := stack[len(stack)-1]
			lineIndent = frame.indent
		} else if first.IsWord("BEGIN") {
			if len(stack) > 0 && stack[len(stack)-1].kind == frameSingleStmt {
				frame := stack[len(stack)-1]
				if frame.fromMultilineIf {
					// User rule 2: BEGIN directly under multiline IF!
					stack = stack[:len(stack)-1]
					lineIndent = frame.indent
				} else {
					// e.g. CASE label followed by BEGIN on next line, or standalone BEGIN
					lineIndent = frame.bodyIndent
				}
			} else if len(stack) == 0 {
				lineIndent = baseIndent
			} else {
				lineIndent = currentIndent()
			}
		} else if len(stack) > 0 && stack[len(stack)-1].kind == frameCase && isCaseLabelLine(codeToks) {
			lineIndent = stack[len(stack)-1].indent + indentSize
		} else if len(stack) > 1 && stack[len(stack)-1].kind == frameSingleStmt && stack[len(stack)-2].kind == frameCase && isCaseLabelLine(codeToks) {
			stack = stack[:len(stack)-1]
			lineIndent = stack[len(stack)-1].indent + indentSize
		}

		// Emit formatted line.
		var rebuilt string
		if lineIdx == 0 && firstLineNoIndent {
			rebuilt = trimmed
		} else {
			rebuilt = strings.Repeat(" ", lineIndent) + trimmed
		}
		if rebuilt != text {
			changed = true
		}
		out = append(out, rebuilt+le)

		// 2. Update stack state AFTER emitting this line.
		// Handle closing of multiline condition when THEN or DO appears on this line.
		if len(stack) > 0 && stack[len(stack)-1].kind == frameMultilineCond {
			if containsWord(codeToks, "THEN") || containsWord(codeToks, "DO") {
				condFrame := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if last.IsWord("BEGIN") {
					stack = append(stack, indentFrame{
						kind:       frameBegin,
						indent:     condFrame.indent,
						bodyIndent: condFrame.indent + indentSize,
					})
				} else if last.IsWord("THEN") || last.IsWord("DO") {
					stack = append(stack, indentFrame{
						kind:            frameSingleStmt,
						indent:          condFrame.indent,
						bodyIndent:      condFrame.indent + indentSize,
						fromMultilineIf: true,
					})
				}
				continue
			}
			// Still inside multiline condition; keep frameMultilineCond.
			continue
		}

		// Single-line BEGIN ... END; on the same line (e.g. "BEGIN END;")
		if first.IsWord("BEGIN") && containsWord(codeToks, "END") {
			popSingleStmts()
			continue
		}

		// Leading END followed by ELSE / ELSE BEGIN on same line (e.g. "END ELSE BEGIN" or "END ELSE").
		if first.IsWord("END") {
			if containsWord(codeToks, "ELSE") {
				if last.IsWord("BEGIN") {
					stack = append(stack, indentFrame{
						kind:       frameBegin,
						indent:     lineIndent,
						bodyIndent: lineIndent + indentSize,
					})
				} else if last.IsWord("ELSE") {
					stack = append(stack, indentFrame{
						kind:       frameSingleStmt,
						indent:     lineIndent,
						bodyIndent: lineIndent + indentSize,
					})
				} else if containsWord(codeToks, "IF") && !containsWord(codeToks, "THEN") {
					stack = append(stack, indentFrame{
						kind:       frameMultilineCond,
						indent:     lineIndent,
						bodyIndent: lineIndent + 3,
					})
				} else if last.IsWord("THEN") {
					stack = append(stack, indentFrame{
						kind:       frameSingleStmt,
						indent:     lineIndent,
						bodyIndent: lineIndent + indentSize,
					})
				}
			} else if last.Kind == cal.TokSemi {
				popSingleStmts()
			}
			continue
		}

		// Opening BEGIN
		if last.IsWord("BEGIN") {
			// If this line was a CASE label with inline BEGIN (e.g. "1: BEGIN"), body is indented under label.
			stack = append(stack, indentFrame{
				kind:       frameBegin,
				indent:     lineIndent,
				bodyIndent: lineIndent + indentSize,
			})
			continue
		}

		// Opening REPEAT
		if first.IsWord("REPEAT") || last.IsWord("REPEAT") {
			stack = append(stack, indentFrame{
				kind:       frameRepeat,
				indent:     lineIndent,
				bodyIndent: lineIndent + indentSize,
			})
			continue
		}

		// Opening CASE ... OF
		if first.IsWord("CASE") && containsWord(codeToks, "OF") {
			stack = append(stack, indentFrame{
				kind:       frameCase,
				indent:     lineIndent,
				bodyIndent: lineIndent + indentSize,
			})
			continue
		}

		// Multiline IF / WHILE condition started on this line (IF without THEN, or WHILE without DO).
		if first.IsWord("IF") && !containsWord(codeToks, "THEN") {
			stack = append(stack, indentFrame{
				kind:       frameMultilineCond,
				indent:     lineIndent,
				bodyIndent: lineIndent + 3,
			})
			continue
		}
		if first.IsWord("WHILE") && !containsWord(codeToks, "DO") {
			stack = append(stack, indentFrame{
				kind:       frameMultilineCond,
				indent:     lineIndent,
				bodyIndent: lineIndent + 6,
			})
			continue
		}

		// Single-statement openers: line ending with THEN, ELSE, DO, or CASE label ':'
		if last.IsWord("THEN") || last.IsWord("ELSE") || last.IsWord("DO") {
			stack = append(stack, indentFrame{
				kind:       frameSingleStmt,
				indent:     lineIndent,
				bodyIndent: lineIndent + indentSize,
			})
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].kind == frameCase && isCaseLabelLine(codeToks) && last.Kind == cal.TokColon {
			stack = append(stack, indentFrame{
				kind:       frameSingleStmt,
				indent:     lineIndent,
				bodyIndent: lineIndent + indentSize,
			})
			continue
		}

		// Statement ended with ';' -> pop any pending single-statement frames.
		if last.Kind == cal.TokSemi {
			popSingleStmts()
		}
	}

	return strings.Join(out, ""), changed
}

func nonTriviaTokens(line string) []cal.Token {
	all := cal.Lex(line)
	var res []cal.Token
	for _, t := range all {
		if !t.IsTrivia() {
			res = append(res, t)
		}
	}
	return res
}

func containsWord(toks []cal.Token, word string) bool {
	for i, t := range toks {
		if t.IsWord(word) && (i == 0 || (toks[i-1].Kind != cal.TokDot && toks[i-1].Kind != cal.TokScope)) {
			return true
		}
	}
	return false
}

// isCaseLabelLine checks whether a line inside CASE ... OF is a case label (contains ':' at depth 0, not ':=' or '::').
func isCaseLabelLine(toks []cal.Token) bool {
	if len(toks) == 0 {
		return false
	}
	if toks[0].IsWord("BEGIN") || toks[0].IsWord("END") || toks[0].IsWord("ELSE") ||
		toks[0].IsWord("IF") || toks[0].IsWord("FOR") || toks[0].IsWord("WHILE") || toks[0].IsWord("REPEAT") {
		return false
	}
	pDepth := 0
	bDepth := 0
	for _, t := range toks {
		switch t.Kind {
		case cal.TokLParen:
			pDepth++
		case cal.TokRParen:
			if pDepth > 0 {
				pDepth--
			}
		case cal.TokLBracket:
			bDepth++
		case cal.TokRBracket:
			if bDepth > 0 {
				bDepth--
			}
		case cal.TokColon:
			if pDepth == 0 && bDepth == 0 {
				return true
			}
		}
	}
	return false
}

func splitLinesKeepEOL(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitLineEOL(line string) (text string, eol string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}
