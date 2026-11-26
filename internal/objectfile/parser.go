package objectfile

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Section represents a top-level section inside a NAV 2017 .txt object.
type Section struct {
	Name        string
	StartOffset int // offset of the section header line
	BodyStart   int // offset immediately after the opening "{\r\n" (or "{\n")
	BodyEnd     int // offset of the closing "  }" line
	EndOffset   int // offset immediately after the closing "}"
}

// TriggerBlock represents an embedded C/AL trigger (e.g. OnRun, OnValidate, OnAction).
type TriggerBlock struct {
	Name                 string
	SectionName          string
	OwnerKey             string // stable key e.g. "PROPERTIES:OnRun" or "FIELDS:2:OnValidate"
	StartOffset          int    // starts at VAR or BEGIN
	EndOffset            int    // ends right after END (including trailing ';' if present)
	BaseIndent           int    // visual column offset of VAR/BEGIN on the first line
	HasTrailingSemicolon bool   // true if ended with "END;"
	Raw                  string
}

// CodeBlock represents the CODE section body between "{\n" and "  }".
type CodeBlock struct {
	StartOffset int
	EndOffset   int
	Raw         string
}

// TableField holds a field parsed from a Table's FIELDS section.
type TableField struct {
	ID       int
	Name     string
	DataType string
}

// DataItemDecl holds a Report DATASET DataItem or XMLport Table element.
type DataItemDecl struct {
	Name    string
	TableID int
}

// ObjectFile is the lossless Level-1 representation of a NAV 2017 .txt object file.
type ObjectFile struct {
	Raw          string
	LineEnding   string // "\r\n" or "\n"
	HadBOM       bool
	ObjectType   string
	ObjectID     int
	ObjectName   string
	SourceTable  int // TableNo / SourceTable ID if present (or ObjectID for Table)
	Sections     []Section
	Code         *CodeBlock
	Triggers     []TriggerBlock
	TableFields  []TableField
	DataItems    []DataItemDecl
	ExternalRefs map[string]bool // lowercased identifiers referenced in non-C/AL sections
	HasRDLData   bool
}

// SpanEdit represents a replacement for a byte range [Start, End) in ObjectFile.Raw.
type SpanEdit struct {
	Start   int
	End     int
	NewText string
}

var objectHeaderRe = regexp.MustCompile(`^OBJECT\s+(Table|Form|Report|Dataport|Codeunit|XMLport|MenuSuite|Page|Query)\s+(\d+)\s+(.+)$`)
var sourceTableRe = regexp.MustCompile(`(?i)\b(?:SourceTable|TableNo)\s*=\s*(?:Table)?(\d+)\b`)

