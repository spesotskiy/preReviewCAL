package rules

import (
	"fmt"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

// Task 6.3 (rule CAL035 / commented-code). Applied as a cleanup pass before
// formatting in formatCodeBody:
//
//  1. Standalone block comments {...} (own single line or multi-line) are removed.
//     Inline block comments (sharing a line with code, e.g. FALSE {OzonOnly}) stay
//     byte-for-byte; the content/indentation inside any {...} is never analyzed.
//  2. Pure "//" comment lines matching comments.commentedCodePattern are removed.
//     Lines matching comments.codeMarkerPattern (change markers like
//     "// LCG PSY NAV-46641 23.11.2026 <<") are always kept; lines whose text
//     starts with comments.keepCommentPrefix ("// - ...") are always kept;
//     ordinary explanatory comments are kept.
//  3. If between a matching "<<"/">>" marker pair nothing executable and no
//     protected comment remains (after step 2 removed the commented-out code),
//     both marker lines are removed too.
//
// In diff ModePartial only comments fully on changed lines are touched.
func applyCommentRules(body string, changedLines map[int]bool, cfg *config.Config, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
	markerRE, commentedRE := cfg.Comments.Patterns()
	if markerRE == nil || commentedRE == nil {
		return body, nil
	}
	keepPrefix := cfg.Comments.KeepCommentPrefix

	lines := splitLinesKeepEOL(body)
	lineStart := make([]int, len(lines)+1)
	for i, l := range lines {
		lineStart[i+1] = lineStart[i] + len(l)
	}
	lineText := func(ln int) string { // 1-based, without EOL
		text, _ := splitLineEOL(lines[ln-1])
		return text
	}

	type span struct{ start, end int } // [start,end) byte ranges
	var removals []span
	nBlock, nCode := 0, 0

	for _, tok := range cal.Lex(body) {
		switch tok.Kind {
		case cal.TokBlockComment:
			first := tok.Line
			last := first + strings.Count(tok.Text, "\n")
			// Standalone only: nothing but whitespace on its lines around it.
			if !wsOnly(lineText(first)[:tok.Start-lineStart[first-1]]) {
				continue
			}
			if !wsOnly(lineText(last)[tok.End-lineStart[last-1]:]) {
				continue
			}
			if !linesAllChanged(changedLines, first, last) {
				continue
			}
			removals = append(removals, span{lineStart[first-1], lineStart[last]})
			nBlock++
		case cal.TokLineComment:
			ln := tok.Line
			text := lineText(ln)
			pos := strings.Index(text, "//")
			if pos < 0 || !wsOnly(text[:pos]) {
				continue // trailing comment on a code line: keep
			}
			if !linesAllChanged(changedLines, ln, ln) {
				continue
			}
			content := strings.TrimSpace(text[pos+2:])
			if markerRE.MatchString(text) {
				continue // code marker: kept here, pair handling in step 3
			}
			if keepPrefix != "" && strings.HasPrefix(content, keepPrefix) {
				continue // protected explanatory comment
			}
			if commentedRE.MatchString(content) {
				removals = append(removals, span{lineStart[ln-1], lineStart[ln]})
				nCode++
			}
		}
	}

	out := body
	var diags []Diagnostic
	if len(removals) > 0 {
		edits := make([]textEdit, 0, len(removals))
		for _, r := range removals {
			edits = append(edits, textEdit{start: r.start, end: r.end, newText: ""})
		}
		out = applyTextEdits(body, edits)
		if nBlock > 0 {
			diags = append(diags, Diagnostic{
				Rule: config.RuleCommentedCode, Severity: SeverityInfo,
				Line: obj.LineOfOffset(fileBaseOffset),
				Message: fmt.Sprintf("removed %d standalone block comment(s) { ... }", nBlock),
				Fixed:   true,
			})
		}
		if nCode > 0 {
			diags = append(diags, Diagnostic{
				Rule: config.RuleCommentedCode, Severity: SeverityInfo,
				Line: obj.LineOfOffset(fileBaseOffset),
				Message: fmt.Sprintf("removed %d commented-out code line(s)", nCode),
				Fixed:   true,
			})
		}
	}

	// 3. Empty code-marker pairs.
	out2, pairDiags := removeEmptyMarkerPairs(out, changedLines, cfg, obj, fileBaseOffset)
	return out2, append(diags, pairDiags...)
}

type markerLine struct {
	line int
	tag  string
	open bool
}

func removeEmptyMarkerPairs(body string, changedLines map[int]bool, cfg *config.Config, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
	markerRE, _ := cfg.Comments.Patterns()
	if markerRE == nil {
		return body, nil
	}
	keepPrefix := cfg.Comments.KeepCommentPrefix

	lines := splitLinesKeepEOL(body)
	lineStart := make([]int, len(lines)+1)
	for i, l := range lines {
		lineStart[i+1] = lineStart[i] + len(l)
	}

	var markers []markerLine
	protected := map[int]bool{}
	for i, raw := range lines {
		text, _ := splitLineEOL(raw)
		trimmed := strings.TrimSpace(text)
		if !strings.HasPrefix(trimmed, "//") || !wsOnly(text[:strings.Index(text, "//")]) {
			continue
		}
		ln := i + 1
		if markerRE.MatchString(text) {
			m := markerLine{line: ln, open: strings.Contains(trimmed, "<<")}
			tag := strings.Replace(trimmed, "<<", " ", 1)
			if !m.open {
				tag = strings.Replace(trimmed, ">>", " ", 1)
			}
			tag = strings.TrimSpace(strings.TrimPrefix(tag, "//"))
			m.tag = strings.Join(strings.Fields(tag), " ")
			markers = append(markers, m)
			continue
		}
		content := strings.TrimSpace(trimmed[2:])
		if keepPrefix != "" && strings.HasPrefix(content, keepPrefix) {
			protected[ln] = true
		}
	}
	if len(markers) == 0 {
		return body, nil
	}

	codeLine := map[int]bool{}
	for _, tok := range cal.Lex(body) {
		if tok.IsTrivia() || tok.Kind == cal.TokEOF {
			continue
		}
		codeLine[tok.Line] = true
	}

	type removal struct{ start, end int }
	var removals []removal
	nPairs := 0
	var stack []markerLine
	for _, m := range markers {
		if m.open {
			stack = append(stack, m)
			continue
		}
		for k := len(stack) - 1; k >= 0; k-- {
			if stack[k].tag != m.tag {
				continue
			}
			open := stack[k]
			stack = stack[:k]
			empty := true
			for ln := open.line + 1; ln < m.line; ln++ {
				if codeLine[ln] || protected[ln] {
					empty = false
					break
				}
			}
			if !empty {
				break
			}
			if changedLines != nil && (!changedLines[open.line] || !changedLines[m.line]) {
				break
			}
			removals = append(removals,
				removal{lineStart[open.line-1], lineStart[open.line]},
				removal{lineStart[m.line-1], lineStart[m.line]})
			nPairs++
			break
		}
	}
	if nPairs == 0 {
		return body, nil
	}
	edits := make([]textEdit, 0, len(removals))
	for _, r := range removals {
		edits = append(edits, textEdit{start: r.start, end: r.end, newText: ""})
	}
	out := applyTextEdits(body, edits)
	diags := []Diagnostic{{
		Rule: config.RuleCommentedCode, Severity: SeverityInfo,
		Line: obj.LineOfOffset(fileBaseOffset),
		Message: fmt.Sprintf("removed %d empty code-marker pair(s) << / >>", nPairs),
		Fixed:   true,
	}}
	return out, diags
}

func wsOnly(s string) bool { return strings.TrimSpace(s) == "" }

func linesAllChanged(changed map[int]bool, first, last int) bool {
	if changed == nil {
		return true
	}
	for ln := first; ln <= last; ln++ {
		if !changed[ln] {
			return false
		}
	}
	return true
}

// blockCommentLineSet returns 1-based line numbers fully covered by block
// comments ({...}); such lines are never re-indented or respaced (task 6.3).
func blockCommentLineSet(text string) map[int]bool {
	set := map[int]bool{}
	for _, tok := range cal.Lex(text) {
		if tok.Kind != cal.TokBlockComment {
			continue
		}
		last := tok.Line + strings.Count(tok.Text, "\n")
		for ln := tok.Line; ln <= last; ln++ {
			set[ln] = true
		}
	}
	return set
}
