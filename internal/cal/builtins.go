package cal

import "strings"

// StatementKeywords are C/AL keywords that CAL033 normalizes to UPPERCASE in code.
var StatementKeywords = map[string]bool{
	"AND":    true,
	"BEGIN":  true,
	"CASE":   true,
	"DIV":    true,
	"DO":     true,
	"DOWNTO": true,
	"ELSE":   true,
	"END":    true,
	"EXIT":   true,
	"FALSE":  true,
	"FOR":    true,
	"IF":     true,
	"IN":     true,
	"MOD":    true,
	"NOT":    true,
	"OF":     true,
	"OR":     true,
	"REPEAT": true,
	"THEN":   true,
	"TO":     true,
	"TRUE":   true,
	"UNTIL":  true,
	"WHILE":  true,
	"WITH":   true,
	"XOR":    true,
}

// DeclKeywords are C/AL declaration keywords.
var DeclKeywords = map[string]bool{
	"ARRAY":             true,
	"EVENT":             true,
	"INDATASET":         true,
	"LOCAL":             true,
	"OF":                true,
	"PROCEDURE":         true,
	"RUNONCLIENT":       true,
	"SECURITYFILTERING": true,
	"TEMPORARY":         true,
	"VAR":               true,
	"WITHEVENTS":        true,
}

// ZeroArgGlobalBuiltins are global built-in functions in NAV 2017 that take 0 arguments
// and should be written with () under CAL020 (including TODAY() and COMPANYNAME()).
// EXIT is intentionally excluded per user specification.
var ZeroArgGlobalBuiltins = map[string]bool{
	"APPLICATIONPATH":        true,
	"BEEP":                   true,
	"CLEARLASTERROR":         true,
	"COMMIT":                 true,
	"COMPANYNAME":            true,
	"CREATEGUID":             true,
	"CURRENTCLIENTTYPE":      true,
	"CURRENTDATETIME":        true,
	"CURRENTTRANSACTIONTYPE": true,
	"DEFAULTCLIENTTYPE":      true,
	"GETLASTERRORCALLSTACK":  true,
	"GETLASTERRORCODE":       true,
	"GETLASTERRORTEXT":       true,
	"GUIALLOWED":             true,
	"OBJECTID":               true,
	"SELECTLATESTVERSION":    true,
	"SERIALNUMBER":           true,
	"SESSIONID":              true,
	"TEMPORARYPATH":          true,
	"TIME":                   true,
	"TODAY":                  true,
	"USERID":                 true,
	"WORKDATE":               true,
	"YIELD":                  true,
}

// RecordZeroArgMethods are unambiguous NAV 2017 Record methods that take 0 arguments
// and should be written with () under CAL020.
var RecordZeroArgMethods = map[string]bool{
	"CLEARMARKS":      true,
	"COUNT":           true,
	"COUNTAPPROX":     true,
	"CURRENTKEY":      true,
	"DELETE":          true,
	"DELETEALL":       true,
	"DELETELINKS":     true,
	"FIND":            true,
	"FINDFIRST":       true,
	"FINDLAST":        true,
	"FINDSET":         true,
	"GETFILTERS":      true,
	"GETPOSITION":     true,
	"GETVIEW":         true,
	"HASFILTER":       true,
	"HASLINKS":        true,
	"INIT":            true,
	"INSERT":          true,
	"ISEMPTY":         true,
	"ISTEMPORARY":     true,
	"LOCKTABLE":       true,
	"MARKEDONLY":      true,
	"MODIFY":          true,
	"NEXT":            true,
	"READPERMISSION":  true,
	"RECORDID":        true,
	"RESET":           true,
	"SETRECFILTER":    true,
	"TABLECAPTION":    true,
	"TABLENAME":       true,
	"WRITEPERMISSION": true,
}

// ObjectZeroArgMethods are zero-arg methods on Page/Report/XMLport/Query/File/Dialog/InStream/OutStream/Codeunit.
var ObjectZeroArgMethods = map[string]bool{
	"ACTIVATE":       true,
	"BREAK":          true,
	"CLOSE":          true,
	"EOS":            true,
	"EXPORT":         true,
	"IMPORT":         true,
	"NEWPAGE":        true,
	"QUIT":           true,
	"RUN":            true,
	"RUNMODAL":       true,
	"SKIP":           true,
	"UPDATE":         true,
	"UPDATECONTROLS": true,
}

