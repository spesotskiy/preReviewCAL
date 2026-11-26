package diffscope

import (
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/objectfile"
)

// Mode describes how much of a procedure or trigger should be transformed.
type Mode int

const (
	// ModeFull transforms the entire procedure/trigger with all enabled rules.
	ModeFull Mode = iota
	// ModeSkip leaves an untouched existing procedure/trigger completely unmodified.
	ModeSkip
	// ModePartial transforms only newly added / modified parts of a large existing procedure/trigger.
	ModePartial
)

// UnitScope holds diff-scoping metadata for a single procedure or embedded trigger.
type UnitScope struct {
	Mode         Mode
	OldBodyRaw   string
	OldBodyLines int
	OldLocalVars map[string]bool
}

// IsNewLocalVar reports whether varName was newly added compared to the base version.
// In ModeFull every local var is considered eligible; in ModeSkip none are.
func (u *UnitScope) IsNewLocalVar(varName string) bool {
	if u == nil || u.Mode == ModeFull {
		return true
	}
	if u.Mode == ModeSkip {
		return false
	}
	return !u.OldLocalVars[strings.ToLower(varName)]
}

// ChangedBodyLines computes 1-based line numbers inside currBody that were added or modified
// compared to OldBodyRaw.
func (u *UnitScope) ChangedBodyLines(currBody string) map[int]bool {
	if u == nil || u.Mode != ModePartial {
		return nil
	}
	// Align leading horizontal spaces on line 0 because OldBodyRaw begins at "BEGIN"
	// while formatting passes may include line 0's leading indentation.
	normCurr := strings.TrimLeft(currBody, " \t")
	normOld := strings.TrimLeft(u.OldBodyRaw, " \t")
	return ChangedLineNumbers(normOld, normCurr)
}

// FileScope describes diff-mode scoping for an entire NAV object file.
type FileScope struct {
	Enabled       bool
	procScopes    map[string]*UnitScope
	triggerScopes map[string]*UnitScope
}

// FullFileScope returns a FileScope with diff restriction disabled (all procedures/triggers ModeFull).
func FullFileScope() *FileScope {
	return &FileScope{Enabled: false}
}

// ProcScope returns the UnitScope for proc.
func (fs *FileScope) ProcScope(proc *cal.ProcDecl) *UnitScope {
	if fs == nil || !fs.Enabled || proc == nil {
		return &UnitScope{Mode: ModeFull}
	}
	if s, ok := fs.procScopes[procKey(proc)]; ok {
		return s
	}
	return &UnitScope{Mode: ModeFull}
}

// TriggerScope returns the UnitScope for tr.
func (fs *FileScope) TriggerScope(tr objectfile.TriggerBlock) *UnitScope {
	if fs == nil || !fs.Enabled {
		return &UnitScope{Mode: ModeFull}
	}
	if s, ok := fs.triggerScopes[triggerKey(tr)]; ok {
		return s
	}
	return &UnitScope{Mode: ModeFull}
}

