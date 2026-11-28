package rules

import (
	"fmt"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

const navMaxIdentLen = 30

// conflictsGlobalBuiltin reports whether a declared identifier equals a NAV
// system function name (GlobalBuiltins), e.g. variable "Time" vs TIME() (task 1.5).
func conflictsGlobalBuiltin(name string) bool {
	if name == "" || cal.IsStatementKeyword(name) {
		return false
	}
	return cal.IsGlobalBuiltin(name)
}

// runSuffixRules implements CAL010 (local variable suffix 'L') and CAL011 (parameter suffix 'P'),
// plus task 1.5 renames for names colliding with system functions
// (local/Return Value -> +L, parameter -> +P, global -> +G, always with a warning).
func runSuffixRules(obj *objectfile.ObjectFile, cfg *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var cs *cal.CodeSection
	if obj.Code != nil {
		cs = cal.ParseCodeSection(obj.Code.Raw)
	}
	osym := buildObjectSymbols(obj, cs)

	localSuffix := cfg.LocalVarSuffix()
	paramSuffix := cfg.ParamSuffix()
	enableLocal := cfg.IsRuleEnabled(config.RuleLocalVarSuffix)
	enableParam := cfg.IsRuleEnabled(config.RuleParamSuffix)

	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	// Task 1.5: globals colliding with system function names get the 'G' suffix.
	// Only safe in full-file mode (every unit gets its references updated) and when
	// no unit shadows the old or the new name.
	globalRenames := make(map[string]string) // lower(old global name) -> new name
	var globalDeclEdits []textEdit
	if enableLocal && cs != nil && cs.GlobalVars != nil && (fscope == nil || !fscope.Enabled) {
		shadowed := make(map[string]bool)
		for _, proc := range cs.Procedures {
			for _, p := range proc.Params {
				shadowed[strings.ToLower(p.Name)] = true
			}
			if proc.ReturnVar != nil {
				shadowed[strings.ToLower(proc.ReturnVar.Name)] = true
			}
			if proc.LocalVars != nil {
				for _, lv := range proc.LocalVars.Decls {
					shadowed[strings.ToLower(lv.Name)] = true
				}
			}
		}
		for _, tr := range obj.Triggers {
			td := cal.ParseTrigger(tr.Raw)
			if td.LocalVars != nil {
				for _, lv := range td.LocalVars.Decls {
					shadowed[strings.ToLower(lv.Name)] = true
				}
			}
		}
		for _, gv := range cs.GlobalVars.Decls {
			if !conflictsGlobalBuiltin(gv.Name) {
				continue
			}
			newName := gv.Name + "G"
			lowerOld := strings.ToLower(gv.Name)
			lowerNew := strings.ToLower(newName)
			lineNum := obj.LineOfOffset(obj.Code.StartOffset + gv.NameStart)
			msgSkip := func(why string) {
				diags = append(diags, Diagnostic{
					Rule:     config.RuleLocalVarSuffix,
					Severity: SeverityWarning,
					Line:     lineNum,
					Message:  fmt.Sprintf("global variable '%s' collides with system function '%s', but %s; not renamed automatically", gv.Name, strings.ToUpper(gv.Name), why),
					Fixed:    false,
				})
			}
			if _, isProc := osym.procs[lowerOld]; isProc || shadowed[lowerOld] {
				msgSkip("the name is shadowed by a procedure or a local/parameter declaration")
				continue
			}
			if len([]rune(newName)) > navMaxIdentLen {
				msgSkip(fmt.Sprintf("'%s' exceeds NAV 30-char limit", newName))
				continue
			}
			collides := shadowed[lowerNew] || osym.dataItems[lowerNew] > 0
			if _, isProc := osym.procs[lowerNew]; isProc {
				collides = true
			}
			for _, other := range cs.GlobalVars.Decls {
				if other != gv && strings.EqualFold(other.Name, newName) {
					collides = true
				}
			}
			if collides {
				msgSkip(fmt.Sprintf("'%s' collides with an existing name", newName))
				continue
			}
			globalRenames[lowerOld] = newName
			globalDeclEdits = append(globalDeclEdits, textEdit{
				start:   gv.NameStart,
				end:     gv.NameEnd,
				newText: cal.FormatIdent(newName, gv.IsQuoted),
			})
			diags = append(diags, Diagnostic{
				Rule:     config.RuleLocalVarSuffix,
				Severity: SeverityWarning,
				Line:     lineNum,
				Message:  fmt.Sprintf("renamed global variable '%s' to '%s': name collided with system function '%s'", gv.Name, newName, strings.ToUpper(gv.Name)),
				Fixed:    true,
			})
		}
	}

	// 1. Embedded triggers (only local vars, CAL010).
	if enableLocal {
		for _, tr := range obj.Triggers {
			us := fscope.TriggerScope(tr)
			if us.Mode == diffscope.ModeSkip {
				continue
			}
			td := cal.ParseTrigger(tr.Raw)
			if td.LocalVars == nil || td.BodyStart < 0 {
				continue
			}
			renames := make(map[string]string) // lower old -> new canonical name
			var edits []textEdit
			occupied := buildOccupiedNames(nil, nil, td.LocalVars, osym)

			for _, lv := range td.LocalVars.Decls {
				if lv.IsTextConst {
					continue
				}
				if !us.IsNewLocalVar(lv.Name) {
					continue
				}
				if strings.HasSuffix(lv.Name, localSuffix) {
					continue
				}
				newName := lv.Name + localSuffix
				lineNum := obj.LineOfOffset(tr.StartOffset + lv.NameStart)
				if len([]rune(newName)) > navMaxIdentLen {
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("local variable '%s' missing '%s' suffix, but '%s' exceeds NAV 30-char limit", lv.Name, localSuffix, newName),
						Fixed:    false,
					})
					continue
				}
				if occupied[strings.ToLower(newName)] {
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("local variable '%s' cannot be renamed to '%s' due to name collision", lv.Name, newName),
						Fixed:    false,
					})
					continue
				}
				occupied[strings.ToLower(newName)] = true
				renames[strings.ToLower(lv.Name)] = newName
				edits = append(edits, textEdit{
					start:   lv.NameStart,
					end:     lv.NameEnd,
					newText: cal.FormatIdent(newName, lv.IsQuoted),
				})
				diags = append(diags, Diagnostic{
					Rule:     config.RuleLocalVarSuffix,
					Severity: SeverityInfo,
					Line:     lineNum,
					Message:  fmt.Sprintf("renamed local variable '%s' to '%s'", lv.Name, newName),
					Fixed:    true,
				})
				if conflictsGlobalBuiltin(lv.Name) {
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("local variable '%s' collided with system function '%s'; renamed to '%s'", lv.Name, strings.ToUpper(lv.Name), newName),
						Fixed:    true,
					})
				}
			}

			for k, v := range globalRenames {
				if _, ok := renames[k]; !ok {
					renames[k] = v
				}
			}

			if len(renames) > 0 {
				body := tr.Raw[td.BodyStart:td.BodyEnd]
				newBody := applyIdentifierRenames(body, renames)
				if newBody != body {
					edits = append(edits, textEdit{
						start:   td.BodyStart,
						end:     td.BodyEnd,
						newText: newBody,
					})
				}
				spanEdits = append(spanEdits, objectfile.SpanEdit{
					Start:   tr.StartOffset,
					End:     tr.EndOffset,
					NewText: applyTextEdits(tr.Raw, edits),
				})
			}
		}
	}

	// 2. CODE section procedures (CAL010 & CAL011).
	if obj.Code != nil && cs != nil {
		var codeEdits []textEdit
		for _, proc := range cs.Procedures {
			us := fscope.ProcScope(proc)
			if us.Mode == diffscope.ModeSkip {
				continue
			}
			if proc.IsEvent {
				continue
			}
			renames := make(map[string]string)
			occupied := buildOccupiedNames(proc.Params, proc.ReturnVar, proc.LocalVars, osym)

			// CAL011: Parameters (skipped for ModePartial large old functions).
			if enableParam && us.Mode == diffscope.ModeFull && len(proc.Params) > 0 {
				isEventSub := proc.IsEventSubscriber()
				// NAV 2017 [Integration]/[Business] publishers are fully excluded from CAL011 (task 1.4).
				isEventPub := proc.IsEventPublisher()
				isSysTrig := cal.IsSystemTriggerName(proc.Name)

				if isEventSub {
					// User rule 4: warn on EventSubscriber, NEVER auto-fix!
					for _, param := range proc.Params {
						if !strings.HasSuffix(param.Name, paramSuffix) {
							diags = append(diags, Diagnostic{
								Rule:     config.RuleParamSuffix,
								Severity: SeverityWarning,
								Line:     obj.LineOfOffset(obj.Code.StartOffset + param.NameStart),
								Message:  fmt.Sprintf("EventSubscriber '%s' parameter '%s' does not end with '%s' (not auto-fixed to preserve event binding)", proc.Name, param.Name, paramSuffix),
								Fixed:    false,
							})
						}
					}
				} else if !isEventPub && !isSysTrig {
					for _, param := range proc.Params {
						if strings.HasSuffix(param.Name, paramSuffix) {
							continue
						}
						newName := param.Name + paramSuffix
						lineNum := obj.LineOfOffset(obj.Code.StartOffset + param.NameStart)
						if len([]rune(newName)) > navMaxIdentLen {
							diags = append(diags, Diagnostic{
								Rule:     config.RuleParamSuffix,
								Severity: SeverityWarning,
								Line:     lineNum,
								Message:  fmt.Sprintf("parameter '%s' missing '%s' suffix, but '%s' exceeds NAV 30-char limit", param.Name, paramSuffix, newName),
								Fixed:    false,
							})
							continue
						}
						if occupied[strings.ToLower(newName)] {
							diags = append(diags, Diagnostic{
								Rule:     config.RuleParamSuffix,
								Severity: SeverityWarning,
								Line:     lineNum,
								Message:  fmt.Sprintf("parameter '%s' cannot be renamed to '%s' due to name collision", param.Name, newName),
								Fixed:    false,
							})
							continue
						}
						occupied[strings.ToLower(newName)] = true
						renames[strings.ToLower(param.Name)] = newName
						codeEdits = append(codeEdits, textEdit{
							start:   param.NameStart,
							end:     param.NameEnd,
							newText: cal.FormatIdent(newName, param.IsQuoted),
						})
						diags = append(diags, Diagnostic{
							Rule:     config.RuleParamSuffix,
							Severity: SeverityInfo,
							Line:     lineNum,
							Message:  fmt.Sprintf("renamed parameter '%s' to '%s'", param.Name, newName),
							Fixed:    true,
						})
						if conflictsGlobalBuiltin(param.Name) {
							diags = append(diags, Diagnostic{
								Rule:     config.RuleParamSuffix,
								Severity: SeverityWarning,
								Line:     lineNum,
								Message:  fmt.Sprintf("parameter '%s' collided with system function '%s'; renamed to '%s'", param.Name, strings.ToUpper(param.Name), newName),
								Fixed:    true,
							})
						}
					}
				}
			}

			// CAL010: Local variables (ReturnVar is intentionally excluded per user rule 1!).
			if enableLocal && proc.LocalVars != nil {
				for _, lv := range proc.LocalVars.Decls {
					if lv.IsTextConst {
						continue
					}
					if !us.IsNewLocalVar(lv.Name) {
						continue
					}
					if strings.HasSuffix(lv.Name, localSuffix) {
						continue
					}
					newName := lv.Name + localSuffix
					lineNum := obj.LineOfOffset(obj.Code.StartOffset + lv.NameStart)
					if len([]rune(newName)) > navMaxIdentLen {
						diags = append(diags, Diagnostic{
							Rule:     config.RuleLocalVarSuffix,
							Severity: SeverityWarning,
							Line:     lineNum,
							Message:  fmt.Sprintf("local variable '%s' missing '%s' suffix, but '%s' exceeds NAV 30-char limit", lv.Name, localSuffix, newName),
							Fixed:    false,
						})
						continue
					}
					if occupied[strings.ToLower(newName)] {
						diags = append(diags, Diagnostic{
							Rule:     config.RuleLocalVarSuffix,
							Severity: SeverityWarning,
							Line:     lineNum,
							Message:  fmt.Sprintf("local variable '%s' cannot be renamed to '%s' due to name collision", lv.Name, newName),
							Fixed:    false,
						})
						continue
					}
					occupied[strings.ToLower(newName)] = true
					renames[strings.ToLower(lv.Name)] = newName
					codeEdits = append(codeEdits, textEdit{
						start:   lv.NameStart,
						end:   lv.NameEnd,
						newText: cal.FormatIdent(newName, lv.IsQuoted),
					})
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityInfo,
						Line:     lineNum,
						Message:  fmt.Sprintf("renamed local variable '%s' to '%s'", lv.Name, newName),
						Fixed:    true,
					})
					if conflictsGlobalBuiltin(lv.Name) {
						diags = append(diags, Diagnostic{
							Rule:     config.RuleLocalVarSuffix,
							Severity: SeverityWarning,
							Line:     lineNum,
							Message:  fmt.Sprintf("local variable '%s' collided with system function '%s'; renamed to '%s'", lv.Name, strings.ToUpper(lv.Name), newName),
							Fixed:    true,
						})
					}
				}
			}

			// Task 1.5: Return Value colliding with a system function gets the local
			// suffix (Return Values normally carry no suffix).
			if enableLocal && proc.ReturnVar != nil && proc.ReturnVar.NameEnd > proc.ReturnVar.NameStart &&
				!proc.IsEventSubscriber() && !proc.IsEventPublisher() &&
				conflictsGlobalBuiltin(proc.ReturnVar.Name) {
				oldName := proc.ReturnVar.Name
				newName := oldName + localSuffix
				lineNum := obj.LineOfOffset(obj.Code.StartOffset + proc.ReturnVar.NameStart)
				switch {
				case len([]rune(newName)) > navMaxIdentLen:
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("Return Value '%s' collides with system function '%s', but '%s' exceeds NAV 30-char limit; not renamed", oldName, strings.ToUpper(oldName), newName),
						Fixed:    false,
					})
				case occupied[strings.ToLower(newName)]:
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("Return Value '%s' collides with system function '%s', but '%s' is already in use; not renamed", oldName, strings.ToUpper(oldName), newName),
						Fixed:    false,
					})
				default:
					occupied[strings.ToLower(newName)] = true
					renames[strings.ToLower(oldName)] = newName
					codeEdits = append(codeEdits, textEdit{
						start:   proc.ReturnVar.NameStart,
						end:     proc.ReturnVar.NameEnd,
						newText: cal.FormatIdent(newName, proc.ReturnVar.IsQuoted),
					})
					diags = append(diags, Diagnostic{
						Rule:     config.RuleLocalVarSuffix,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("renamed Return Value '%s' to '%s': name collided with system function '%s'", oldName, newName, strings.ToUpper(oldName)),
						Fixed:    true,
					})
				}
			}

			for k, v := range globalRenames {
				if _, ok := renames[k]; !ok {
					renames[k] = v
				}
			}

			if len(renames) > 0 && proc.BodyStart >= 0 && proc.BodyEnd > proc.BodyStart {
				body := obj.Code.Raw[proc.BodyStart:proc.BodyEnd]
				newBody := applyIdentifierRenames(body, renames)
				if newBody != body {
					codeEdits = append(codeEdits, textEdit{
						start:   proc.BodyStart,
						end:     proc.BodyEnd,
						newText: newBody,
					})
				}
			}
		}

		codeEdits = append(codeEdits, globalDeclEdits...)
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

