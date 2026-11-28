package objectfile

import (
	"os"
	"path/filepath"
	"strings"

	"prereviewcal/internal/cal"
	"prereviewcal/internal/config"
)

// TableSchema holds field and public procedure metadata for a NAV object
// (used for strict WITH resolution in CAL005, task 1.3).
type TableSchema struct {
	TableID    int // object ID of the table/codeunit/page/... the schema belongs to
	ObjectType string
	TableName  string
	Fields     map[string]string // lowercased field name -> canonical field name
	Procs      map[string]string // lowercased public procedure name -> canonical name
}

// HasField reports whether the table declares a field with the given name (case-insensitive).
func (ts *TableSchema) HasField(name string) bool {
	if ts == nil {
		return false
	}
	_, ok := ts.Fields[strings.ToLower(name)]
	return ok
}

// HasProc reports whether the object declares a public procedure with the given name.
func (ts *TableSchema) HasProc(name string) bool {
	if ts == nil {
		return false
	}
	_, ok := ts.Procs[strings.ToLower(name)]
	return ok
}

type schemaKey struct {
	objectType string
	id         int
}

// WorkspaceIndex lazily indexes NAV object files (tables and other objects) in a
// directory using FileNamingConfig: tables provide field schemas, all objects
// provide their public procedure lists (task 1.3).
type WorkspaceIndex struct {
	naming  config.FileNamingConfig
	byKey   map[schemaKey]*TableSchema
	byPath  map[schemaKey]string
	loaded  map[schemaKey]bool
}

// NewWorkspaceIndex scans dir recursively for object .txt files matching the
// configured FileNamingConfig.
func NewWorkspaceIndex(dir string, naming config.FileNamingConfig) *WorkspaceIndex {
	idx := &WorkspaceIndex{
		naming: naming,
		byKey:  make(map[schemaKey]*TableSchema),
		byPath: make(map[schemaKey]string),
		loaded: make(map[schemaKey]bool),
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
		if ok && objType != "" {
			idx.byPath[schemaKey{indexType(objType), id}] = path
		}
		return nil
	})
	return idx
}

func indexType(objectType string) string {
	switch strings.ToLower(objectType) {
	case "record":
		return "Table"
	case "port":
		return "XMLport"
	}
	return canonicalType(objectType)
}

// SchemaFromObject builds a TableSchema directly from a parsed ObjectFile
// (fields + public, non-local procedures). Used for the object being processed itself.
func SchemaFromObject(obj *ObjectFile) *TableSchema {
	if obj == nil {
		return nil
	}
	fields := make(map[string]string, len(obj.TableFields))
	for _, f := range obj.TableFields {
		fields[strings.ToLower(f.Name)] = f.Name
	}
	procs := make(map[string]string)
	if obj.Code != nil {
		cs := cal.ParseCodeSection(obj.Code.Raw)
		for _, p := range cs.Procedures {
			if p.IsLocal || p.IsEvent || p.Name == "" {
				continue
			}
			procs[strings.ToLower(p.Name)] = p.Name
		}
	}
	return &TableSchema{
		TableID:    obj.ObjectID,
		ObjectType: obj.ObjectType,
		TableName:  obj.ObjectName,
		Fields:     fields,
		Procs:      procs,
	}
}

// RegisterObject registers a parsed ObjectFile directly into the index
// (any object type: fields for tables, public procedures for everything).
func (idx *WorkspaceIndex) RegisterObject(obj *ObjectFile) {
	if idx == nil || obj == nil {
		return
	}
	key := schemaKey{indexType(obj.ObjectType), obj.ObjectID}
	idx.byKey[key] = SchemaFromObject(obj)
	idx.loaded[key] = true
}

// RegisterTable registers a parsed Table ObjectFile directly into the index.
func (idx *WorkspaceIndex) RegisterTable(obj *ObjectFile) {
	if obj == nil || !strings.EqualFold(obj.ObjectType, "Table") {
		return
	}
	idx.RegisterObject(obj)
}

// LookupTable returns the TableSchema for tableID, parsing its file lazily if indexed.
func (idx *WorkspaceIndex) LookupTable(tableID int) *TableSchema {
	return idx.LookupObject("Table", tableID)
}

// LookupObject returns the schema for an object type + ID (Record subtype ID maps
// to "Table"), parsing its file lazily if indexed. Base type names such as
// "Record", "Codeunit", "Page" are accepted case-insensitively.
func (idx *WorkspaceIndex) LookupObject(objectType string, id int) *TableSchema {
	if idx == nil || id <= 0 || objectType == "" {
		return nil
	}
	key := schemaKey{indexType(objectType), id}
	if idx.loaded[key] {
		return idx.byKey[key]
	}
	idx.loaded[key] = true
	path, ok := idx.byPath[key]
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	obj, err := Parse(data)
	if err != nil {
		return nil
	}
	idx.RegisterObject(obj)
	return idx.byKey[key]
}

