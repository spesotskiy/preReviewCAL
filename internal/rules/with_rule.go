package rules

import (
	"fmt"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

// runNoWithRule implements CAL005: forbid WITH ... DO and replace it by prefixing fields/methods
// with the WITH target variable.
func runNoWithRule(obj *objectfile.ObjectFile, cfg *config.Config, ws *objectfile.WorkspaceIndex, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var cs *cal.CodeSection
	if obj.Code != nil {
		cs = cal.ParseCodeSection(obj.Code.Raw)
	}
	osym := buildObjectSymbols(obj, cs)

	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	// Process embedded triggers.
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
		scope := buildLocalScope(nil, nil, td.LocalVars, osym)
		newBody, bodyDiags := eliminateWithInBody(body, scope, osym, obj, ws, us, tr.StartOffset+td.BodyStart, obj.LineEnding, cfg.IndentSize())
		diags = append(diags, bodyDiags...)
		if newBody != body {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset + td.BodyStart,
				End:     tr.StartOffset + td.BodyEnd,
				NewText: newBody,
			})
		}
	}

	// Process CODE procedures.
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
			body := obj.Code.Raw[proc.BodyStart:proc.BodyEnd]
			scope := buildLocalScope(proc.Params, proc.ReturnVar, proc.LocalVars, osym)
			newBody, bodyDiags := eliminateWithInBody(body, scope, osym, obj, ws, us, obj.Code.StartOffset+proc.BodyStart, obj.LineEnding, cfg.IndentSize())
			diags = append(diags, bodyDiags...)
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

type localScope struct {
	vars map[string]*cal.VarDecl
	// reserved: object procedures, DataItems and own-table field names (lowercased).
	// CAL033 must never UPPERCASE these either (review fix item 7).
	reserved map[string]bool
}

// knownName reports whether the lowercased identifier is a declared variable,
// object procedure, DataItem or own-table field (never a builtin reference).
func (s *localScope) knownName(lower string) bool {
	if s == nil {
		return false
	}
	if _, ok := s.vars[lower]; ok {
		return true
	}
	return s.reserved[lower]
}

func buildLocalScope(params []*cal.VarDecl, retVar *cal.VarDecl, localVars *cal.VarSection, osym *objectSymbols) *localScope {
	s := &localScope{vars: make(map[string]*cal.VarDecl), reserved: make(map[string]bool)}
	for k, v := range osym.globals {
		s.vars[k] = v
		s.reserved[k] = true
	}
	for k := range osym.procs {
		s.reserved[k] = true
	}
	for k := range osym.dataItems {
		s.reserved[k] = true
	}
	for k := range osym.ownFields {
		s.reserved[k] = true
	}
	for _, p := range params {
		s.vars[strings.ToLower(p.Name)] = p
	}
	if retVar != nil {
		s.vars[strings.ToLower(retVar.Name)] = retVar
	}
	if localVars != nil {
		for _, v := range localVars.Decls {
			s.vars[strings.ToLower(v.Name)] = v
		}
	}
	return s
}