// Build compares baseBytes and currBytes and classifies every procedure and embedded trigger
// into ModeSkip, ModeFull, or ModePartial according to maxOldFuncLines.
// If len(baseBytes) == 0 (brand-new file), all units are ModeFull.
func Build(baseBytes, currBytes []byte, maxOldFuncLines int) (*FileScope, error) {
	if len(baseBytes) == 0 {
		return FullFileScope(), nil
	}

	baseObj, err := objectfile.Parse(baseBytes)
	if err != nil {
		// If base file cannot be parsed (e.g. non-NAV or empty stub), treat current file as new.
		return FullFileScope(), nil
	}
	currObj, err := objectfile.Parse(currBytes)
	if err != nil {
		return nil, err
	}

	fs := &FileScope{
		Enabled:       true,
		procScopes:    make(map[string]*UnitScope),
		triggerScopes: make(map[string]*UnitScope),
	}

	// 1. Match embedded triggers by OwnerKey (fallback to occurrence key).
	baseTriggers := make(map[string]objectfile.TriggerBlock, len(baseObj.Triggers))
	for _, tr := range baseObj.Triggers {
		baseTriggers[triggerKey(tr)] = tr
	}
	for _, currTr := range currObj.Triggers {
		key := triggerKey(currTr)
		baseTr, existed := baseTriggers[key]
		if !existed {
			fs.triggerScopes[key] = &UnitScope{Mode: ModeFull}
			continue
		}
		if normalizeEOL(baseTr.Raw) == normalizeEOL(currTr.Raw) {
			fs.triggerScopes[key] = &UnitScope{Mode: ModeSkip}
			continue
		}
		baseTD := cal.ParseTrigger(baseTr.Raw)
		oldBody := ""
		if baseTD.BodyStart >= 0 && baseTD.BodyEnd > baseTD.BodyStart {
			oldBody = baseTr.Raw[baseTD.BodyStart:baseTD.BodyEnd]
		}
		oldBodyLines := countLines(oldBody)
		if maxOldFuncLines == 0 || oldBodyLines <= maxOldFuncLines {
			fs.triggerScopes[key] = &UnitScope{
				Mode:         ModeFull,
				OldBodyRaw:   oldBody,
				OldBodyLines: oldBodyLines,
			}
		} else {
			fs.triggerScopes[key] = &UnitScope{
				Mode:         ModePartial,
				OldBodyRaw:   oldBody,
				OldBodyLines: oldBodyLines,
				OldLocalVars: collectLocalVarNames(baseTD.LocalVars),
			}
		}
	}

	// 2. Match CODE procedures by lowercased name (or @ID fallback).
	if currObj.Code != nil {
		currCS := cal.ParseCodeSection(currObj.Code.Raw)
		var baseCS *cal.CodeSection
		baseByName := make(map[string]*cal.ProcDecl)
		baseByID := make(map[string]*cal.ProcDecl)
		if baseObj.Code != nil {
			baseCS = cal.ParseCodeSection(baseObj.Code.Raw)
			for _, bp := range baseCS.Procedures {
				baseByName[strings.ToLower(bp.Name)] = bp
				if bp.ID != "" {
					baseByID[bp.ID] = bp
				}
			}
		}

		for _, cp := range currCS.Procedures {
			key := procKey(cp)
			bp := baseByName[strings.ToLower(cp.Name)]
			if bp == nil && cp.ID != "" {
				bp = baseByID[cp.ID]
			}
			if bp == nil || baseObj.Code == nil {
				fs.procScopes[key] = &UnitScope{Mode: ModeFull}
				continue
			}

			baseProcRaw := baseObj.Code.Raw[bp.StartOffset:bp.EndOffset]
			currProcRaw := currObj.Code.Raw[cp.StartOffset:cp.EndOffset]
			if normalizeEOL(baseProcRaw) == normalizeEOL(currProcRaw) {
				fs.procScopes[key] = &UnitScope{Mode: ModeSkip}
				continue
			}

			oldBody := ""
			if bp.BodyStart >= 0 && bp.BodyEnd > bp.BodyStart {
				oldBody = baseObj.Code.Raw[bp.BodyStart:bp.BodyEnd]
			}
			oldBodyLines := countLines(oldBody)
			if maxOldFuncLines == 0 || oldBodyLines <= maxOldFuncLines {
				fs.procScopes[key] = &UnitScope{
					Mode:         ModeFull,
					OldBodyRaw:   oldBody,
					OldBodyLines: oldBodyLines,
				}
			} else {
				fs.procScopes[key] = &UnitScope{
					Mode:         ModePartial,
					OldBodyRaw:   oldBody,
					OldBodyLines: oldBodyLines,
					OldLocalVars: collectLocalVarNames(bp.LocalVars),
				}
			}
		}
	}

	return fs, nil
}

// ChangedLineNumbers computes the 1-based line numbers in curr that are not matched in the LCS with base.
func ChangedLineNumbers(base, curr string) map[int]bool {
	a := splitNormalizedLines(base)
	b := splitNormalizedLines(curr)

	changed := make(map[int]bool)

	// Trim common prefix and suffix for speed and deterministic anchoring.
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	aEnd := len(a)
	bEnd := len(b)
	for aEnd > prefix && bEnd > prefix && a[aEnd-1] == b[bEnd-1] {
		aEnd--
		bEnd--
	}

	aMid := a[prefix:aEnd]
	bMid := b[prefix:bEnd]
	if len(bMid) == 0 {
		return changed
	}
	if len(aMid) == 0 {
		for idx := prefix; idx < bEnd; idx++ {
			changed[idx+1] = true
		}
		return changed
	}

	// LCS DP on the differing middle slices.
	m, n := len(aMid), len(bMid)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if aMid[i] == bMid[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	i, j := 0, 0
	for i < m && j < n {
		if aMid[i] == bMid[j] {
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			changed[prefix+j+1] = true
			j++
		}
	}
	for ; j < n; j++ {
		changed[prefix+j+1] = true
	}

	return changed
}

func procKey(p *cal.ProcDecl) string {
	return strings.ToLower(p.Name) + "#" + p.ID
}

func triggerKey(tr objectfile.TriggerBlock) string {
	if tr.OwnerKey != "" {
		return strings.ToLower(tr.OwnerKey)
	}
	return strings.ToLower(tr.SectionName + ":" + tr.Name)
}

func collectLocalVarNames(vs *cal.VarSection) map[string]bool {
	m := make(map[string]bool)
	if vs == nil {
		return m
	}
	for _, vd := range vs.Decls {
		m[strings.ToLower(vd.Name)] = true
	}
	return m
}

func normalizeEOL(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func splitNormalizedLines(s string) []string {
	norm := normalizeEOL(s)
	if norm == "" {
		return nil
	}
	return strings.Split(norm, "\n")
}

func countLines(s string) int {
	norm := strings.TrimRight(normalizeEOL(s), "\n")
	if norm == "" {
		return 0
	}
	return 1 + strings.Count(norm, "\n")
}
