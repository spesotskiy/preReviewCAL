package rules

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
	"prereviewcal/internal/diffscope"
	"prereviewcal/internal/objectfile"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Diagnostic reports a finding or automatic fix applied by a rule.
type Diagnostic struct {
	Rule     string
	Severity Severity
	Line     int
	Message  string
	Fixed    bool
}

func (d Diagnostic) String() string {
	status := "warn"
	if d.Fixed {
		status = "fixed"
	} else if d.Severity == SeverityError {
		status = "error"
	}
	return fmt.Sprintf("line %d: [%s] (%s) %s", d.Line, d.Rule, status, d.Message)
}

// Result holds the transformed file bytes and all diagnostics produced.
type Result struct {
	Output      []byte
	Changed     bool
	Diagnostics []Diagnostic
}

// HasWarnings reports whether any unfixed warning/error diagnostics exist.
func (r *Result) HasWarnings() bool {
	for _, d := range r.Diagnostics {
		if !d.Fixed && (d.Severity == SeverityWarning || d.Severity == SeverityError) {
			return true
		}
	}
	return false
}

// Process runs all enabled rules on a NAV 2017 .txt object file (full-file mode).
func Process(input []byte, cfg *config.Config, ws *objectfile.WorkspaceIndex) (*Result, error) {
	return ProcessWithScope(input, cfg, ws, nil)
}

// ProcessDiff runs enabled rules scoped only to changes between base and input,
// respecting cfg.Diff.MaxOldFunctionLines.
func ProcessDiff(base, input []byte, cfg *config.Config, ws *objectfile.WorkspaceIndex) (*Result, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	fscope, err := diffscope.Build(base, input, cfg.Diff.MaxOldFunctionLines)
	if err != nil {
		return nil, err
	}
	return ProcessWithScope(input, cfg, ws, fscope)
}

// ProcessWithScope runs enabled rules on input constrained by fscope (nil = full-file mode).
func ProcessWithScope(input []byte, cfg *config.Config, ws *objectfile.WorkspaceIndex, fscope *diffscope.FileScope) (*Result, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	if fscope == nil {
		fscope = diffscope.FullFileScope()
	}

	current := input
	var allDiags []Diagnostic

	// Helper to run a pass that receives parsed ObjectFile and returns edits + diagnostics.
	runPass := func(fn func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic)) error {
		obj, err := objectfile.Parse(current)
		if err != nil {
			return err
		}
		if ws != nil && obj.ObjectType == "Table" {
			ws.RegisterTable(obj)
		}
		edits, diags := fn(obj)
		allDiags = append(allDiags, diags...)
		if len(edits) > 0 {
			current = obj.ApplyEdits(edits)
		}
		return nil
	}

	// Pass 1: CAL005 (no-with) - eliminate WITH ... DO before variable usage & naming analysis.
	if cfg.IsRuleEnabled(config.RuleNoWith) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runNoWithRule(obj, cfg, ws, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 2: CAL001 (unused-local-var) - clean unused locals first so unused local TextConst is dropped, not promoted.
	if cfg.IsRuleEnabled(config.RuleUnusedLocalVar) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runUnusedLocalVarRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 3: CAL004 (local-textconst) - promote remaining (used) local TextConst declarations to global VAR.
	if cfg.IsRuleEnabled(config.RuleLocalTextConst) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runLocalTextConstRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 4: CAL002 (unused-global-var) & CAL003 (unused-param).
	if cfg.IsRuleEnabled(config.RuleUnusedGlobalVar) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runUnusedGlobalVarRule(obj, cfg)
		}); err != nil {
			return nil, err
		}
	}
	if cfg.IsRuleEnabled(config.RuleUnusedParam) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runUnusedParamRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 5: CAL010 (local-var-suffix) & CAL011 (param-suffix).
	if cfg.IsRuleEnabled(config.RuleLocalVarSuffix) || cfg.IsRuleEnabled(config.RuleParamSuffix) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runSuffixRules(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 6: CAL012 (identifier-casing).
	if cfg.IsRuleEnabled(config.RuleIdentifierCasing) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runIdentifierCasingRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 7: CAL020 (call-parens) & CAL021 (redundant-parens).
	if cfg.IsRuleEnabled(config.RuleCallParens) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runCallParensRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}
	if cfg.IsRuleEnabled(config.RuleRedundantParens) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runRedundantParensRule(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Pass 8: CAL033 (keyword-casing), CAL034 (blank-lines-and-semicolons),
	//         CAL032 (spacing), CAL031 (begin-placement), CAL030 (indentation).
	if cfg.IsRuleEnabled(config.RuleKeywordCasing) ||
		cfg.IsRuleEnabled(config.RuleBlankLinesAndSemicols) ||
		cfg.IsRuleEnabled(config.RuleSpacing) ||
		cfg.IsRuleEnabled(config.RuleBeginPlacement) ||
		cfg.IsRuleEnabled(config.RuleIndentation) {
		if err := runPass(func(obj *objectfile.ObjectFile) ([]objectfile.SpanEdit, []Diagnostic) {
			return runFormatRules(obj, cfg, fscope)
		}); err != nil {
			return nil, err
		}
	}

	// Ensure UTF-8 without BOM and optional line ending preference.
	out := current
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		out = out[3:]
	}
	switch strings.ToLower(cfg.LineEnding) {
	case "crlf":
		out = []byte(strings.ReplaceAll(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n", "\r\n"))
	case "lf":
		out = []byte(strings.ReplaceAll(string(out), "\r\n", "\n"))
	}

	return &Result{
		Output:      out,
		Changed:     !bytes.Equal(input, out),
		Diagnostics: allDiags,
	}, nil
}

// textEdit is a local byte-range edit inside a block string.
type textEdit struct {
	start   int
	end     int
	newText string
}

func applyTextEdits(src string, edits []textEdit) string {
	if len(edits) == 0 {
		return src
	}
	sorted := make([]textEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end < sorted[j].end
	})

	var b strings.Builder
	b.Grow(len(src))
	cursor := 0
	for _, ed := range sorted {
		if ed.start < cursor {
			continue
		}
		b.WriteString(src[cursor:ed.start])
		b.WriteString(ed.newText)
		cursor = ed.end
	}
	b.WriteString(src[cursor:])
	return b.String()
}