// AllRecordMethods lists NAV 2017 Record methods (0-arg and N-arg) for CAL005 (WITH replacement).
var AllRecordMethods = map[string]bool{
	"ADDLINK":             true,
	"ASCENDING":           true,
	"CALCFIELDS":          true,
	"CALCSUMS":            true,
	"CHANGECOMPANY":       true,
	"CLEARMARKS":          true,
	"COPY":                true,
	"COPYFILTER":          true,
	"COPYFILTERS":         true,
	"COPYLINKS":           true,
	"COUNT":               true,
	"COUNTAPPROX":         true,
	"CURRENTKEY":          true,
	"DELETE":              true,
	"DELETEALL":           true,
	"DELETELINK":          true,
	"DELETELINKS":         true,
	"FIELDACTIVE":         true,
	"FIELDCAPTION":        true,
	"FIELDERROR":          true,
	"FIELDNAME":           true,
	"FILTERGROUP":         true,
	"FIND":                true,
	"FINDFIRST":           true,
	"FINDLAST":            true,
	"FINDSET":             true,
	"GET":                 true,
	"GETFILTER":           true,
	"GETFILTERS":          true,
	"GETPOSITION":         true,
	"GETRANGEMAX":         true,
	"GETRANGEMIN":         true,
	"GETRECORD":           true,
	"GETVIEW":             true,
	"HASFILTER":           true,
	"HASLINKS":            true,
	"INIT":                true,
	"INSERT":              true,
	"ISEMPTY":             true,
	"ISTEMPORARY":         true,
	"LOCKTABLE":           true,
	"MARK":                true,
	"MARKEDONLY":          true,
	"MODIFY":              true,
	"MODIFYALL":           true,
	"NEXT":                true,
	"READCONSISTENCY":     true,
	"READPERMISSION":      true,
	"RECORDID":            true,
	"RENAME":              true,
	"RESET":               true,
	"SECURITYFILTERING":   true,
	"SETASCENDING":        true,
	"SETAUTOCALCFIELDS":   true,
	"SETCURRENTKEY":       true,
	"SETFILTER":           true,
	"SETPERMISSIONFILTER": true,
	"SETPOSITION":         true,
	"SETRANGE":            true,
	"SETRECFILTER":        true,
	"SETVIEW":             true,
	"TABLECAPTION":        true,
	"TABLENAME":           true,
	"TESTFIELD":           true,
	"TRANSFERFIELDS":      true,
	"VALIDATE":            true,
	"WRITEPERMISSION":     true,
}

// GlobalBuiltins are all global C/AL functions, objects, and built-in variables
// that must NOT be prefixed when eliminating WITH ... DO (CAL005).
var GlobalBuiltins = map[string]bool{
	"ABS": true, "APPLICATIONPATH": true, "ARRAYLEN": true, "BEEP": true,
	"CALCDATE": true, "CLEAR": true, "CLEARALL": true, "CLEARLASTERROR": true,
	"CLIENTTYPE": true, "CLOSINGDATE": true, "CODEUNIT": true, "COMMIT": true,
	"COMPANYNAME": true, "COMPRESSARRAY": true, "CONFIRM": true, "CONVERTSTR": true,
	"COPYARRAY": true, "COPYSTR": true, "CREATEDATETIME": true, "CREATEGUID": true,
	"CURRENTCLIENTTYPE": true, "CURRENTDATETIME": true, "CURRENTTRANSACTIONTYPE": true,
	"CURRPAGE": true, "CURRREPORT": true, "CURRXMLPORT": true,
	"DATABASE": true, "DATE2DMY": true, "DATE2DWY": true, "DEFAULTCLIENTTYPE": true,
	"DELCHR": true, "DELSTR": true, "DIALOG": true, "DMY2DATE": true,
	"DOWNLOAD": true, "DOWNLOADFROMSTREAM": true, "DT2DATE": true, "DT2TIME": true,
	"DWY2DATE": true, "ENVIRON": true, "ERASE": true, "ERROR": true,
	"EVALUATE": true, "EXISTS": true, "FILE": true, "FORMAT": true,
	"GETLASTERRORCALLSTACK": true, "GETLASTERRORCODE": true, "GETLASTERROROBJECT": true,
	"GETLASTERRORTEXT": true, "GETURL": true, "GUIALLOWED": true, "HYPERLINK": true,
	"INCSTR": true, "INSSTR": true, "ISCLEAR": true, "ISNULLGUID": true,
	"LOWERCASE": true, "MAXSTRLEN": true, "MESSAGE": true, "NORMALDATE": true,
	"OBJECTID": true, "OBJECTTYPE": true, "PADSTR": true, "PAGE": true,
	"POWER": true, "QUERY": true, "RANDOM": true, "RANDOMIZE": true,
	"REC": true, "REPORT": true, "ROUND": true, "SELECTLATESTVERSION": true,
	"SELECTSTR": true, "SERIALNUMBER": true, "SESSIONID": true, "SHELL": true,
	"SLEEP": true, "STARTSESSION": true, "STOPSESSION": true, "STRCHECKSUM": true,
	"STRLEN": true, "STRMENU": true, "STRPOS": true, "STRSUBSTNO": true,
	"SYSTEM": true, "TASKSCHEDULER": true, "TEMPORARYPATH": true, "TIME": true,
	"TODAY": true, "UPLOAD": true, "UPLOADINTOSTREAM": true, "UPPERCASE": true,
	"USERID": true, "WORKDATE": true, "XMLPORT": true, "XREC": true, "YIELD": true,
	// NAV 2017 system Option/enum type names: never prefixed before '::' (task 1.3).
	"ACTION": true, "DATATYPE": true, "DATEORDER": true, "EXECUTIONMODE": true,
	"FORM": true, "REPORTFORMAT": true, "SECURITYFILTER": true,
	"TESTPERMISSIONS": true, "TEXTENCODING": true, "TRANSACTIONTYPE": true,
}

