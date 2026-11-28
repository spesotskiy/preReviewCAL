package cal

import (
	"strconv"
	"strings"
)

// AttrDecl represents a C/AL procedure attribute such as [EventSubscriber(...)].
type AttrDecl struct {
	Name        string
	Raw         string
	StartOffset int
	EndOffset   int
}

// VarDecl represents a variable, parameter, or return value declaration.
type VarDecl struct {
	Name         string
	RawName      string
	ID           string
	IsQuoted     bool
	IsVarParam   bool // VAR modifier on procedure parameter
	IsTemporary  bool
	IsWithEvents bool
	IsInDataSet  bool
	IsTextConst  bool
	BaseType     string // e.g. Record, Codeunit, Integer, TextConst, DotNet...
	SubtypeID    int    // e.g. 32 for Record 32
	TypeText     string
	NameStart    int    // exact byte offset of identifier token in source
	NameEnd      int    // exact byte end offset of identifier token in source
	StartOffset  int    // start of declaration (line start for VAR decls, param start for params)
	EndOffset    int    // end of declaration (after trailing newline for VAR decls, before ';' or ')' for params)
	DeclText     string // trimmed declaration text (e.g. "MyVar@1000 : Integer;")
}

// VarSection represents a VAR ... block (global, procedure-local, or trigger-local).
type VarSection struct {
	StartOffset int // start of the "VAR" line
	EndOffset   int // end of last VarDecl (or end of VAR line if empty)
	VarWordPos  int // byte offset of "VAR"
	Decls       []*VarDecl
}

// ProcDecl represents a PROCEDURE / LOCAL PROCEDURE / EVENT declaration in CODE section.
type ProcDecl struct {
	Attributes  []AttrDecl
	IsLocal     bool
	IsEvent     bool
	Name        string
	RawName     string
	ID          string
	NameStart   int
	NameEnd     int
	Params      []*VarDecl
	ReturnVar   *VarDecl
	LocalVars   *VarSection
	HeaderStart int
	HeaderEnd   int
	BodyStart   int // byte offset of BEGIN
	BodyEnd     int // byte offset right after END;
	StartOffset int
	EndOffset   int
}

