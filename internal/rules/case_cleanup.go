package rules

import (
	"fmt"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

// Task 6.1: cleanup of empty CASE constructs, applied as a pass before formatting.
//
//	1. remove empty "ELSE;" sections inside CASE;
//	2. remove CASE arms with an empty body ("Status::New: ;") when the CASE has no
//	   ELSE section (an empty "ELSE;" is removed first, so it does not count);
//	3. when no executable code remains in "CASE ... END;" - remove the whole
//	   construct; if the selector contains a procedure/function call (e.g.
//	   "CASE Next() OF"), do NOT remove silently - emit a warning about possible
//	   side effects and leave the block untouched.
//
// Reported under CAL034 (dead-statement cleanup family). In diff ModePartial only
// arms/ELSE/CASE fully located on changed lines are removed.

type caseArm struct {
	labelStart int // first code token index of the arm (label)
	colonIdx   int // index of the ':' ending the label
	semiIdx    int // index of the ';' ending the arm, or -1
	empty      bool
}

type caseParse struct {
	caseIdx   int
	ofIdx     int
	endIdx    int
	arms      []caseArm
	elseIdx   int // -1 when no ELSE
	elseSemi  int // index of ';' ending the ELSE section, or -1
	elseEmpty bool
}

func codeTokensBetween(tokens []cal.Token, from, to int) int {
	n := 0
	for i := from; i < to && i < len(tokens); i++ {
		if i >= 0 && !tokens[i].IsTrivia() && tokens[i].Kind != cal.TokEOF {
			n++
		}
	}
	return n
}

func commentsBetween(tokens []cal.Token, from, to int) bool {
	for i := from; i < to && i < len(tokens); i++ {
		if i >= 0 && tokens[i].IsComment() {
			return true
		}
	}
	return false
}

// analyzeCase parses one CASE ... OF ... END block starting at caseIdx.
func analyzeCase(tokens []cal.Token, caseIdx int) (*caseParse, bool) {
	p := &caseParse{caseIdx: caseIdx, elseIdx: -1, elseSemi: -1}

	// Locate OF at selector depth 0.
	pDepth, bDepth := 0, 0
	for i := caseIdx + 1; i < len(tokens); i++ {
		t := tokens[i]
		if t.IsTrivia() {
			continue
		}
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
			continue
		}
		if t.Kind == cal.TokLParen || t.Kind == cal.TokLBracket || t.Kind == cal.TokRParen || t.Kind == cal.TokRBracket {
			continue
		}
		if pDepth == 0 && bDepth == 0 {
			if t.IsWord("OF") && !isMemberAccess(tokens, i) {
				p.ofIdx = i
				break
			}
			if t.Kind == cal.TokSemi || t.Kind == cal.TokAssign {
				return nil, false
			}
		}
	}
	if p.ofIdx == -1 {
		return nil, false
	}

	// Scan the CASE body.
	depth := 1
	pDepth, bDepth = 0, 0
	armStart := -1
	colonIdx := -1
	armCode := 0  // code tokens after the colon of the current arm
	armComment := false
	armSeenColon := false
	inElse := false
	elseCode := 0
	elseComment := false
	for i := p.ofIdx + 1; i < len(tokens); i++ {
		t := tokens[i]
		if t.IsTrivia() {
			if t.IsComment() {
				if inElse {
					elseComment = true
				} else if armSeenColon {
					armComment = true
				}
			}
			continue
		}
		switch t.Kind {
		case cal.TokLParen:
			pDepth++
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			} else {
				if armStart == -1 {
					armStart = i
				}
			}
			continue
		case cal.TokRParen:
			if pDepth > 0 {
				pDepth--
			}
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			}
			continue
		case cal.TokLBracket:
			bDepth++
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			} else if armStart == -1 {
				armStart = i
			}
			continue
		case cal.TokRBracket:
			if bDepth > 0 {
				bDepth--
			}
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			}
			continue
		}
		if pDepth != 0 || bDepth != 0 {
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			} else if armStart == -1 {
				armStart = i
			}
			continue
		}

		switch {
		case t.IsWord("BEGIN") || t.IsWord("CASE") || t.IsWord("REPEAT"):
			if isMemberAccess(tokens, i) {
				break
			}
			if t.IsWord("CASE") {
				if inElse {
					elseCode++
				} else if armSeenColon {
					armCode++
				} else if armStart == -1 {
					armStart = i
				}
			} else {
				depth++
				if inElse {
					elseCode++
				} else if armSeenColon {
					armCode++
				} else if armStart == -1 {
					armStart = i
				}
				continue
			}
			if t.IsWord("CASE") {
				depth++
			}
			continue
		case t.IsWord("UNTIL"):
			if isMemberAccess(tokens, i) {
				break
			}
			depth--
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			}
			continue
		case t.IsWord("END"):
			if isMemberAccess(tokens, i) {
				break
			}
			if depth == 1 {
				// End of this CASE: close a pending arm/else without ';'.
				if !inElse && armSeenColon {
					p.arms = append(p.arms, caseArm{
						labelStart: armStart, colonIdx: colonIdx, semiIdx: -1,
						empty: armCode == 0 && !armComment,
					})
				}
				if inElse {
					p.elseEmpty = elseCode == 0 && !elseComment
				}
				p.endIdx = i
				return p, true
			}
			depth--
			if inElse {
				elseCode++
			} else if armSeenColon {
				armCode++
			} else if armStart == -1 {
				armStart = i
			}
			continue
		case t.IsWord("ELSE") && !isMemberAccess(tokens, i):
			if inElse {
				elseCode++
				continue
			}
			// Close a pending arm without ';'.
			if armSeenColon {
				p.arms = append(p.arms, caseArm{
					labelStart: armStart, colonIdx: colonIdx, semiIdx: -1,
					empty: armCode == 0 && !armComment,
				})
			}
			elseIdx := i
			p.elseIdx = elseIdx
			inElse = true
			armStart, colonIdx, armCode, armComment, armSeenColon = -1, -1, 0, false, false
			continue
		case t.Kind == cal.TokColon:
			if inElse {
				elseCode++
				continue
			}
			if armSeenColon {
				// second colon before ';' - treat label as compound (rare), arm not empty.
				armCode++
				continue
			}
			if armStart == -1 {
				armStart = i
			}
			colonIdx = i
			armSeenColon = true
			armCode, armComment = 0, false
			continue
		case t.Kind == cal.TokSemi:
			if inElse {
				p.elseEmpty = elseCode == 0 && !elseComment
				p.elseSemi = i
				inElse = false
				elseCode, elseComment = 0, false
				armStart, colonIdx, armCode, armComment, armSeenColon = -1, -1, 0, false, false
				continue
			}
			if armSeenColon {
				p.arms = append(p.arms, caseArm{
					labelStart: armStart, colonIdx: colonIdx, semiIdx: i,
					empty: armCode == 0 && !armComment,
				})
			}
			armStart, colonIdx, armCode, armComment, armSeenColon = -1, -1, 0, false, false
			continue
		}

		// Ordinary code token.
		if inElse {
			elseCode++
			continue
		}
		if armSeenColon {
			armCode++
			continue
		}
		if armStart == -1 {
			armStart = i
		}
	}
	return nil, false
}