// Parse parses a single NAV 2017 .txt object file without losing any bytes.
func Parse(data []byte) (*ObjectFile, error) {
	hadBOM := bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if hadBOM {
		data = data[3:]
	}
	raw := string(data)

	lineEnding := "\r\n"
	if !strings.Contains(raw, "\r\n") && strings.Contains(raw, "\n") {
		lineEnding = "\n"
	}

	obj := &ObjectFile{
		Raw:          raw,
		LineEnding:   lineEnding,
		HadBOM:       hadBOM,
		ExternalRefs: make(map[string]bool),
	}

	// Parse first non-empty line as OBJECT header.
	pos := 0
	for pos < len(raw) {
		lineEnd := nextLineEnd(raw, pos)
		line := strings.TrimRight(raw[pos:lineEnd], "\r\n")
		if strings.TrimSpace(line) != "" {
			m := objectHeaderRe.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				return nil, fmt.Errorf("invalid NAV object header: %q", line)
			}
			obj.ObjectType = canonicalType(m[1])
			id, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, fmt.Errorf("invalid object ID %q: %w", m[2], err)
			}
			obj.ObjectID = id
			obj.ObjectName = strings.TrimSpace(m[3])
			if obj.ObjectType == "Table" {
				obj.SourceTable = obj.ObjectID
			}
			pos = lineEnd
			break
		}
		pos = lineEnd
	}

	if obj.ObjectType == "" {
		return nil, fmt.Errorf("empty NAV object file")
	}

	// Find opening '{' of the object.
	for pos < len(raw) {
		lineEnd := nextLineEnd(raw, pos)
		if strings.TrimSpace(raw[pos:lineEnd]) == "{" {
			pos = lineEnd
			break
		}
		pos = lineEnd
	}

	// Scan top-level sections.
	for pos < len(raw) {
		lineEnd := nextLineEnd(raw, pos)
		trimmed := strings.TrimSpace(raw[pos:lineEnd])
		if trimmed == "}" {
			break
		}
		if trimmed == "" {
			pos = lineEnd
			continue
		}

		if isTopLevelSection(trimmed) {
			secStart := pos
			secName := trimmed
			openLineStart := lineEnd
			openLineEnd := nextLineEnd(raw, openLineStart)
			if strings.TrimSpace(raw[openLineStart:openLineEnd]) != "{" {
				pos = lineEnd
				continue
			}

			bodyStart := openLineEnd
			var bodyEnd, secEnd int

			switch secName {
			case "RDLDATA":
				obj.HasRDLData = true
				bodyEnd, secEnd = scanRawTerminatedSection(raw, bodyStart, "END_OF_RDLDATA")
			case "WORDLAYOUT":
				bodyEnd, secEnd = scanRawTerminatedSection(raw, bodyStart, "END_OF_WORDLAYOUT")
			case "CODE":
				bodyEnd, secEnd = scanCodeSectionEnd(raw, bodyStart)
				obj.Code = &CodeBlock{
					StartOffset: bodyStart,
					EndOffset:   bodyEnd,
					Raw:         raw[bodyStart:bodyEnd],
				}
			default:
				var triggers []TriggerBlock
				bodyEnd, secEnd, triggers = scanStructuredSection(raw, bodyStart, secName)
				obj.Triggers = append(obj.Triggers, triggers...)
				extractSectionMetadata(obj, secName, raw[bodyStart:bodyEnd], triggers, bodyStart)
			}

			obj.Sections = append(obj.Sections, Section{
				Name:        secName,
				StartOffset: secStart,
				BodyStart:   bodyStart,
				BodyEnd:     bodyEnd,
				EndOffset:   secEnd,
			})
			pos = nextLineEnd(raw, secEnd)
			continue
		}

		pos = lineEnd
	}

	return obj, nil
}

// ApplyEdits produces the updated file bytes by applying non-overlapping SpanEdits to Raw.
// Output is always UTF-8 without BOM.
func (o *ObjectFile) ApplyEdits(edits []SpanEdit) []byte {
	if len(edits) == 0 {
		return []byte(o.Raw)
	}
	sorted := make([]SpanEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})

	var b strings.Builder
	b.Grow(len(o.Raw))
	cursor := 0
	for _, ed := range sorted {
		if ed.Start < cursor {
			continue
		}
		b.WriteString(o.Raw[cursor:ed.Start])
		b.WriteString(ed.NewText)
		cursor = ed.End
	}
	b.WriteString(o.Raw[cursor:])
	return []byte(b.String())
}

// LineOfOffset returns 1-based line number for a byte offset in Raw.
func (o *ObjectFile) LineOfOffset(offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(o.Raw) {
		offset = len(o.Raw)
	}
	return 1 + strings.Count(o.Raw[:offset], "\n")
}

func isTopLevelSection(name string) bool {
	switch name {
	case "OBJECT-PROPERTIES", "PROPERTIES", "FIELDS", "KEYS", "FIELDGROUPS",
		"CONTROLS", "DATASET", "REQUESTPAGE", "LABELS", "ELEMENTS",
		"MENUNODES", "CODE", "RDLDATA", "WORDLAYOUT":
		return true
	default:
		return false
	}
}

func canonicalType(s string) string {
	switch strings.ToLower(s) {
	case "table":
		return "Table"
	case "page":
		return "Page"
	case "report":
		return "Report"
	case "codeunit":
		return "Codeunit"
	case "xmlport":
		return "XMLport"
	case "query":
		return "Query"
	case "menusuite":
		return "MenuSuite"
	case "form":
		return "Form"
	case "dataport":
		return "Dataport"
	default:
		return s
	}
}