// HasAttribute reports whether the procedure has an attribute matching name (case-insensitive).
func (p *ProcDecl) HasAttribute(name string) bool {
	for _, a := range p.Attributes {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

// IsEventSubscriber reports whether the procedure is an event subscriber.
func (p *ProcDecl) IsEventSubscriber() bool {
	return p.HasAttribute("EventSubscriber")
}

// IsEventPublisher reports whether the procedure publishes events.
// NAV 2017 uses [Integration] / [Integration(...)] and [Business] / [Business(...)],
// AL uses [IntegrationEvent] and [BusinessEvent] (task 1.4). Case-insensitive.
func (p *ProcDecl) IsEventPublisher() bool {
	return p.HasAttribute("Integration") || p.HasAttribute("Business") ||
		p.HasAttribute("IntegrationEvent") || p.HasAttribute("BusinessEvent")
}

// CodeSection is the parsed structure of a NAV object's CODE section body.
type CodeSection struct {
	Raw        string
	GlobalVars *VarSection
	Procedures []*ProcDecl
	DocStart   int // byte offset of documentation BEGIN ... END. (-1 if none)
	DocEnd     int
}

// TriggerDecl is the parsed structure of an embedded C/AL trigger (OnRun, OnValidate, etc.).
type TriggerDecl struct {
	Raw       string
	LocalVars *VarSection
	BodyStart int
	BodyEnd   int
}

// ParseCodeSection parses the body of a NAV CODE section.
func ParseCodeSection(raw string) *CodeSection {
	cs := &CodeSection{
		Raw:      raw,
		DocStart: -1,
		DocEnd:   -1,
	}
	tokens := Lex(raw)
	i := NextCode(tokens, 0)
	if i == -1 {
		return cs
	}

	// Optional global VAR section at top of CODE.
	if tokens[i].IsWord("VAR") {
		vs, nextIdx := parseVarSection(raw, tokens, i)
		cs.GlobalVars = vs
		i = NextCode(tokens, nextIdx)
	}

	// Parse attributes, procedures/events, and trailing Documentation BEGIN ... END.
	for i != -1 && i < len(tokens) {
		var attrs []AttrDecl
		procStart := lineStartOf(raw, tokens[i].Start)

		// Collect attributes: [AttrName] or [AttrName(...)]
		for i != -1 && tokens[i].Kind == TokLBracket {
			attr, nextIdx := parseAttribute(raw, tokens, i)
			attrs = append(attrs, attr)
			i = NextCode(tokens, nextIdx)
		}
		if i == -1 {
			break
		}

		// Documentation trigger: BEGIN ... END.
		if tokens[i].IsWord("BEGIN") {
			docEndIdx := scanMatchingEnd(tokens, i)
			cs.DocStart = tokens[i].Start
			if docEndIdx >= 0 {
				endOff := tokens[docEndIdx].End
				// Include trailing '.' (or ';')
				nextAfter := NextCode(tokens, docEndIdx+1)
				if nextAfter != -1 && (tokens[nextAfter].Kind == TokDot || tokens[nextAfter].Kind == TokSemi) {
					endOff = tokens[nextAfter].End
				}
				cs.DocEnd = endOff
			} else {
				cs.DocEnd = len(raw)
			}
			break
		}

		// Procedure / LOCAL PROCEDURE / EVENT
		if tokens[i].IsWord("LOCAL") || tokens[i].IsWord("PROCEDURE") || tokens[i].IsWord("EVENT") {
			proc, nextIdx := parseProcedure(raw, tokens, i, attrs, procStart)
			if proc != nil {
				cs.Procedures = append(cs.Procedures, proc)
			}
			i = NextCode(tokens, nextIdx)
			continue
		}

		// Unexpected token: advance safely to avoid infinite loop.
		i = NextCode(tokens, i+1)
	}

	return cs
}

// ParseTrigger parses an embedded trigger body (starting at VAR or BEGIN).
func ParseTrigger(raw string) *TriggerDecl {
	td := &TriggerDecl{
		Raw:       raw,
		BodyStart: -1,
		BodyEnd:   -1,
	}
	tokens := Lex(raw)
	i := NextCode(tokens, 0)
	if i == -1 {
		return td
	}

	if tokens[i].IsWord("VAR") {
		vs, nextIdx := parseVarSection(raw, tokens, i)
		td.LocalVars = vs
		i = NextCode(tokens, nextIdx)
	}

	if i != -1 && tokens[i].IsWord("BEGIN") {
		td.BodyStart = tokens[i].Start
		endIdx := scanMatchingEnd(tokens, i)
		if endIdx >= 0 {
			endOff := tokens[endIdx].End
			nextAfter := NextCode(tokens, endIdx+1)
			if nextAfter != -1 && tokens[nextAfter].Kind == TokSemi {
				endOff = tokens[nextAfter].End
			}
			td.BodyEnd = endOff
		} else {
			td.BodyEnd = len(raw)
		}
	}

	return td
}

func parseAttribute(raw string, tokens []Token, lbracketIdx int) (AttrDecl, int) {
	start := tokens[lbracketIdx].Start
	depth := 0
	name := ""
	i := lbracketIdx
	for i < len(tokens) {
		if tokens[i].Kind == TokLBracket {
			depth++
			if depth == 1 {
				nIdx := NextCode(tokens, i+1)
				if nIdx != -1 && (tokens[nIdx].Kind == TokIdent || tokens[nIdx].Kind == TokQuotedIdent) {
					name = tokens[nIdx].IdentName()
				}
			}
		} else if tokens[i].Kind == TokRBracket {
			depth--
			if depth == 0 {
				end := tokens[i].End
				return AttrDecl{
					Name:        name,
					Raw:         raw[start:end],
					StartOffset: start,
					EndOffset:   end,
				}, i + 1
			}
		}
		i++
	}
	return AttrDecl{Name: name, Raw: raw[start:], StartOffset: start, EndOffset: len(raw)}, len(tokens)
}

func parseProcedure(raw string, tokens []Token, idx int, attrs []AttrDecl, procStart int) (*ProcDecl, int) {
	p := &ProcDecl{
		Attributes:  attrs,
		HeaderStart: tokens[idx].Start,
		StartOffset: procStart,
		BodyStart:   -1,
		BodyEnd:     -1,
	}

	i := idx
	if tokens[i].IsWord("LOCAL") {
		p.IsLocal = true
		i = NextCode(tokens, i+1)
	}
	if i == -1 {
		return p, len(tokens)
	}
	if tokens[i].IsWord("EVENT") {
		p.IsEvent = true
		i = NextCode(tokens, i+1)
	} else if tokens[i].IsWord("PROCEDURE") {
		i = NextCode(tokens, i+1)
	}
	if i == -1 {
		return p, len(tokens)
	}

	// Procedure name (for EVENT it can be Var@ID::EventName@ID)
	if p.IsEvent {
		// Scan until '('
		nameStart := tokens[i].Start
		lastEnd := tokens[i].End
		for i != -1 && tokens[i].Kind != TokLParen && tokens[i].Kind != TokSemi {
			lastEnd = tokens[i].End
			i = NextCode(tokens, i+1)
		}
		p.RawName = strings.TrimSpace(raw[nameStart:lastEnd])
		p.Name = p.RawName
		p.NameStart = nameStart
		p.NameEnd = lastEnd
	} else if tokens[i].Kind == TokIdent || tokens[i].Kind == TokQuotedIdent {
		p.RawName = tokens[i].Text
		p.Name = tokens[i].IdentName()
		p.NameStart = tokens[i].Start
		p.NameEnd = tokens[i].End
		i = NextCode(tokens, i+1)
		if i != -1 && tokens[i].Kind == TokAt {
			idIdx := NextCode(tokens, i+1)
			if idIdx != -1 && tokens[idIdx].Kind == TokNumber {
				p.ID = tokens[idIdx].Text
				i = NextCode(tokens, idIdx+1)
			}
		}
	}

	// Parameter list (...)
	if i != -1 && tokens[i].Kind == TokLParen {
		rpIdx := findMatchingParen(tokens, i)
		if rpIdx > i {
			p.Params = parseParamList(raw, tokens, i+1, rpIdx)
			i = NextCode(tokens, rpIdx+1)
		} else {
			i = NextCode(tokens, i+1)
		}
	}

	// Optional return value: [ReturnName] : ReturnType;
	if i != -1 && tokens[i].Kind != TokSemi {
		if tokens[i].Kind == TokColon {
			// Unnamed return type: ": Boolean;"
			i = scanUntilSemi(tokens, i)
		} else if tokens[i].Kind == TokIdent || tokens[i].Kind == TokQuotedIdent {
			// Named return variable: "Result : Boolean;"
			rv := &VarDecl{
				Name:        tokens[i].IdentName(),
				RawName:     tokens[i].Text,
				IsQuoted:    tokens[i].Kind == TokQuotedIdent,
				NameStart:   tokens[i].Start,
				NameEnd:     tokens[i].End,
				StartOffset: tokens[i].Start,
			}
			next := NextCode(tokens, i+1)
			if next != -1 && tokens[next].Kind == TokAt {
				idIdx := NextCode(tokens, next+1)
				if idIdx != -1 && tokens[idIdx].Kind == TokNumber {
					rv.ID = tokens[idIdx].Text
					next = NextCode(tokens, idIdx+1)
				}
			}
			if next != -1 && tokens[next].Kind == TokColon {
				semiIdx := scanUntilSemi(tokens, next)
				if semiIdx > next {
					typeText := strings.TrimSpace(raw[tokens[next].End:tokens[semiIdx].Start])
					populateVarType(rv, typeText)
					rv.EndOffset = tokens[semiIdx].Start
					rv.DeclText = strings.TrimSpace(raw[rv.StartOffset:rv.EndOffset])
				}
				p.ReturnVar = rv
				i = semiIdx
			} else {
				i = scanUntilSemi(tokens, i)
			}
		} else {
			i = scanUntilSemi(tokens, i)
		}
	}

	if i != -1 && tokens[i].Kind == TokSemi {
		p.HeaderEnd = tokens[i].End
		i = NextCode(tokens, i+1)
	}

	// Optional local VAR section
	if i != -1 && tokens[i].IsWord("VAR") {
		vs, nextIdx := parseVarSection(raw, tokens, i)
		p.LocalVars = vs
		i = NextCode(tokens, nextIdx)
	}

	// Procedure body BEGIN ... END;
	if i != -1 && tokens[i].IsWord("BEGIN") {
		p.BodyStart = tokens[i].Start
		endIdx := scanMatchingEnd(tokens, i)
		if endIdx >= 0 {
			endOff := tokens[endIdx].End
			nextIdx := endIdx + 1
			nextCode := NextCode(tokens, endIdx+1)
			if nextCode != -1 && tokens[nextCode].Kind == TokSemi {
				endOff = tokens[nextCode].End
				nextIdx = nextCode + 1
			}
			p.BodyEnd = endOff
			p.EndOffset = lineEndAfter(raw, endOff)
			return p, nextIdx
		}
		p.BodyEnd = len(raw)
		p.EndOffset = len(raw)
		return p, len(tokens)
	}

	p.EndOffset = p.HeaderEnd
	return p, i
}

func parseParamList(raw string, tokens []Token, startIdx, rpIdx int) []*VarDecl {
	var params []*VarDecl
	i := NextCode(tokens, startIdx)
	for i != -1 && i < rpIdx {
		paramStart := tokens[i].Start
		isVar := false
		if tokens[i].IsWord("VAR") {
			isVar = true
			i = NextCode(tokens, i+1)
			if i == -1 || i >= rpIdx {
				break
			}
		}

		if tokens[i].Kind != TokIdent && tokens[i].Kind != TokQuotedIdent {
			// Skip to next ';'
			for i < rpIdx && tokens[i].Kind != TokSemi {
				i++
			}
			i = NextCode(tokens, i+1)
			continue
		}

		vd := &VarDecl{
			Name:        tokens[i].IdentName(),
			RawName:     tokens[i].Text,
			IsQuoted:    tokens[i].Kind == TokQuotedIdent,
			IsVarParam:  isVar,
			NameStart:   tokens[i].Start,
			NameEnd:     tokens[i].End,
			StartOffset: paramStart,
		}
		i = NextCode(tokens, i+1)
		if i != -1 && i < rpIdx && tokens[i].Kind == TokAt {
			idIdx := NextCode(tokens, i+1)
			if idIdx != -1 && idIdx < rpIdx && tokens[idIdx].Kind == TokNumber {
				vd.ID = tokens[idIdx].Text
				i = NextCode(tokens, idIdx+1)
			}
		}

		if i != -1 && i < rpIdx && tokens[i].Kind == TokColon {
			typeStart := tokens[i].End
			semiIdx := findSemiBefore(tokens, i+1, rpIdx)
			typeEnd := tokens[rpIdx].Start
			if semiIdx != -1 {
				typeEnd = tokens[semiIdx].Start
			}
			typeText := strings.TrimSpace(raw[typeStart:typeEnd])
			populateVarType(vd, typeText)
			vd.EndOffset = typeEnd
			vd.DeclText = strings.TrimSpace(raw[paramStart:typeEnd])
			params = append(params, vd)

			if semiIdx != -1 {
				i = NextCode(tokens, semiIdx+1)
			} else {
				break
			}
		} else {
			break
		}
	}
	return params
}

func parseVarSection(raw string, tokens []Token, varIdx int) (*VarSection, int) {
	vs := &VarSection{
		StartOffset: lineStartOf(raw, tokens[varIdx].Start),
		VarWordPos:  tokens[varIdx].Start,
		EndOffset:   lineEndAfter(raw, tokens[varIdx].End),
	}

	i := NextCode(tokens, varIdx+1)
	for i != -1 && i < len(tokens) {
		tok := tokens[i]
		// Stop when we reach BEGIN, PROCEDURE, LOCAL, EVENT, or an attribute '['.
		if tok.IsWord("BEGIN") || tok.IsWord("PROCEDURE") || tok.IsWord("LOCAL") || tok.IsWord("EVENT") || tok.Kind == TokLBracket {
			break
		}
		if tok.Kind != TokIdent && tok.Kind != TokQuotedIdent {
			break
		}

		declLineStart := lineStartOf(raw, tok.Start)
		vd := &VarDecl{
			Name:        tok.IdentName(),
			RawName:     tok.Text,
			IsQuoted:    tok.Kind == TokQuotedIdent,
			NameStart:   tok.Start,
			NameEnd:     tok.End,
			StartOffset: declLineStart,
		}

		j := NextCode(tokens, i+1)
		if j != -1 && tokens[j].Kind == TokAt {
			idIdx := NextCode(tokens, j+1)
			if idIdx != -1 && tokens[idIdx].Kind == TokNumber {
				vd.ID = tokens[idIdx].Text
				j = NextCode(tokens, idIdx+1)
			}
		}
		if j == -1 || tokens[j].Kind != TokColon {
			break
		}

		typeStart := tokens[j].End
		// Scan until ';' at paren/bracket depth 0.
		// Special handling for TextConst that may wrap quotes across lines.
		semiIdx := scanVarDeclSemi(raw, tokens, j+1)
		if semiIdx == -1 {
			break
		}

		typeText := strings.TrimSpace(raw[typeStart:tokens[semiIdx].Start])
		populateVarType(vd, typeText)
		vd.DeclText = strings.TrimSpace(raw[tok.Start:tokens[semiIdx].End])
		vd.EndOffset = lineEndAfter(raw, tokens[semiIdx].End)
		vs.Decls = append(vs.Decls, vd)
		vs.EndOffset = vd.EndOffset

		i = NextCode(tokens, semiIdx+1)
	}

	return vs, i
}

// scanVarDeclSemi finds the terminating ';' of a VAR declaration.
func scanVarDeclSemi(raw string, tokens []Token, startIdx int) int {
	// Check if this is a TextConst declaration (could have multi-line wrapped string).
	firstType := NextCode(tokens, startIdx)
	if firstType != -1 && tokens[firstType].IsWord("TextConst") {
		// Scan raw bytes from after TextConst tracking single-quoted strings across newlines!
		pos := tokens[firstType].End
		inStr := false
		for pos < len(raw) {
			ch := raw[pos]
			if ch == '\'' {
				if inStr {
					if pos+1 < len(raw) && raw[pos+1] == '\'' {
						pos += 2
						continue
					}
					inStr = false
				} else {
					inStr = true
				}
				pos++
				continue
			}
			if !inStr && ch == ';' {
				// Find the token starting at pos.
				for k := firstType + 1; k < len(tokens); k++ {
					if tokens[k].Start == pos && tokens[k].Kind == TokSemi {
						return k
					}
					if tokens[k].Start > pos {
						break
					}
				}
				break
			}
			pos++
		}
	}

	pDepth := 0
	bDepth := 0
	for k := startIdx; k < len(tokens); k++ {
		switch tokens[k].Kind {
		case TokLParen:
			pDepth++
		case TokRParen:
			if pDepth > 0 {
				pDepth--
			}
		case TokLBracket:
			bDepth++
		case TokRBracket:
			if bDepth > 0 {
				bDepth--
			}
		case TokSemi:
			if pDepth == 0 && bDepth == 0 {
				return k
			}
		}
	}
	return -1
}

func populateVarType(vd *VarDecl, typeText string) {
	vd.TypeText = typeText
	tTokens := Lex(typeText)
	var words []string
	for _, t := range tTokens {
		if t.IsTrivia() {
			continue
		}
		if t.Kind == TokIdent {
			upper := strings.ToUpper(t.Text)
			switch upper {
			case "TEMPORARY":
				vd.IsTemporary = true
			case "WITHEVENTS":
				vd.IsWithEvents = true
			case "INDATASET":
				vd.IsInDataSet = true
			case "TEXTCONST":
				vd.IsTextConst = true
			}
			words = append(words, t.Text)
		} else if t.Kind == TokNumber {
			words = append(words, t.Text)
		}
	}

	// Determine BaseType and optional SubtypeID (e.g. "TEMPORARY Record 32", "ARRAY [10] OF Codeunit 80").
	idx := 0
	// Skip ARRAY ... OF
	for idx < len(words) {
		if strings.EqualFold(words[idx], "ARRAY") {
			for idx < len(words) && !strings.EqualFold(words[idx], "OF") {
				idx++
			}
			if idx < len(words) && strings.EqualFold(words[idx], "OF") {
				idx++
			}
			continue
		}
		if strings.EqualFold(words[idx], "TEMPORARY") {
			idx++
			continue
		}
		break
	}
	if idx < len(words) {
		vd.BaseType = words[idx]
		if idx+1 < len(words) {
			if id, err := strconv.Atoi(words[idx+1]); err == nil {
				vd.SubtypeID = id
			}
		}
	}
}

func scanMatchingEnd(tokens []Token, beginIdx int) int {
	depth := 0
	for i := beginIdx; i < len(tokens); i++ {
		if tokens[i].IsTrivia() {
			continue
		}
		if tokens[i].IsWord("BEGIN") || tokens[i].IsWord("CASE") {
			depth++
		} else if tokens[i].IsWord("END") {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findMatchingParen(tokens []Token, lpIdx int) int {
	depth := 0
	for i := lpIdx; i < len(tokens); i++ {
		if tokens[i].Kind == TokLParen {
			depth++
		} else if tokens[i].Kind == TokRParen {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findSemiBefore(tokens []Token, startIdx, endIdx int) int {
	pDepth := 0
	bDepth := 0
	for i := startIdx; i < endIdx; i++ {
		switch tokens[i].Kind {
		case TokLParen:
			pDepth++
		case TokRParen:
			if pDepth > 0 {
				pDepth--
			}
		case TokLBracket:
			bDepth++
		case TokRBracket:
			if bDepth > 0 {
				bDepth--
			}
		case TokSemi:
			if pDepth == 0 && bDepth == 0 {
				return i
			}
		}
	}
	return -1
}

func scanUntilSemi(tokens []Token, startIdx int) int {
	for i := startIdx; i < len(tokens); i++ {
		if tokens[i].Kind == TokSemi {
			return i
		}
	}
	return len(tokens) - 1
}

func lineStartOf(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	idx := strings.LastIndexByte(s[:pos], '\n')
	if idx == -1 {
		return 0
	}
	return idx + 1
}

func lineEndAfter(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	idx := strings.IndexByte(s[pos:], '\n')
	if idx == -1 {
		return len(s)
	}
	return pos + idx + 1
}
