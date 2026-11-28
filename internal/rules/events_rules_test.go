package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"prereviewcal/internal/config"
	"prereviewcal/internal/objectfile"
)

const nav2017EventsObject = `OBJECT Codeunit 50023 NAV2017 Events
{
  OBJECT-PROPERTIES
  {
    Date=28.11.26;
    Time=12:00:00;
    Modified=Yes;
    Version List=TEST;
  }
  PROPERTIES
  {
  }
  CODE
  {
    [Integration]
    LOCAL PROCEDURE OnAfterCalcDiscount@10(DiscountGroup@1000 : Code[20];DiscValue@1001 : Decimal);
    BEGIN
    END;

    [integration()]
    LOCAL PROCEDURE OnBeforeScoped@11(ScopeKey@1000 : Text[50]);
    BEGIN
    END;

    [Business]
    LOCAL PROCEDURE OnBigSale@12(CustomerName@1000 : Text[100];HiddenNote@1001 : Text[100]);
    BEGIN
    END;

    [Business(Scope='Instance', IntegrationKey='SaleKey')]
    LOCAL PROCEDURE OnScopedSale@13(BizData@1000 : Record 36);
    BEGIN
    END;

    [EventSubscriber(Codeunit,50023,OnAfterCalcDiscount)]
    LOCAL PROCEDURE HandleDiscount@14(UsedDocNo@1000 : Code[20];UnusedSub@1001 : Integer);
    BEGIN
      MESSAGE('%1', UsedDocNo);
    END;

    PROCEDURE RegularProc@1(Amount@1000 : Decimal);
    VAR
      TotalL@1001 : Decimal;
    BEGIN
      TotalL := Amount;
    END;

    BEGIN
    END.
  }
}
`

func TestNAV2017EventPublisherExclusion(t *testing.T) {
	root := repoRoot(t)
	cfg := config.Default()
	ws := objectfile.NewWorkspaceIndex(filepath.Join(root, "testdata", "symbols"), cfg.FileNaming)

	res, err := Process([]byte(nav2017EventsObject), cfg, ws)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	output := string(res.Output)

	// 1. Event publishers must NOT be renamed by CAL011 (no P suffix added).
	for _, name := range []string{"DiscountGroup", "DiscValue", "ScopeKey", "CustomerName", "HiddenNote", "BizData"} {
		if strings.Contains(output, name+"P@") {
			t.Errorf("event publisher parameter '%s' was renamed (must stay untouched)", name)
		}
	}

	// 2. Event publishers must NOT produce CAL003 (unused-param) or CAL011 (param-suffix) diagnostics.
	for _, d := range res.Diagnostics {
		if d.Rule == config.RuleUnusedParam || d.Rule == config.RuleParamSuffix {
			if strings.Contains(d.Message, "DiscountGroup") || strings.Contains(d.Message, "DiscValue") ||
				strings.Contains(d.Message, "ScopeKey") || strings.Contains(d.Message, "CustomerName") ||
				strings.Contains(d.Message, "HiddenNote") || strings.Contains(d.Message, "BizData") {
				t.Errorf("unexpected diagnostic for event publisher param: %+v", d)
			}
		}
	}

	// 3. Regular procedure parameter MUST still be renamed by CAL011.
	if !strings.Contains(output, "AmountP@") {
		t.Errorf("regular proc parameter Amount was not renamed to AmountP:\n%s", output)
	}

	// 4. EventSubscriber: params never auto-renamed (CAL011 warning only).
	if strings.Contains(output, "UnusedSubP@") || strings.Contains(output, "UsedDocNoP@") {
		t.Errorf("EventSubscriber parameters must not be auto-renamed:\n%s", output)
	}
	// 5. EventSubscriber: unused param keeps CAL003 warning (Fixed=false).
	foundCAL003Sub := false
	foundCAL011Sub := false
	for _, d := range res.Diagnostics {
		if d.Rule == config.RuleUnusedParam && !d.Fixed && strings.Contains(d.Message, "UnusedSub") {
			foundCAL003Sub = true
		}
		if d.Rule == config.RuleParamSuffix && !d.Fixed && strings.Contains(d.Message, "EventSubscriber") {
			foundCAL011Sub = true
		}
	}
	if !foundCAL003Sub {
		t.Error("expected CAL003 warning for unused EventSubscriber parameter UnusedSub")
	}
	if !foundCAL011Sub {
		t.Error("expected CAL011 warning for EventSubscriber parameter")
	}
}
