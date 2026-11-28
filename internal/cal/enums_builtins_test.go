package cal

import "testing"

// Task 1.3: NAV 2017 system enum/Option type names must be known builtins so that
// CAL005 never prefixes them before '::' (ACTION::LookupOK must survive).
func TestSystemEnumTypesAreGlobalBuiltins(t *testing.T) {
	for _, name := range []string{
		"ACTION", "Datatype", "EXECUTIONMODE", "REPORTFORMAT",
		"securityfilter", "TEXTENCODING", "TransactionType", "OBJECTTYPE",
		"TESTPERMISSIONS", "CLIENTTYPE", "DATEORDER", "FORM",
	} {
		if !IsGlobalBuiltin(name) {
			t.Errorf("IsGlobalBuiltin(%q) = false, want true", name)
		}
	}
}