// selectorHasCall reports whether the CASE selector expression contains a
// procedure/function call (side effects possible).
func selectorHasCall(tokens []cal.Token, p *caseParse) bool {
	prevIdx := -1
	for i := p.caseIdx + 1; i < p.ofIdx; i++ {
		t := tokens[i]
		if t.IsTrivia() {
			continue
		}
		if t.Kind == cal.TokLParen && prevIdx != -1 {
			pt := tokens[prevIdx]
			if pt.Kind == cal.TokIdent || pt.Kind == cal.TokQuotedIdent || pt.Kind == cal.TokRParen || pt.Kind == cal.TokRBracket {
				if !isMemberAccess(tokens, i) || pt.Kind == cal.TokRParen {
					return true
				}
			}
		}
		prevIdx = i
	}
	return false
}

// deletionRange returns a byte range [start,end) for tokens [fromIdx..toIdx],
// extended to whole lines when the partial lines contain only whitespace.
func deletionRange(s string, tokens []cal.Token, fromIdx, toIdx int) (int, int) {
	start := tokens[fromIdx].Start
	ls := lineStartPos(s, start)
	if strings.TrimSpace(s[ls:start]) == "" {
		start = ls
	}
	end := tokens[toIdx].End
	rest := s[end:]
	if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
		if strings.TrimSpace(rest[:idx]) == "" {
			end += idx + 1
		}
	} else if strings.TrimSpace(rest) == "" {
		end = len(s)
	}
	return start, end
}

func tokensChangedOK(tokens []cal.Token, fromIdx, toIdx int, changedLines map[int]bool) bool {
	if changedLines == nil {
		return true
	}
	for i := fromIdx; i <= toIdx && i < len(tokens); i++ {
		if tokens[i].IsTrivia() || tokens[i].Kind == cal.TokEOF {
			continue
		}
		if !changedLines[tokens[i].Line] {
			return false
		}
	}
	return true
}