// runIdentifierCasingRule implements CAL012: align identifier casing in code to declaration casing.
func runIdentifierCasingRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
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
		canon := buildCanonicalMap(nil, nil, td.LocalVars, osym)
		body := tr.Raw[td.BodyStart:td.BodyEnd]
		var changedLines map[int]bool
		if us.Mode == diffscope.ModePartial {
			changedLines = us.ChangedBodyLines(body)
		}
		newBody, d := fixIdentifierCasing(body, canon, changedLines, obj, tr.StartOffset+td.BodyStart)
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
			canon := buildCanonicalMap(proc.Params, proc.ReturnVar, proc.LocalVars, osym)
			body := obj.Code.Raw[proc.BodyStart:proc.BodyEnd]
			var changedLines map[int]bool
			if us.Mode == diffscope.ModePartial {
				changedLines = us.ChangedBodyLines(body)
			}
			newBody, d := fixIdentifierCasing(body, canon, changedLines, obj, obj.Code.StartOffset+proc.BodyStart)
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

func buildOccupiedNames(params []*cal.VarDecl, retVar *cal.VarDecl, localVars *cal.VarSection, osym *objectSymbols) map[string]bool {
	occ := make(map[string]bool)
	for k := range osym.globals {
		occ[k] = true
	}
	for k := range osym.procs {
		occ[k] = true
	}
	for k := range osym.dataItems {
		occ[k] = true
	}
	for _, p := range params {
		occ[strings.ToLower(p.Name)] = true
	}
	if retVar != nil {
		occ[strings.ToLower(retVar.Name)] = true
	}
	if localVars != nil {
		for _, v := range localVars.Decls {
			occ[strings.ToLower(v.Name)] = true
		}
	}
	return occ
}

