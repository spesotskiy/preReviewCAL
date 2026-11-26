package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	RuleUnusedLocalVar        = "CAL001"
	RuleUnusedGlobalVar       = "CAL002"
	RuleUnusedParam           = "CAL003"
	RuleLocalTextConst        = "CAL004"
	RuleNoWith                = "CAL005"
	RuleLocalVarSuffix        = "CAL010"
	RuleParamSuffix           = "CAL011"
	RuleIdentifierCasing      = "CAL012"
	RuleCallParens            = "CAL020"
	RuleRedundantParens       = "CAL021"
	RuleIndentation           = "CAL030"
	RuleBeginPlacement        = "CAL031"
	RuleSpacing               = "CAL032"
	RuleKeywordCasing         = "CAL033"
	RuleBlankLinesAndSemicols = "CAL034"
)

var AllRuleCodes = []string{
	RuleUnusedLocalVar,
	RuleUnusedGlobalVar,
	RuleUnusedParam,
	RuleLocalTextConst,
	RuleNoWith,
	RuleLocalVarSuffix,
	RuleParamSuffix,
	RuleIdentifierCasing,
	RuleCallParens,
	RuleRedundantParens,
	RuleIndentation,
	RuleBeginPlacement,
	RuleSpacing,
	RuleKeywordCasing,
	RuleBlankLinesAndSemicols,
}

type FileNamingConfig struct {
	Template     string            `json:"template"`
	TypePrefixes map[string]string `json:"typePrefixes"`
}

type RuleConfig struct {
	Enabled    *bool  `json:"enabled,omitempty"`
	Suffix     string `json:"suffix,omitempty"`
	IndentSize int    `json:"indentSize,omitempty"`
}

type DiffConfig struct {
	// MaxOldFunctionLines is the maximum line count of an existing function/trigger BEGIN..END body
	// that will be transformed as a whole when touched in diff mode.
	// 0 means no limit (any touched old function is transformed as a whole).
	MaxOldFunctionLines int `json:"maxOldFunctionLines"`
}

type Config struct {
	Encoding   string                `json:"encoding"`
	LineEnding string                `json:"lineEnding"`
	FileNaming FileNamingConfig      `json:"fileNaming"`
	Diff       DiffConfig            `json:"diff"`
	Rules      map[string]RuleConfig `json:"rules"`
}

func boolPtr(v bool) *bool { return &v }

// Default returns the default PreReviewCAL configuration.
func Default() *Config {
	rules := make(map[string]RuleConfig, len(AllRuleCodes))
	for _, code := range AllRuleCodes {
		rc := RuleConfig{Enabled: boolPtr(true)}
		switch code {
		case RuleLocalVarSuffix:
			rc.Suffix = "L"
		case RuleParamSuffix:
			rc.Suffix = "P"
		case RuleIndentation:
			rc.IndentSize = 2
		}
		rules[code] = rc
	}
	return &Config{
		Encoding:   "utf-8-nobom",
		LineEnding: "preserve",
		FileNaming: FileNamingConfig{
			Template: "{prefix}{id} - {name}.txt",
			TypePrefixes: map[string]string{
				"Table":     "t",
				"Page":      "p",
				"Report":    "r",
				"Codeunit":  "c",
				"XMLport":   "x",
				"Query":     "q",
				"MenuSuite": "m",
				"Form":      "f",
				"Dataport":  "d",
			},
		},
		Diff: DiffConfig{
			MaxOldFunctionLines: 50,
		},
		Rules: rules,
	}
}

// AllDisabled returns a copy of Default with every rule disabled (useful for round-trip tests).
func AllDisabled() *Config {
	cfg := Default()
	for _, code := range AllRuleCodes {
		rc := cfg.Rules[code]
		rc.Enabled = boolPtr(false)
		cfg.Rules[code] = rc
	}
	return cfg
}