func nextLineEnd(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	idx := strings.IndexByte(s[pos:], '\n')
	if idx == -1 {
		return len(s)
	}
	return pos + idx + 1
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

func scanRawTerminatedSection(raw string, bodyStart int, sentinel string) (bodyEnd int, secEnd int) {
	pos := bodyStart
	foundSentinel := false
	for pos < len(raw) {
		lineEnd := nextLineEnd(raw, pos)
		trimmed := strings.TrimSpace(raw[pos:lineEnd])
		if !foundSentinel {
			if trimmed == sentinel {
				foundSentinel = true
			}
			pos = lineEnd
			continue
		}
		if trimmed == "}" {
			closeIdx := strings.IndexByte(raw[pos:lineEnd], '}')
			return pos, pos + closeIdx + 1
		}
		pos = lineEnd
	}
	return len(raw), len(raw)
}

// scanCodeSectionEnd scans a CODE section while respecting C/AL strings, // comments, and { } comments.
func scanCodeSectionEnd(raw string, bodyStart int) (bodyEnd int, secEnd int) {
	depth := 1
	i := bodyStart
	for i < len(raw) {
		ch := raw[i]
		// C/AL string literal '...'
		if ch == '\'' {
			i++
			for i < len(raw) {
				if raw[i] == '\'' {
					if i+1 < len(raw) && raw[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				if raw[i] == '\n' {
					break
				}
				i++
			}
			continue
		}
		// Line comment //
		if ch == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			i = nextLineEnd(raw, i)
			continue
		}
		if ch == '{' {
			depth++
			i++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return lineStartOf(raw, i), i + 1
			}
			i++
			continue
		}
		i++
	}
	return len(raw), len(raw)
}

// scanStructuredSection scans PROPERTIES, FIELDS, CONTROLS, DATASET, REQUESTPAGE, ELEMENTS, etc.
// It finds the matching closing '}' of the section and extracts all embedded C/AL triggers.
func scanStructuredSection(raw string, bodyStart int, sectionName string) (bodyEnd int, secEnd int, triggers []TriggerBlock) {
	depth := 1
	i := bodyStart
	currentEntryID := ""
	for i < len(raw) {
		ch := raw[i]

		// Detect embedded trigger: On<Ident>=VAR or On<Ident>=BEGIN
		if (ch == 'O' || ch == 'o') && (i == bodyStart || isTriggerBoundary(raw[i-1])) {
			if trig, nextPos, ok := tryScanEmbeddedTrigger(raw, i, sectionName); ok {
				if currentEntryID != "" {
					trig.OwnerKey = sectionName + ":" + currentEntryID + ":" + trig.Name
				} else {
					trig.OwnerKey = sectionName + ":" + trig.Name
				}
				triggers = append(triggers, trig)
				i = nextPos
				continue
			}
		}

		// Bracketed ML strings like CaptionML=[ENU=...;RUS=...] can contain braces or semicolons.
		if ch == '[' {
			i = skipBracketValue(raw, i)
			continue
		}

		if ch == '{' {
			if depth == 1 {
				currentEntryID = peekEntryID(raw, i+1)
			}
			depth++
			i++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 1 {
				currentEntryID = ""
			}
			if depth == 0 {
				return lineStartOf(raw, i), i + 1, triggers
			}
			i++
			continue
		}
		i++
	}
	return len(raw), len(raw), triggers
}

func peekEntryID(raw string, start int) string {
	// Read trimmed token before first ';' or '}' or newline (handles numeric IDs and XMLport [{GUID}]).
	end := start
	bDepth := 0
	for end < len(raw) {
		ch := raw[end]
		if ch == '[' {
			bDepth++
		} else if ch == ']' && bDepth > 0 {
			bDepth--
		} else if bDepth == 0 && (ch == ';' || ch == '}' || ch == '\n' || ch == '\r') {
			break
		}
		end++
	}
	return strings.TrimSpace(raw[start:end])
}

func isTriggerBoundary(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == ';' || b == '{'
}

func skipBracketValue(raw string, start int) int {
	depth := 0
	i := start
	for i < len(raw) {
		switch raw[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return len(raw)
}

func tryScanEmbeddedTrigger(raw string, pos int, sectionName string) (TriggerBlock, int, bool) {
	// Read identifier starting with On...
	j := pos
	for j < len(raw) && isASCIIIdentPart(raw[j]) {
		j++
	}
	name := raw[pos:j]
	if len(name) < 3 || !strings.EqualFold(name[:2], "On") {
		return TriggerBlock{}, pos, false
	}
	if j >= len(raw) || raw[j] != '=' {
		return TriggerBlock{}, pos, false
	}
	trigStart := j + 1
	if trigStart >= len(raw) {
		return TriggerBlock{}, pos, false
	}

	// Must start with VAR or BEGIN keyword right after '='.
	kw, _ := peekKeyword(raw, trigStart)
	if !strings.EqualFold(kw, "VAR") && !strings.EqualFold(kw, "BEGIN") {
		return TriggerBlock{}, pos, false
	}

	endPos, hasSemi, ok := scanCALTriggerBody(raw, trigStart)
	if !ok {
		return TriggerBlock{}, pos, false
	}

	ls := lineStartOf(raw, trigStart)
	baseIndent := utf8.RuneCountInString(raw[ls:trigStart])

	return TriggerBlock{
		Name:                 name,
		SectionName:          sectionName,
		StartOffset:          trigStart,
		EndOffset:            endPos,
		BaseIndent:           baseIndent,
		HasTrailingSemicolon: hasSemi,
		Raw:                  raw[trigStart:endPos],
	}, endPos, true
}

// scanCALTriggerBody scans a C/AL trigger starting at VAR or BEGIN until the matching END[;].
func scanCALTriggerBody(raw string, start int) (endPos int, hasSemicolon bool, ok bool) {
	i := start
	inCode := false
	depth := 0

	for i < len(raw) {
		ch := raw[i]

		// String literal '...'
		if ch == '\'' {
			i++
			for i < len(raw) {
				if raw[i] == '\'' {
					if i+1 < len(raw) && raw[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				if raw[i] == '\n' {
					break
				}
				i++
			}
			continue
		}

		// Quoted identifier "..."
		if ch == '"' {
			i++
			for i < len(raw) {
				if raw[i] == '"' {
					if i+1 < len(raw) && raw[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				if raw[i] == '\n' {
					break
				}
				i++
			}
			continue
		}

		// Line comment //
		if ch == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			i = nextLineEnd(raw, i)
			continue
		}

		// Block comment { ... } inside C/AL code or VAR
		if ch == '{' {
			cDepth := 1
			i++
			for i < len(raw) && cDepth > 0 {
				if raw[i] == '{' {
					cDepth++
				} else if raw[i] == '}' {
					cDepth--
				}
				i++
			}
			continue
		}

		if isASCIIIdentStart(ch) {
			wordStart := i
			for i < len(raw) && isASCIIIdentPart(raw[i]) {
				i++
			}
			word := strings.ToUpper(raw[wordStart:i])
			switch word {
			case "BEGIN":
				inCode = true
				depth++
			case "CASE":
				if inCode {
					depth++
				}
			case "END":
				if inCode {
					depth--
					if depth == 0 {
						// Check optional trailing semicolon (possibly after whitespace? No, in C/AL END; is immediate).
						if i < len(raw) && raw[i] == ';' {
							return i + 1, true, true
						}
						return i, false, true
					}
				}
			}
			continue
		}

		i++
	}
	return start, false, false
}

func peekKeyword(raw string, pos int) (string, int) {
	i := pos
	for i < len(raw) && isASCIIIdentPart(raw[i]) {
		i++
	}
	return raw[pos:i], i
}

func isASCIIIdentStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isASCIIIdentPart(b byte) bool {
	return isASCIIIdentStart(b) || (b >= '0' && b <= '9')
}

// extractSectionMetadata collects table fields, SourceTable, DataItems, and external variable refs.
func extractSectionMetadata(obj *ObjectFile, sectionName string, body string, triggers []TriggerBlock, bodyOffset int) {
	// Mask out embedded trigger code so we only scan property values outside C/AL triggers.
	cleaned := maskTriggers(body, triggers, bodyOffset)

	if sectionName == "PROPERTIES" {
		if m := sourceTableRe.FindStringSubmatch(cleaned); m != nil {
			if id, err := strconv.Atoi(m[1]); err == nil && obj.SourceTable == 0 {
				obj.SourceTable = id
			}
		}
	}

	if sectionName == "FIELDS" && obj.ObjectType == "Table" {
		obj.TableFields = parseTableFields(cleaned)
	}

	if sectionName == "DATASET" {
		parseDatasetMetadata(obj, cleaned)
	}

	if sectionName == "ELEMENTS" {
		parseElementsMetadata(obj, cleaned)
	}

	// Extract identifiers from expression-bearing properties in PROPERTIES/CONTROLS/REQUESTPAGE/FIELDS/DATASET/ELEMENTS.
	extractPropertyRefs(obj, cleaned)
}

func maskTriggers(body string, triggers []TriggerBlock, bodyOffset int) string {
	if len(triggers) == 0 {
		return body
	}
	buf := []byte(body)
	for _, tr := range triggers {
		s := tr.StartOffset - bodyOffset
		e := tr.EndOffset - bodyOffset
		if s < 0 {
			s = 0
		}
		if e > len(buf) {
			e = len(buf)
		}
		for k := s; k < e; k++ {
			if buf[k] != '\n' && buf[k] != '\r' {
				buf[k] = ' '
			}
		}
	}
	return string(buf)
}

var fieldEntryRe = regexp.MustCompile(`\{\s*(\d+)\s*;[^;]*;\s*([^;]+?)\s*;\s*([^;\}]+?)(?:\s*;|\s*\})`)

func parseTableFields(body string) []TableField {
	var fields []TableField
	matches := fieldEntryRe.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		name := strings.TrimSpace(m[2])
		dt := strings.TrimSpace(m[3])
		if name != "" {
			fields = append(fields, TableField{ID: id, Name: name, DataType: dt})
		}
	}
	return fields
}

var datasetRowRe = regexp.MustCompile(`\{\s*\d+\s*;\s*\d*\s*;\s*([^;]*?)\s*;\s*(DataItem|Column)\s*;([^\}]*)\}`)
var dataItemTableRe = regexp.MustCompile(`(?i)\bDataItemTable\s*=\s*Table(\d+)\b`)

func parseDatasetMetadata(obj *ObjectFile, body string) {
	matches := datasetRowRe.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		kind := strings.TrimSpace(m[2])
		rest := m[3]
		if strings.EqualFold(kind, "DataItem") {
			if tm := dataItemTableRe.FindStringSubmatch(rest); tm != nil {
				id, _ := strconv.Atoi(tm[1])
				if name != "" {
					obj.DataItems = append(obj.DataItems, DataItemDecl{Name: name, TableID: id})
				}
			}
		} else if strings.EqualFold(kind, "Column") {
			// The first part of rest (before next ';' or end) is the Column's SourceExpr.
			expr := rest
			if semi := strings.IndexByte(rest, ';'); semi >= 0 {
				expr = rest[:semi]
			}
			addIdentsFromExpr(obj.ExternalRefs, expr)
		}
	}
}

var xmlElementVarRe = regexp.MustCompile(`(?i)\b(?:VariableName|SourceField)\s*=\s*([^;\}]+)`)

func parseElementsMetadata(obj *ObjectFile, body string) {
	for _, m := range xmlElementVarRe.FindAllStringSubmatch(body, -1) {
		addIdentsFromExpr(obj.ExternalRefs, m[1])
	}
}

var exprPropRe = regexp.MustCompile(`(?i)\b(?:SourceExpr|Visible|Enabled|Editable|StyleExpr|CaptionClass|HideValue|ShowMandatory|QuickEntry)\s*=\s*([^;\}]+)`)

func extractPropertyRefs(obj *ObjectFile, body string) {
	for _, m := range exprPropRe.FindAllStringSubmatch(body, -1) {
		addIdentsFromExpr(obj.ExternalRefs, m[1])
	}
}

func addIdentsFromExpr(dst map[string]bool, expr string) {
	i := 0
	for i < len(expr) {
		ch := expr[i]
		// Skip single-quoted strings
		if ch == '\'' {
			i++
			for i < len(expr) {
				if expr[i] == '\'' {
					i++
					if i < len(expr) && expr[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		// Double-quoted identifier
		if ch == '"' {
			start := i + 1
			i++
			for i < len(expr) && expr[i] != '"' {
				i++
			}
			if i > start {
				ident := strings.ToLower(expr[start:i])
				dst[ident] = true
			}
			if i < len(expr) {
				i++
			}
			continue
		}
		// Unquoted identifier
		r, size := utf8.DecodeRuneInString(expr[i:])
		if unicode.IsLetter(r) || r == '_' {
			start := i
			i += size
			for i < len(expr) {
				r2, sz2 := utf8.DecodeRuneInString(expr[i:])
				if unicode.IsLetter(r2) || unicode.IsDigit(r2) || r2 == '_' {
					i += sz2
				} else {
					break
				}
			}
			dst[strings.ToLower(expr[start:i])] = true
			continue
		}
		i += size
	}
}