func buildCanonicalMap(params []*cal.VarDecl, retVar *cal.VarDecl, localVars *cal.VarSection, osym *objectSymbols) map[string]string {
	m := make(map[string]string)
	// Lower priority first, higher priority overrides.
	for k, p := range osym.procs {
		m[k] = p.Name
	}
	for k, g := range osym.globals {
		m[k] = g.Name
	}
	for _, p := range params {
		m[strings.ToLower(p.Name)] = p.Name
	}
	if retVar != nil {
		m[strings.ToLower(retVar.Name)] = retVar.Name
	}
	if localVars != nil {
		for _, v := range localVars.Decls {
			m[strings.ToLower(v.Name)] = v.Name
		}
	}
	return m
}

func applyIdentifierRenames(body string, renames map[string]string) string {
	tokens := cal.Lex(body)
	var edits []textEdit
	for i, tok := range tokens {
		if tok.Kind != cal.TokIdent && tok.Kind != cal.TokQuotedIdent {
			continue
		}
		if isMemberAccess(tokens, i) {
			continue
		}
		if tok.Kind == cal.TokIdent && cal.IsStatementKeyword(tok.Text) {
			continue
		}
		if newName, ok := renames[strings.ToLower(tok.IdentName())]; ok {
			// Task 1.5: an exact-UPPERCASE reference to a system builtin (e.g. WORKDATE)
			// is a function call, not a reference to the colliding variable; never rename it.
			if tok.Kind == cal.TokIdent && tok.Text == strings.ToUpper(tok.Text) && cal.IsGlobalBuiltin(tok.Text) {
				continue
			}
			edits = append(edits, textEdit{
				start:   tok.Start,
				end:     tok.End,
				newText: cal.FormatIdent(newName, tok.Kind == cal.TokQuotedIdent),
			})
		}
	}
	return applyTextEdits(body, edits)
}

func fixIdentifierCasing(body string, canon map[string]string, changedLines map[int]bool, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
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
		if isMemberAccess(tokens, i) {
			continue
		}
		if tok.Kind == cal.TokIdent && cal.IsStatementKeyword(tok.Text) {
			continue
		}
		curName := tok.IdentName()
		if expected, ok := canon[strings.ToLower(curName)]; ok && curName != expected {
			edits = append(edits, textEdit{
				start:   tok.Start,
				end:     tok.End,
				newText: cal.FormatIdent(expected, tok.Kind == cal.TokQuotedIdent),
			})
			diags = append(diags, Diagnostic{
				Rule:     config.RuleIdentifierCasing,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset + tok.Start),
				Message:  fmt.Sprintf("adjusted identifier casing '%s' -> '%s'", curName, expected),
				Fixed:    true,
			})
		}
	}

	return applyTextEdits(body, edits), diags
}