// collectUnqualifiedRefs returns lowercased identifier names referenced without a leading '.' or '::'.
func collectUnqualifiedRefs(body string) map[string]bool {
	refs := make(map[string]bool)
	tokens := cal.Lex(body)
	for i, tok := range tokens {
		if tok.Kind != cal.TokIdent && tok.Kind != cal.TokQuotedIdent {
			continue
		}
		prev := cal.PrevCode(tokens, i-1)
		if prev != -1 && (tokens[prev].Kind == cal.TokDot || tokens[prev].Kind == cal.TokScope) {
			continue
		}
		if tok.Kind == cal.TokIdent && cal.IsStatementKeyword(tok.Text) {
			continue
		}
		refs[strings.ToLower(tok.IdentName())] = true
	}
	return refs
}

// isMemberAccess reports whether token at idx is preceded by '.' or '::'.
func isMemberAccess(tokens []cal.Token, idx int) bool {
	prev := cal.PrevCode(tokens, idx-1)
	return prev != -1 && (tokens[prev].Kind == cal.TokDot || tokens[prev].Kind == cal.TokScope)
}

// objectSymbols caches globals, procedures, dataitems, and table fields for an ObjectFile.
type objectSymbols struct {
	globals    map[string]*cal.VarDecl
	procs      map[string]*cal.ProcDecl
	dataItems  map[string]int // lower -> tableID
	ownFields  map[string]string
	hasOwnRec  bool
}

func buildObjectSymbols(obj *objectfile.ObjectFile, cs *cal.CodeSection) *objectSymbols {
	osym := &objectSymbols{
		globals:   make(map[string]*cal.VarDecl),
		procs:     make(map[string]*cal.ProcDecl),
		dataItems: make(map[string]int),
		ownFields: make(map[string]string),
		hasOwnRec: obj.ObjectType == "Table" || obj.ObjectType == "Page" || obj.SourceTable > 0,
	}
	if cs != nil {
		if cs.GlobalVars != nil {
			for _, v := range cs.GlobalVars.Decls {
				osym.globals[strings.ToLower(v.Name)] = v
			}
		}
		for _, p := range cs.Procedures {
			if !p.IsEvent {
				osym.procs[strings.ToLower(p.Name)] = p
			}
		}
	}
	for _, di := range obj.DataItems {
		osym.dataItems[strings.ToLower(di.Name)] = di.TableID
	}
	for _, f := range obj.TableFields {
		osym.ownFields[strings.ToLower(f.Name)] = f.Name
	}
	return osym
}
