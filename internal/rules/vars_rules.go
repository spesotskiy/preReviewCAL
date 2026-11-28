package rules

import (
	"fmt"
	"strconv"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

// runUnusedLocalVarRule implements CAL001: remove unused local variables (and empty local VAR sections).
func runUnusedLocalVarRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	var spanEdits []objectfile.SpanEdit
	var diags []Diagnostic

	// Embedded triggers.
	for _, tr := range obj.Triggers {
		us := fscope.TriggerScope(tr)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		td := cal.ParseTrigger(tr.Raw)
		if td.LocalVars == nil || len(td.LocalVars.Decls) == 0 || td.BodyStart < 0 {
			continue
		}
		refs := collectUnqualifiedRefs(tr.Raw[td.BodyStart:td.BodyEnd])
		edits, localDiags := removeUnusedVars(tr.Raw, td.LocalVars, refs, nil, true, tr.BaseIndent, obj.LineEnding, obj, tr.StartOffset, config.RuleUnusedLocalVar)
		diags = append(diags, localDiags...)
		if len(edits) > 0 {
			newRaw := applyTextEdits(tr.Raw, edits)
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset,
				End:     tr.EndOffset,
				NewText: newRaw,
			})
		}
	}

	// CODE section procedures.
	if obj.Code != nil {
		cs := cal.ParseCodeSection(obj.Code.Raw)
		var codeEdits []textEdit
		for _, proc := range cs.Procedures {
			us := fscope.ProcScope(proc)
			if us.Mode == diffscope.ModeSkip {
				continue
			}
			if proc.LocalVars == nil || len(proc.LocalVars.Decls) == 0 || proc.BodyStart < 0 {
				continue
			}
			refs := collectUnqualifiedRefs(obj.Code.Raw[proc.BodyStart:proc.BodyEnd])
			edits, localDiags := removeUnusedVars(obj.Code.Raw, proc.LocalVars, refs, nil, false, 4, obj.LineEnding, obj, obj.Code.StartOffset, config.RuleUnusedLocalVar)
			diags = append(diags, localDiags...)
			codeEdits = append(codeEdits, edits...)
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

// runUnusedGlobalVarRule implements CAL002: remove unused global variables.
func runUnusedGlobalVarRule(obj *objectfile.ObjectFile, _ *config.Config) ([]objectfile.SpanEdit, []Diagnostic) {
	if obj.Code == nil {
		return nil, nil
	}
	cs := cal.ParseCodeSection(obj.Code.Raw)
	if cs.GlobalVars == nil || len(cs.GlobalVars.Decls) == 0 {
		return nil, nil
	}

	// Gather references from ALL embedded triggers, ALL procedures, and non-C/AL ExternalRefs.
	used := make(map[string]bool)
	for k := range obj.ExternalRefs {
		used[k] = true
	}
	for _, tr := range obj.Triggers {
		td := cal.ParseTrigger(tr.Raw)
		if td.BodyStart >= 0 && td.BodyEnd > td.BodyStart {
			for k := range collectUnqualifiedRefs(tr.Raw[td.BodyStart:td.BodyEnd]) {
				used[k] = true
			}
		}
	}
	for _, proc := range cs.Procedures {
		if proc.IsEvent {
			// EVENT MyDotNet@1000::OnSomething -> mark MyDotNet as used!
			for k := range collectUnqualifiedRefs(proc.RawName) {
				used[k] = true
			}
		}
		if proc.BodyStart >= 0 && proc.BodyEnd > proc.BodyStart {
			// Exclude identifiers shadowed by local vars / params of this procedure?
			// Conservative approach: if referenced anywhere unqualified and NOT shadowed by local/param, mark global used.
			localNames := make(map[string]bool)
			for _, p := range proc.Params {
				localNames[strings.ToLower(p.Name)] = true
			}
			if proc.ReturnVar != nil {
				localNames[strings.ToLower(proc.ReturnVar.Name)] = true
			}
			if proc.LocalVars != nil {
				for _, lv := range proc.LocalVars.Decls {
					localNames[strings.ToLower(lv.Name)] = true
				}
			}
			for k := range collectUnqualifiedRefs(obj.Code.Raw[proc.BodyStart:proc.BodyEnd]) {
				if !localNames[k] {
					used[k] = true
				}
			}
		}
	}

	// Protect WITHEVENTS and INDATASET global variables from deletion.
	for _, gv := range cs.GlobalVars.Decls {
		if gv.IsWithEvents || gv.IsInDataSet {
			used[strings.ToLower(gv.Name)] = true
		}
	}

	edits, diags := removeUnusedVars(obj.Code.Raw, cs.GlobalVars, used, nil, false, 4, obj.LineEnding, obj, obj.Code.StartOffset, config.RuleUnusedGlobalVar)
	if len(edits) == 0 {
		return nil, diags
	}

	newCode := applyTextEdits(obj.Code.Raw, edits)
	// Clean up leading blank line if global VAR was completely deleted.
	if len(edits) == 1 && edits[0].start == cs.GlobalVars.StartOffset {
		newCode = strings.TrimLeft(newCode, "\r\n")
	}

	return []objectfile.SpanEdit{{
		Start:   obj.Code.StartOffset,
		End:     obj.Code.EndOffset,
		NewText: newCode,
	}}, diags
}

// runUnusedParamRule implements CAL003: report warnings for unused procedure parameters (never auto-deletes).
func runUnusedParamRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	if obj.Code == nil {
		return nil, nil
	}
	cs := cal.ParseCodeSection(obj.Code.Raw)
	var diags []Diagnostic

	for _, proc := range cs.Procedures {
		us := fscope.ProcScope(proc)
		if us.Mode != diffscope.ModeFull {
			continue
		}
		if proc.IsEvent || proc.BodyStart < 0 || proc.BodyEnd <= proc.BodyStart {
			continue
		}
		// Skip event publishers (NAV 2017 [Integration]/[Business] and AL [IntegrationEvent]/[BusinessEvent],
		// their bodies are intentionally empty) and system triggers (task 1.4).
		if proc.IsEventPublisher() || cal.IsSystemTriggerName(proc.Name) {
			continue
		}
		refs := collectUnqualifiedRefs(obj.Code.Raw[proc.BodyStart:proc.BodyEnd])
		for _, param := range proc.Params {
			if !refs[strings.ToLower(param.Name)] {
				diags = append(diags, Diagnostic{
					Rule:     config.RuleUnusedParam,
					Severity: SeverityWarning,
					Line:     obj.LineOfOffset(obj.Code.StartOffset + param.NameStart),
					Message:  fmt.Sprintf("unused parameter '%s' in procedure '%s'", param.Name, proc.Name),
					Fixed:    false,
				})
			}
		}
	}
	return nil, diags
}

// runLocalTextConstRule implements CAL004: TextConst must be global (promote local TextConst to global VAR).
func runLocalTextConstRule(obj *objectfile.ObjectFile, _ *config.Config, fscope *diffscope.FileScope) ([]objectfile.SpanEdit, []Diagnostic) {
	if obj.Code == nil {
		return nil, nil
	}
	cs := cal.ParseCodeSection(obj.Code.Raw)

	existingGlobals := make(map[string]bool)
	maxGlobalID := 1000
	if cs.GlobalVars != nil {
		for _, gv := range cs.GlobalVars.Decls {
			existingGlobals[strings.ToLower(gv.Name)] = true
			if id, err := strconv.Atoi(gv.ID); err == nil && id >= maxGlobalID {
				maxGlobalID = id + 1
			}
		}
	}

	var promotedLines []string
	var spanEdits []objectfile.SpanEdit
	var codeEdits []textEdit
	var diags []Diagnostic

	promoteFromVarSection := func(raw string, vs *cal.VarSection, us *diffscope.UnitScope, isTrigger bool, baseIndent int, fileBaseOffset int) []textEdit {
		if us.Mode == diffscope.ModeSkip || vs == nil || len(vs.Decls) == 0 {
			return nil
		}
		var toRemove []*cal.VarDecl
		for _, vd := range vs.Decls {
			if !vd.IsTextConst {
				continue
			}
			if !us.IsNewLocalVar(vd.Name) {
				continue
			}
			lineNum := obj.LineOfOffset(fileBaseOffset + vd.NameStart)
			lower := strings.ToLower(vd.Name)
			if existingGlobals[lower] {
				diags = append(diags, Diagnostic{
					Rule:     config.RuleLocalTextConst,
					Severity: SeverityWarning,
					Line:     lineNum,
					Message:  fmt.Sprintf("local TextConst '%s' conflicts with existing global variable; not moved automatically", vd.Name),
					Fixed:    false,
				})
				continue
			}
			existingGlobals[lower] = true

			// Build global declaration line with fresh @ID if needed.
			declLine := rebuildGlobalVarDecl(vd, &maxGlobalID)
			promotedLines = append(promotedLines, "      "+declLine+obj.LineEnding)
			toRemove = append(toRemove, vd)

			diags = append(diags, Diagnostic{
				Rule:     config.RuleLocalTextConst,
				Severity: SeverityInfo,
				Line:     lineNum,
				Message:  fmt.Sprintf("moved local TextConst '%s' to global VAR", vd.Name),
				Fixed:    true,
			})
		}

		if len(toRemove) == 0 {
			return nil
		}
		return buildVarRemovalEdits(raw, vs, toRemove, isTrigger, baseIndent)
	}

	// Embedded triggers.
	for _, tr := range obj.Triggers {
		us := fscope.TriggerScope(tr)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		td := cal.ParseTrigger(tr.Raw)
		edits := promoteFromVarSection(tr.Raw, td.LocalVars, us, true, tr.BaseIndent, tr.StartOffset)
		if len(edits) > 0 {
			spanEdits = append(spanEdits, objectfile.SpanEdit{
				Start:   tr.StartOffset,
				End:     tr.EndOffset,
				NewText: applyTextEdits(tr.Raw, edits),
			})
		}
	}

	// CODE procedures.
	for _, proc := range cs.Procedures {
		us := fscope.ProcScope(proc)
		if us.Mode == diffscope.ModeSkip {
			continue
		}
		edits := promoteFromVarSection(obj.Code.Raw, proc.LocalVars, us, false, 4, obj.Code.StartOffset)
		codeEdits = append(codeEdits, edits...)
	}

	if len(promotedLines) == 0 {
		return spanEdits, diags
	}

	// Insert promoted TextConst lines into global VAR section (or create global VAR at top of CODE).
	joined := strings.Join(promotedLines, "")
	if cs.GlobalVars != nil {
		codeEdits = append(codeEdits, textEdit{
			start:   cs.GlobalVars.EndOffset,
			end:     cs.GlobalVars.EndOffset,
			newText: joined,
		})
	} else {
		newGlobalBlock := "    VAR" + obj.LineEnding + joined + obj.LineEnding
		codeEdits = append(codeEdits, textEdit{
			start:   0,
			end:     0,
			newText: newGlobalBlock,
		})
	}

	spanEdits = append(spanEdits, objectfile.SpanEdit{
		Start:   obj.Code.StartOffset,
		End:     obj.Code.EndOffset,
		NewText: applyTextEdits(obj.Code.Raw, codeEdits),
	})

	return spanEdits, diags
}

func rebuildGlobalVarDecl(vd *cal.VarDecl, nextID *int) string {
	idStr := ""
	if vd.ID != "" {
		idStr = fmt.Sprintf("@%d", *nextID)
		*nextID++
	}
	return fmt.Sprintf("%s%s : %s;", vd.RawName, idStr, vd.TypeText)
}

func removeUnusedVars(raw string, vs *cal.VarSection, used map[string]bool, eligible func(string) bool, isTrigger bool, baseIndent int, eol string, obj *objectfile.ObjectFile, fileBaseOffset int, ruleCode string) ([]textEdit, []Diagnostic) {
	var toRemove []*cal.VarDecl
	var diags []Diagnostic

	for _, vd := range vs.Decls {
		if eligible != nil && !eligible(vd.Name) {
			continue
		}
		if !used[strings.ToLower(vd.Name)] {
			toRemove = append(toRemove, vd)
			diags = append(diags, Diagnostic{
				Rule:     ruleCode,
				Severity: SeverityInfo,
				Line:     obj.LineOfOffset(fileBaseOffset + vd.NameStart),
				Message:  fmt.Sprintf("removed unused variable '%s'", vd.Name),
				Fixed:    true,
			})
		}
	}

	if len(toRemove) == 0 {
		return nil, diags
	}
	_ = eol
	return buildVarRemovalEdits(raw, vs, toRemove, isTrigger, baseIndent), diags
}

// buildVarRemovalEdits builds textEdits to remove toRemove declarations from vs,
// removing the VAR header too if no declarations remain.
func buildVarRemovalEdits(raw string, vs *cal.VarSection, toRemove []*cal.VarDecl, isTrigger bool, baseIndent int) []textEdit {
	if len(toRemove) == len(vs.Decls) {
		if isTrigger {
			// In an embedded trigger, raw starts with "VAR\r\n  ...\r\n      BEGIN".
			// Removing VAR means the trigger now starts immediately with "BEGIN" (without leading spaces before BEGIN!).
			beginPos := vs.EndOffset
			for beginPos < len(raw) && (raw[beginPos] == ' ' || raw[beginPos] == '\t' || raw[beginPos] == '\r' || raw[beginPos] == '\n') {
				beginPos++
			}
			return []textEdit{{
				start:   0,
				end:     beginPos,
				newText: "",
			}}
		}
		_ = baseIndent
		return []textEdit{{
			start:   vs.StartOffset,
			end:     vs.EndOffset,
			newText: "",
		}}
	}

	var edits []textEdit
	for _, vd := range toRemove {
		edits = append(edits, textEdit{
			start:   vd.StartOffset,
			end:     vd.EndOffset,
			newText: "",
		})
	}
	return edits
}