// Load reads a JSON config file and merges it on top of Default().
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse unmarshals JSON config bytes on top of Default().
func Parse(data []byte) (*Config, error) {
	cfg := Default()

	var raw struct {
		Encoding   string            `json:"encoding"`
		LineEnding string            `json:"lineEnding"`
		FileNaming *FileNamingConfig `json:"fileNaming"`
		Diff       *struct {
			MaxOldFunctionLines *int `json:"maxOldFunctionLines"`
		} `json:"diff"`
		Rules map[string]RuleConfig `json:"rules"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if raw.Encoding != "" {
		cfg.Encoding = raw.Encoding
	}
	if raw.LineEnding != "" {
		cfg.LineEnding = raw.LineEnding
	}
	if raw.FileNaming != nil {
		if raw.FileNaming.Template != "" {
			cfg.FileNaming.Template = raw.FileNaming.Template
		}
		for k, v := range raw.FileNaming.TypePrefixes {
			cfg.FileNaming.TypePrefixes[k] = v
		}
	}
	if raw.Diff != nil && raw.Diff.MaxOldFunctionLines != nil {
		if *raw.Diff.MaxOldFunctionLines < 0 {
			return nil, fmt.Errorf("diff.maxOldFunctionLines must be >= 0, got %d", *raw.Diff.MaxOldFunctionLines)
		}
		cfg.Diff.MaxOldFunctionLines = *raw.Diff.MaxOldFunctionLines
	}
	for code, rc := range raw.Rules {
		upper := strings.ToUpper(strings.TrimSpace(code))
		existing := cfg.Rules[upper]
		if rc.Enabled != nil {
			existing.Enabled = boolPtr(*rc.Enabled)
		}
		if rc.Suffix != "" {
			existing.Suffix = rc.Suffix
		}
		if rc.IndentSize > 0 {
			existing.IndentSize = rc.IndentSize
		}
		cfg.Rules[upper] = existing
	}

	return cfg, nil
}

// IsRuleEnabled reports whether a rule code is enabled.
func (c *Config) IsRuleEnabled(code string) bool {
	if c == nil {
		return true
	}
	rc, ok := c.Rules[strings.ToUpper(code)]
	if !ok || rc.Enabled == nil {
		return true
	}
	return *rc.Enabled
}

// SetRuleEnabled toggles a rule in-memory.
func (c *Config) SetRuleEnabled(code string, enabled bool) {
	upper := strings.ToUpper(code)
	rc := c.Rules[upper]
	rc.Enabled = boolPtr(enabled)
	c.Rules[upper] = rc
}

func (c *Config) LocalVarSuffix() string {
	if rc, ok := c.Rules[RuleLocalVarSuffix]; ok && rc.Suffix != "" {
		return rc.Suffix
	}
	return "L"
}

func (c *Config) ParamSuffix() string {
	if rc, ok := c.Rules[RuleParamSuffix]; ok && rc.Suffix != "" {
		return rc.Suffix
	}
	return "P"
}

func (c *Config) IndentSize() int {
	if rc, ok := c.Rules[RuleIndentation]; ok && rc.IndentSize > 0 {
		return rc.IndentSize
	}
	return 2
}

// PrefixForType returns the configured filename prefix for a NAV object type (case-insensitive).
func (f FileNamingConfig) PrefixForType(objectType string) (string, bool) {
	for k, v := range f.TypePrefixes {
		if strings.EqualFold(k, objectType) {
			return v, true
		}
	}
	return "", false
}

// TypeForPrefix resolves a filename prefix back to canonical NAV object type.
func (f FileNamingConfig) TypeForPrefix(prefix string) (string, bool) {
	for k, v := range f.TypePrefixes {
		if strings.EqualFold(v, prefix) {
			return k, true
		}
	}
	return "", false
}

// FormatFilename builds the expected filename for a NAV object according to FileNamingConfig.
func (f FileNamingConfig) FormatFilename(objectType string, id int, name string) string {
	tmpl := f.Template
	if tmpl == "" {
		tmpl = "{prefix}{id} - {name}.txt"
	}
	prefix, ok := f.PrefixForType(objectType)
	if !ok && len(objectType) > 0 {
		prefix = strings.ToLower(objectType[:1])
	}
	r := strings.NewReplacer(
		"{prefix}", prefix,
		"{type}", objectType,
		"{id}", strconv.Itoa(id),
		"{name}", sanitizeFilenamePart(name),
	)
	return r.Replace(tmpl)
}

// ParseFilename extracts objectType, id, and name from a filename using the configured template.
func (f FileNamingConfig) ParseFilename(filename string) (objectType string, id int, name string, ok bool) {
	base := filepath.Base(filename)
	tmpl := f.Template
	if tmpl == "" {
		tmpl = "{prefix}{id} - {name}.txt"
	}

	re, err := f.buildTemplateRegex(tmpl)
	if err != nil {
		return "", 0, "", false
	}
	m := re.FindStringSubmatch(base)
	if m == nil {
		return "", 0, "", false
	}

	var prefix, typ, idStr string
	for i, groupName := range re.SubexpNames() {
		switch groupName {
		case "prefix":
			prefix = m[i]
		case "type":
			typ = m[i]
		case "id":
			idStr = m[i]
		case "name":
			name = m[i]
		}
	}

	if typ != "" {
		objectType = canonicalObjectType(typ)
	} else if prefix != "" {
		resolved, found := f.TypeForPrefix(prefix)
		if !found {
			return "", 0, "", false
		}
		objectType = resolved
	} else {
		return "", 0, "", false
	}

	parsedID, err := strconv.Atoi(idStr)
	if err != nil {
		return "", 0, "", false
	}
	return objectType, parsedID, name, true
}

func (f FileNamingConfig) buildTemplateRegex(tmpl string) (*regexp.Regexp, error) {
	var prefixes []string
	for _, p := range f.TypePrefixes {
		if p != "" {
			prefixes = append(prefixes, regexp.QuoteMeta(p))
		}
	}
	prefixPattern := `[A-Za-z]+`
	if len(prefixes) > 0 {
		prefixPattern = `(?:` + strings.Join(prefixes, `|`) + `)`
	}

	// Tokenize template by placeholders.
	placeholders := map[string]string{
		"{prefix}": `(?P<prefix>` + prefixPattern + `)`,
		"{type}":   `(?P<type>[A-Za-z]+)`,
		"{id}":     `(?P<id>[0-9]+)`,
		"{name}":   `(?P<name>.+?)`,
	}

	var b strings.Builder
	b.WriteString(`(?i)^`)
	for i := 0; i < len(tmpl); {
		matched := false
		for ph, pat := range placeholders {
			if strings.HasPrefix(tmpl[i:], ph) {
				b.WriteString(pat)
				i += len(ph)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteString(regexp.QuoteMeta(tmpl[i : i+1]))
			i++
		}
	}
	b.WriteString(`$`)
	return regexp.Compile(b.String())
}

func sanitizeFilenamePart(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}

func canonicalObjectType(s string) string {
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