func eliminateWithInBody(body string, scope *localScope, osym *objectSymbols, obj *objectfile.ObjectFile, ws *objectfile.WorkspaceIndex, us *diffscope.UnitScope, baseFileOffset int, eol string, indentSize int) (string, []Diagnostic) {
	var diags []Diagnostic
	cur := body

	// Iteratively eliminate innermost eligible WITH blocks first so nested WITH resolves accurately!
	// Task 1.3: a WITH block is eliminated ONLY when its target resolves to a schema from
	// --symbols-dir (or the object itself) and every identifier in the block is classified
	// (field / record method / public procedure / known variable / builtin). Otherwise the
	// block is left untouched and a warning is emitted.
	for {
		tokens := cal.Lex(cur)
		var changedLines map[int]bool
		if us != nil && us.Mode == diffscope.ModePartial {
			changedLines = us.ChangedBodyLines(cur)
		}
		cands := findAllEligibleWiths(tokens, changedLines)
		if len(cands) == 0 {
			break
		}

		progress := false
		var skipDiags []Diagnostic
		for _, withIdx := range cands { // innermost first
			doIdx := findDoAfterWith(tokens, withIdx)
			if doIdx == -1 {
				continue
			}
			recExpr := strings.TrimSpace(cur[tokens[withIdx].End:tokens[doIdx].Start])
			if recExpr == "" {
				continue
			}
			stmtStartIdx := cal.NextCode(tokens, doIdx+1)
			if stmtStartIdx == -1 {
				continue
			}
			lineNum := obj.LineOfOffset(baseFileOffset + tokens[withIdx].Start)

			prevBeforeWith := cal.PrevCode(tokens, withIdx-1)
			inSingleStmtSlot := prevBeforeWith != -1 && (tokens[prevBeforeWith].IsWord("THEN") ||
				tokens[prevBeforeWith].IsWord("ELSE") ||
				tokens[prevBeforeWith].IsWord("DO"))

			var beginIdx, endIdx, endStop, fragStart, fragEnd int
			var isBeginBlock bool
			if tokens[stmtStartIdx].IsWord("BEGIN") {
				endIdx = findMatchingEndToken(tokens, stmtStartIdx)
				if endIdx == -1 {
					continue
				}
				beginIdx = stmtStartIdx
				isBeginBlock = true
				endStop = tokens[endIdx].End
				afterEnd := cal.NextCode(tokens, endIdx+1)
				if afterEnd != -1 && tokens[afterEnd].Kind == cal.TokSemi {
					endStop = tokens[afterEnd].End
				}
				fragStart = tokens[beginIdx].End
				fragEnd = tokens[endIdx].Start
			} else {
				endIdx = findSingleStatementEnd(tokens, stmtStartIdx)
				if endIdx == -1 {
					continue
				}
				endStop = tokens[endIdx].End
				fragStart = tokens[stmtStartIdx].Start
				fragEnd = tokens[endIdx].End
			}

			// A still-unresolved inner WITH inside this block (e.g. skipped because its
			// schema is unknown) makes prefixing unsafe -> skip this block too.
			if containsWithKeyword(tokens, doIdx, endIdx) {
				skipDiags = append(skipDiags, Diagnostic{
					Rule:     config.RuleNoWith,
					Severity: SeverityWarning,
					Line:     lineNum,
					Message:  fmt.Sprintf("WITH %s DO not expanded: it contains an inner WITH block that could not be eliminated", recExpr),
					Fixed:    false,
				})
				continue
			}

			schema := resolveWithSchema(recExpr, scope, osym, obj, ws)
			if schema == nil {
				skipDiags = append(skipDiags, Diagnostic{
					Rule:     config.RuleNoWith,
					Severity: SeverityWarning,
					Line:     lineNum,
					Message:  fmt.Sprintf("WITH %s DO not expanded: schema of the target object was not found in --symbols-dir", recExpr),
					Fixed:    false,
				})
				continue
			}

			prefixed, unknowns := prefixWithIdentifiers(cur[fragStart:fragEnd], recExpr, scope, osym, schema)
			if len(unknowns) > 0 {
				quoted := make([]string, len(unknowns))
				for k, u := range unknowns {
					quoted[k] = "'" + u + "'"
				}
				skipDiags = append(skipDiags, Diagnostic{
					Rule:     config.RuleNoWith,
					Severity: SeverityWarning,
					Line:     lineNum,
					Message: fmt.Sprintf("WITH %s DO not expanded: %s not found in schema of %s %d (--symbols-dir)",
						recExpr, strings.Join(quoted, ", "), schema.ObjectType, schema.TableID),
					Fixed: false,
				})
				continue
			}

			if isBeginBlock {
				if inSingleStmtSlot {
					// Keep BEGIN ... END[;] because WITH was the body of IF/ELSE/FOR/WHILE!
					// Replace only "WITH <recExpr> DO" + whitespace before BEGIN.
					var b strings.Builder
					b.WriteString(cur[:tokens[withIdx].Start])
					b.WriteString(cur[tokens[beginIdx].Start:fragStart])
					b.WriteString(prefixed)
					b.WriteString(cur[fragEnd:])
					cur = b.String()
				} else {
					// Unwrap WITH ... DO BEGIN ... END;
					unwrapped := unwrapWithBeginBody(prefixed, eol, indentSize)
					withLineStart := lineStartPos(cur, tokens[withIdx].Start)
					endLineEnd := lineEndPos(cur, endStop)

					// Preserve leading indentation/comments before WITH if WITH wasn't first on line.
					prefix := cur[:withLineStart]
					if strings.TrimSpace(cur[withLineStart:tokens[withIdx].Start]) != "" {
						prefix = cur[:tokens[withIdx].Start]
					}
					// Check if trailing text after END; on the same line exists (e.g. inline comment).
					trailingOnEndLine := strings.TrimSpace(cur[endStop:endLineEnd])
					suffix := cur[endLineEnd:]
					if trailingOnEndLine != "" {
						suffix = cur[endStop:]
					}

					cur = prefix + unwrapped + suffix
				}
			} else {
				// Single-statement WITH <RecExpr> DO <Stmt>;
				// Preserve indent of WITH line if statement was on the next line.
				withLineStart := lineStartPos(cur, tokens[withIdx].Start)
				withIndent := cur[withLineStart:tokens[withIdx].Start]
				if strings.TrimSpace(withIndent) != "" {
					// WITH was inline (e.g. "IF x THEN WITH r DO ...") -> replace from WITH to stmt start.
					cur = cur[:tokens[withIdx].Start] + prefixed + cur[fragEnd:]
				} else {
					// WITH was at start of line -> replace "WITH ... DO [newline+indent]" with withIndent + prefixedStmt
					cur = cur[:withLineStart] + withIndent + prefixed + cur[fragEnd:]
				}
			}

			diags = append(diags, Diagnostic{
				Rule:     config.RuleNoWith,
				Severity: SeverityInfo,
				Line:     lineNum,
				Message:  fmt.Sprintf("replaced WITH %s DO with explicit %s. qualifiers", recExpr, recExpr),
				Fixed:    true,
			})
			progress = true
			break // restart: offsets changed
		}

		if !progress {
			diags = append(diags, skipDiags...)
			break
		}
	}

	return cur, diags
}

