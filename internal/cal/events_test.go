package cal

import "testing"

// Task 1.4: NAV 2017 event attributes [Integration]/[Business] (with or without
// arguments, any case) plus AL [IntegrationEvent]/[BusinessEvent] must all be
// recognized as event publishers; [EventSubscriber] as subscriber.
func TestProcDeclEventAttributes(t *testing.T) {
	code := "    [Integration]\n" +
		"    LOCAL PROCEDURE OnAfterCalc@1(DocNo@1000 : Code[20]);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    [integration()]\n" +
		"    PROCEDURE OnBeforeScoped@2(Qty@1000 : Decimal);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    [Business]\n" +
		"    LOCAL PROCEDURE OnBigSale@3(NameP@1000 : Text[100]);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    [Business(Scope='Instance', IntegrationKey='K1')]\n" +
		"    LOCAL PROCEDURE OnScoped@4(CodeP@1000 : Code[10]);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    [IntegrationEvent(false, false)]\n" +
		"    LOCAL PROCEDURE OnAlEvent@5(XP@1000 : Integer);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    [EventSubscriber(Codeunit,80,OnBeforePost)]\n" +
		"    LOCAL PROCEDURE OnHandle@6(YP@1000 : Integer);\n" +
		"    BEGIN\n" +
		"    END;\n" +
		"\n" +
		"    PROCEDURE Regular@7(ZP@1000 : Integer);\n" +
		"    BEGIN\n" +
		"    END;\n"

	cs := ParseCodeSection(code)
	if len(cs.Procedures) != 7 {
		t.Fatalf("expected 7 procedures, got %d", len(cs.Procedures))
	}
	wantPub := []bool{true, true, true, true, true, false, false}
	wantSub := []bool{false, false, false, false, false, true, false}
	for i, p := range cs.Procedures {
		if got := p.IsEventPublisher(); got != wantPub[i] {
			t.Errorf("proc %s: IsEventPublisher() = %v, want %v", p.Name, got, wantPub[i])
		}
		if got := p.IsEventSubscriber(); got != wantSub[i] {
			t.Errorf("proc %s: IsEventSubscriber() = %v, want %v", p.Name, got, wantSub[i])
		}
	}
}