// applyCaseCleanup removes empty CASE parts per task 6.1. Returns the rewritten
// body and diagnostics (lines are relative to fileBaseOffset in obj).
func applyCaseCleanup(body string, changedLines map[int]bool, obj *objectfile.ObjectFile, fileBaseOffset int) (string, []Diagnostic) {
	cur := body
	var diags []Diagnostic

	for pass := 0; pass < 10; pass++ {
		tokens := cal.Lex(cur)
		type span struct {
			start, end int
			diag       Diagnostic
		}
		var spans []span
		changed := false

		for i := 0; i < len(tokens); i++ {
			if !tokens[i].IsWord("CASE") || isMemberAccess(tokens, i) {
				continue
			}
			p, ok := analyzeCase(tokens, i)
			if !ok {
				continue
			}
			// Do NOT fast-forward past END: nested CASE blocks must be analyzed too.

			nonEmptyArms := 0
			for _, a := range p.arms {
				if !a.empty {
					nonEmptyArms++
				}
			}
			elseWithContent := p.elseIdx != -1 && !p.elseEmpty
			allEmpty := nonEmptyArms == 0 && !elseWithContent

			if allEmpty {
				if selectorHasCall(tokens, p) {
					lineNum := obj.LineOfOffset(fileBaseOffset + tokens[p.caseIdx].Start)
					diags = append(diags, Diagnostic{
						Rule:     config.RuleBlankLinesAndSemicols,
						Severity: SeverityWarning,
						Line:     lineNum,
						Message:  fmt.Sprintf("empty CASE at line %d was NOT removed: its selector '%s' may contain a call with side effects",
							lineNum, strings.TrimSpace(cur[tokens[p.caseIdx].End:tokens[p.ofIdx].Start])),
						Fixed: false,
					})
					continue
				}
				// Do not remove a CASE that occupies the only statement slot of IF/WHILE/FOR/ELSE.
				prev := cal.PrevCode(tokens, p.caseIdx-1)
				if prev != -1 && (tokens[prev].IsWord("THEN") || tokens[prev].IsWord("DO") || tokens[prev].IsWord("ELSE")) {
					continue
				}
				endTok := p.endIdx
				after := cal.NextCode(tokens, p.endIdx+1)
				if after != -1 && tokens[after].Kind == cal.TokSemi {
					endTok = after
				}
				if !tokensChangedOK(tokens, p.caseIdx, endTok, changedLines) {
					continue
				}
				st, en := deletionRange(cur, tokens, p.caseIdx, endTok)
				lineNum := obj.LineOfOffset(fileBaseOffset + tokens[p.caseIdx].Start)
				spans = append(spans, span{start: st, end: en, diag: Diagnostic{
					Rule:     config.RuleBlankLinesAndSemicols,
					Severity: SeverityInfo,
					Line:     lineNum,
					Message:  "removed empty CASE ... END construct",
					Fixed:    true,
				}})
				changed = true
				continue
			}

			// 1. Empty ELSE section.
			if p.elseIdx != -1 && p.elseEmpty {
				endTok := p.elseIdx
				if p.elseSemi != -1 {
					endTok = p.elseSemi
				}
				if tokensChangedOK(tokens, p.elseIdx, endTok, changedLines) {
					st, en := deletionRange(cur, tokens, p.elseIdx, endTok)
					lineNum := obj.LineOfOffset(fileBaseOffset + tokens[p.elseIdx].Start)
					spans = append(spans, span{start: st, end: en, diag: Diagnostic{
						Rule:     config.RuleBlankLinesAndSemicols,
						Severity: SeverityInfo,
						Line:     lineNum,
						Message:  "removed empty ELSE section in CASE",
						Fixed:    true,
					}})
					changed = true
				}
			}

			// 2. Empty arms only when the CASE has no (contentful) ELSE section.
			if !elseWithContent {
				for _, a := range p.arms {
					if !a.empty || a.semiIdx == -1 {
						continue
					}
					if !tokensChangedOK(tokens, a.labelStart, a.semiIdx, changedLines) {
						continue
					}
					st, en := deletionRange(cur, tokens, a.labelStart, a.semiIdx)
					lineNum := obj.LineOfOffset(fileBaseOffset + tokens[a.labelStart].Start)
					spans = append(spans, span{start: st, end: en, diag: Diagnostic{
						Rule:     config.RuleBlankLinesAndSemicols,
						Severity: SeverityInfo,
						Line:     lineNum,
						Message:  fmt.Sprintf("removed empty CASE branch '%s'", strings.TrimSpace(cur[tokens[a.labelStart].Start:tokens[a.colonIdx].End])),
						Fixed:    true,
					}})
					changed = true
				}
			}
		}

		if !changed {
			break
		}
		var edits []textEdit
		for _, sp := range spans {
			edits = append(edits, textEdit{start: sp.start, end: sp.end})
			diags = append(diags, sp.diag)
		}
		cur = applyTextEdits(cur, edits)
	}

	return cur, diags
}
