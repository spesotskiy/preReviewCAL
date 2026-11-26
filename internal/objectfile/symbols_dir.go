package objectfile

import (
	"os"
	"path/filepath"
	"strings"

	"prereviewcal/internal/config"
)

// TableSchema holds field metadata for a NAV Table object (used for WITH resolution).
type TableSchema struct {
	TableID   int
	TableName string
	Fields    map[string]string // lowercased field name -> canonical field name
}

// HasField reports whether the table declares a field with the given name (case-insensitive).
func (ts *TableSchema) HasField(name string) bool {
	if ts == nil {
		return false
	}
	_, ok := ts.Fields[strings.ToLower(name)]
	return ok
}

// WorkspaceIndex lazily indexes NAV table object files in a directory using FileNamingConfig.
type WorkspaceIndex struct {
	naming  config.FileNamingConfig
	byID    map[int]*TableSchema
	byPath  map[int]string
	loaded  map[int]bool
}

// NewWorkspaceIndex scans dir (non-recursively and recursively up to subdirs) for Table files
// matching the configured FileNamingConfig, and also allows explicit registration.
func NewWorkspaceIndex(dir string, naming config.FileNamingConfig) *WorkspaceIndex {
	idx := &WorkspaceIndex{
		naming: naming,
		byID:   make(map[int]*TableSchema),
		byPath: make(map[int]string),
		loaded: make(map[int]bool),
	}
	if dir == "" {
		return idx
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".txt") {
			return nil
		}
		objType, id, _, ok := naming.ParseFilename(d.Name())
		if ok && strings.EqualFold(objType, "Table") {
			idx.byPath[id] = path
		}
		return nil
	})
	return idx
}

// RegisterTable registers a parsed Table ObjectFile directly into the index.
func (idx *WorkspaceIndex) RegisterTable(obj *ObjectFile) {
	if idx == nil || obj == nil || obj.ObjectType != "Table" {
		return
	}
	fields := make(map[string]string, len(obj.TableFields))
	for _, f := range obj.TableFields {
		fields[strings.ToLower(f.Name)] = f.Name
	}
	idx.byID[obj.ObjectID] = &TableSchema{
		TableID:   obj.ObjectID,
		TableName: obj.ObjectName,
		Fields:    fields,
	}
	idx.loaded[obj.ObjectID] = true
}

// LookupTable returns the TableSchema for tableID, parsing its file lazily if indexed.
func (idx *WorkspaceIndex) LookupTable(tableID int) *TableSchema {
	if idx == nil || tableID <= 0 {
		return nil
	}
	if idx.loaded[tableID] {
		return idx.byID[tableID]
	}
	idx.loaded[tableID] = true
	path, ok := idx.byPath[tableID]
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	obj, err := Parse(data)
	if err != nil || obj.ObjectType != "Table" {
		return nil
	}
	idx.RegisterTable(obj)
	return idx.byID[tableID]
}