// resolveWithSchema resolves the WITH target to an object schema from the
// WorkspaceIndex (or from the object being processed itself, for implicit Rec
// inside its own Table). Returns nil when the schema is unavailable (task 1.3).
func resolveWithSchema(recExpr string, scope *localScope, osym *objectSymbols, obj *objectfile.ObjectFile, ws *objectfile.WorkspaceIndex) *objectfile.TableSchema {
	recKey := strings.ToLower(strings.Trim(recExpr, `"`))
	if recKey == "rec" || recKey == "xrec" {
		if obj.SourceTable > 0 {
			if ws != nil {
				if ts := ws.LookupTable(obj.SourceTable); ts != nil {
					return ts
				}
			}
			// Processing the table object itself: its fields/procedures are known directly.
			if strings.EqualFold(obj.ObjectType, "Table") && obj.ObjectID == obj.SourceTable {
				return objectfile.SchemaFromObject(obj)
			}
		}
		return nil
	}
	if vd, ok := scope.vars[recKey]; ok && vd.SubtypeID > 0 && strings.EqualFold(vd.BaseType, "Record") {
		if ws != nil {
			return ws.LookupObject("Table", vd.SubtypeID)
		}
		return nil
	}
	if vd, ok := scope.vars[recKey]; ok && vd.SubtypeID > 0 {
		switch strings.ToLower(vd.BaseType) {
		case "codeunit", "page", "report", "query", "xmlport", "port":
			if ws != nil {
				return ws.LookupObject(vd.BaseType, vd.SubtypeID)
			}
			return nil
		}
	}
	if tblID, ok := osym.dataItems[recKey]; ok && tblID > 0 {
		if ws != nil {
			return ws.LookupTable(tblID)
		}
	}
	return nil
}

// containsWithKeyword reports whether an unqualified WITH keyword occurs between
// tokens (from, to] — i.e. an inner WITH block is still present in the block body.
func containsWithKeyword(tokens []cal.Token, from, to int) bool {
	for i := from + 1; i <= to && i < len(tokens); i++ {
		if tokens[i].IsWord("WITH") && !isMemberAccess(tokens, i) {
			return true
		}
	}
	return false
}

// findAllEligibleWiths returns eligible WITH keyword token indices, innermost (last) first.
func findAllEligibleWiths(tokens []cal.Token, changedLines map[int]bool) []int {
	var res []int
	for i := len(tokens) - 1; i >= 0; i-- {
		if !tokens[i].IsWord("WITH") || isMemberAccess(tokens, i) {
			continue
		}
		if changedLines == nil {
			res = append(res, i)
			continue
		}
		// In ModePartial: the WITH header must be on a new/changed line.
		if !changedLines[tokens[i].Line] {
			continue
		}
		doIdx := findDoAfterWith(tokens, i)
		if doIdx == -1 {
			continue
		}
		stmtStart := cal.NextCode(tokens, doIdx+1)
		if stmtStart == -1 {
			continue
		}
		if tokens[stmtStart].IsWord("BEGIN") {
			endIdx := findMatchingEndToken(tokens, stmtStart)
			if endIdx == -1 || !changedLines[tokens[endIdx].Line] {
				continue
			}
			// Ensure every non-empty line in the WITH..END block is new (not wrapping untouched old code).
			allNew := true
			for k := i; k <= endIdx; k++ {
				if !tokens[k].IsTrivia() && !changedLines[tokens[k].Line] {
					allNew = false
					break
				}
			}
			if !allNew {
				continue
			}
		} else {
			stmtEnd := findSingleStatementEnd(tokens, stmtStart)
			if stmtEnd == -1 || !changedLines[tokens[stmtEnd].Line] {
				continue
			}
		}
		res = append(res, i)
	}
	return res
}