// SystemTriggerNames lists platform/system procedure/trigger names whose parameters must not be renamed by CAL011.
var SystemTriggerNames = map[string]bool{
	"ONRUN":                 true,
	"ONINSERT":              true,
	"ONMODIFY":              true,
	"ONDELETE":              true,
	"ONRENAME":              true,
	"ONVALIDATE":            true,
	"ONLOOKUP":              true,
	"ONASSISTEDIT":          true,
	"ONDRILLDOWN":           true,
	"ONACTION":              true,
	"ONINIT":                true,
	"ONOPENPAGE":            true,
	"ONCLOSEPAGE":           true,
	"ONFINDRECORD":          true,
	"ONNEXTRECORD":          true,
	"ONAFTERGETRECORD":      true,
	"ONAFTERGETCURRRECORD":  true,
	"ONNEWRECORD":           true,
	"ONINSERTRECORD":        true,
	"ONMODIFYRECORD":        true,
	"ONDELETERECORD":        true,
	"ONQUERYCLOSEPAGE":      true,
	"ONINITREPORT":          true,
	"ONPREREPORT":           true,
	"ONPOSTREPORT":          true,
	"ONPREDATAITEM":         true,
	"ONPOSTDATAITEM":        true,
	"ONINITXMLPORT":         true,
	"ONPREXMLPORT":          true,
	"ONPOSTXMLPORT":         true,
	"ONAFTERINITRECORD":     true,
	"ONBEFOREINSERTRECORD":  true,
	"ONAFTERINSERTRECORD":   true,
	"ONBEFOREMODIFYRECORD":  true,
	"ONAFTERMODIFYRECORD":   true,
	"ONBEFOREPASSFIELD":     true,
	"ONAFTERASSIGNFIELD":    true,
	"ONBEFOREPASSVARIABLE":  true,
	"ONAFTERASSIGNVARIABLE": true,
	"ONCONTROLADDIN":        true,
	"ONINPUTCHANGE":         true,
}

func IsStatementKeyword(s string) bool {
	return StatementKeywords[strings.ToUpper(s)]
}

func IsDeclKeyword(s string) bool {
	return DeclKeywords[strings.ToUpper(s)]
}

func IsZeroArgGlobalBuiltin(s string) bool {
	return ZeroArgGlobalBuiltins[strings.ToUpper(s)]
}

func IsRecordZeroArgMethod(s string) bool {
	return RecordZeroArgMethods[strings.ToUpper(s)]
}

func IsObjectZeroArgMethod(s string) bool {
	return ObjectZeroArgMethods[strings.ToUpper(s)]
}

func IsRecordMethod(s string) bool {
	return AllRecordMethods[strings.ToUpper(s)]
}

func IsGlobalBuiltin(s string) bool {
	return GlobalBuiltins[strings.ToUpper(s)]
}

func IsSystemTriggerName(s string) bool {
	return SystemTriggerNames[strings.ToUpper(s)]
}