func findDoAfterWith(tokens []cal.Token, withIdx int) int {
	for i := withIdx + 1; i < len(tokens); i++ {
		if tokens[i].IsWord("DO") && !isMemberAccess(tokens, i) {
			return i
		}
	}
	return -1
}

func findMatchingEndToken(tokens []cal.Token, beginIdx int) int {
	depth := 0
	for i := beginIdx; i < len(tokens); i++ {
		if tokens[i].IsTrivia() {
			continue
		}
		if (tokens[i].IsWord("BEGIN") || tokens[i].IsWord("CASE")) && !isMemberAccess(tokens, i) {
			depth++
		} else if tokens[i].IsWord("END") && !isMemberAccess(tokens, i) {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findSingleStatementEnd(tokens []cal.Token, startIdx int) int {
	pDepth := 0
	bDepth := 0
	blockDepth := 0
	lastCode := startIdx
	for i := startIdx; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.IsTrivia() {
			continue
		}
		lastCode = i
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
			if pDepth == 0 && bDepth == 0 && blockDepth == 0 {
				return i
			}
		}
		if !isMemberAccess(tokens, i) {
			if tok.IsWord("BEGIN") || tok.IsWord("CASE") || tok.IsWord("REPEAT") {
				blockDepth++
			} else if tok.IsWord("END") || tok.IsWord("UNTIL") {
				if blockDepth == 0 {
					// Terminated by enclosing END / UNTIL / ELSE before semicolon.
					return cal.PrevCode(tokens, i-1)
				}
				blockDepth--
			} else if tok.IsWord("ELSE") && blockDepth == 0 {
				return cal.PrevCode(tokens, i-1)
			}
		}
	}
	return lastCode
}

var recordFirstArgFieldMethods = map[string]bool{
	"SETRANGE":     true,
	"SETFILTER":    true,
	"VALIDATE":     true,
	"TESTFIELD":    true,
	"GETFILTER":    true,
	"GETRANGEMIN":  true,
	"GETRANGEMAX":  true,
	"FIELDNAME":    true,
	"FIELDCAPTION": true,
	"FIELDERROR":   true,
	"FIELDACTIVE":  true,
	"SETASCENDING": true,
	"COPYFILTER":   true,
	"MODIFYALL":    true,
}

var recordAllArgsFieldMethods = map[string]bool{
	"SETCURRENTKEY":     true,
	"CALCFIELDS":        true,
	"CALCSUMS":          true,
	"SETAUTOCALCFIELDS": true,
}

type callArgFrame struct {
	fieldArgMode int // 0 = normal, 1 = first arg is field ref, 2 = all args are field refs
	argIndex     int
}

// prefixWithIdentifiers prefixes field/method references that belong to the WITH
// target with recExpr. Strict mode (task 1.3): it NEVER guesses — every identifier
// that is not a known variable/procedure/builtin/record method must be present in
// the resolved object schema (fields or public procedures). Unresolvable names are
// returned as "unknowns" so the caller can refuse the WITH elimination.
func prefixWithIdentifiers(fragment string, recExpr string, scope *localScope, osym *objectSymbols, tableSchema *objectfile.TableSchema) (string, []string) {
	tokens := cal.Lex(fragment)
	var edits []textEdit
	var unknowns []string
	unknownSeen := make(map[string]bool)
	var callStack []callArgFrame

	for i, tok := range tokens {
		switch tok.Kind {
		case cal.TokLParen:
			mode := 0
			prev := cal.PrevCode(tokens, i-1)
			if prev != -1 && (tokens[prev].Kind == cal.TokIdent || tokens[prev].Kind == cal.TokQuotedIdent) {
				upper := strings.ToUpper(tokens[prev].IdentName())
				if recordAllArgsFieldMethods[upper] {
					mode = 2
				} else if recordFirstArgFieldMethods[upper] {
					mode = 1
				}
			}
			callStack = append(callStack, callArgFrame{fieldArgMode: mode, argIndex: 0})
			continue
		case cal.TokRParen:
			if len(callStack) > 0 {
				callStack = callStack[:len(callStack)-1]
			}
			continue
		case cal.TokComma:
			if len(callStack) > 0 {
				callStack[len(callStack)-1].argIndex++
			}
			continue
		}

		if tok.Kind != cal.TokIdent && tok.Kind != cal.TokQuotedIdent {
			continue
		}
		if isMemberAccess(tokens, i) {
			continue
		}
		if tok.Kind == cal.TokIdent && cal.IsStatementKeyword(tok.Text) {
			continue
		}

		// If we are inside a Record method's field-name argument slot (e.g. SETRANGE("Item No.", ...)),
		// do NOT prefix the field name with recExpr.
		if len(callStack) > 0 {
			top := callStack[len(callStack)-1]
			if top.fieldArgMode == 2 || (top.fieldArgMode == 1 && top.argIndex == 0) {
				continue
			}
		}

		name := tok.IdentName()
		lower := strings.ToLower(name)

		// Do not prefix the WITH target itself (e.g. CLEAR(RecVar) or RecVar.Field).
		if strings.EqualFold(name, strings.Trim(recExpr, `"`)) {
			continue
		}

		// Table fields from the resolved schema are ALWAYS prefixed — including
		// Option fields before '::' ("Document Type"::Order -> X."Document Type"::Order).
		// System enum qualifiers (ACTION::LookupOK) are not schema fields and are
		// protected below (task 1.3 + review fix).
		isSchemaField := tableSchema != nil && tableSchema.HasField(name)

		// Never prefix an enum qualifier that is not a target field.
		if next := cal.NextCode(tokens, i+1); next != -1 && tokens[next].Kind == cal.TokScope && !isSchemaField {
			continue
		}

		shouldPrefix := false

		// Record methods only make sense for a Record (Table) target; other objects
		// expose their own zero-arg methods instead.
		isTableTarget := tableSchema == nil || strings.EqualFold(tableSchema.ObjectType, "Table")
		_, shadowedByVar := scope.vars[lower]

		if isSchemaField {
			shouldPrefix = true
		} else if isTableTarget && cal.IsRecordMethod(name) && !shadowedByVar {
			// Built-in Record method (SETRANGE, FINDSET, MODIFY, VALIDATE, TESTFIELD, INIT, INSERT, etc.)
			// not shadowed by a local/global variable of the same name.
			shouldPrefix = true
		} else if !isTableTarget && cal.IsObjectZeroArgMethod(name) && !shadowedByVar {
			// Zero-arg method of Page/Report/Codeunit/XMLport/File/Dialog objects.
			shouldPrefix = true
		} else {
			// Known variable, own-object procedure, own-table field, DataItem or global builtin: never prefix.
			_, isVar := scope.vars[lower]
			_, isProc := osym.procs[lower]
			_, isDataItem := osym.dataItems[lower]
			_, isOwnField := osym.ownFields[lower]
			isBuiltin := cal.IsGlobalBuiltin(name)

			switch {
			case isVar || isProc || isDataItem || isOwnField || isBuiltin:
				// leave unqualified (own fields stay accessible via the implicit Rec)
			case tableSchema != nil && tableSchema.HasProc(name):
				// Public procedure of the WITH target object (task 1.3): prefix it.
				shouldPrefix = true
			default:
				// Unresolvable name -> strict mode: refuse elimination for this block (deduplicated).
				if !unknownSeen[lower] {
					unknownSeen[lower] = true
					unknowns = append(unknowns, name)
				}
			}
		}

		if shouldPrefix {
			edits = append(edits, textEdit{
				start:   tok.Start,
				end:     tok.End,
				newText: recExpr + "." + tok.Text,
			})
		}
	}

	return applyTextEdits(fragment, edits), unknowns
}

func unwrapWithBeginBody(inner string, eol string, indentSize int) string {
	// Trim the leading newline right after BEGIN.
	s := inner
	if strings.HasPrefix(s, "\r\n") {
		s = s[2:]
	} else if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}

	// Trim trailing whitespace on the line right before END.
	lastNL := strings.LastIndexByte(s, '\n')
	if lastNL >= 0 && strings.TrimSpace(s[lastNL+1:]) == "" {
		s = s[:lastNL+1]
	}

	// Dedent each non-empty line by indentSize spaces (up to indentSize).
	if indentSize <= 0 {
		indentSize = 2
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		spaces := 0
		for spaces < indentSize && spaces < len(line) && line[spaces] == ' ' {
			spaces++
		}
		lines[i] = line[spaces:]
	}
	res := strings.Join(lines, "\n")
	if res != "" && !strings.HasSuffix(res, "\n") {
		res += eol
	}
	return res
}

func lineStartPos(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	idx := strings.LastIndexByte(s[:pos], '\n')
	if idx == -1 {
		return 0
	}
	return idx + 1
}

func lineEndPos(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	idx := strings.IndexByte(s[pos:], '\n')
	if idx == -1 {
		return len(s)
	}
	return pos + idx + 1
}
