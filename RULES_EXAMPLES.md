# Примеры исполнения правил PreReviewCAL

Документ сгенерирован по всем существующим тестам: golden-кейсы `testdata/cases/` (проверяются `TestGoldenCases` — побайтовое сравнение, идемпотентность, round-trip, CRLF), unit-тесты и дифф-тесты режимов.

Правила, помеченные «только warning», не изменяют код.

## Сводная таблица

| Правило | Что делает | Тест |
|---|---|---|
| CAL001 | Удаляет неиспользуемые локальные переменные и пустые `VAR`-блоки | [input](testdata/cases/cal001_unused_local/input/c50001%20-%20Unused%20Local%20Var.txt) / [expected](testdata/cases/cal001_unused_local/expected/c50001%20-%20Unused%20Local%20Var.txt) |
| CAL002 | Удаляет неиспользуемые глобальные переменные | [input](testdata/cases/cal002_unused_global/input/p50002%20-%20Unused%20Global%20Page.txt) / [expected](testdata/cases/cal002_unused_global/expected/p50002%20-%20Unused%20Global%20Page.txt) |
| CAL003 | Неиспользуемый параметр — только warning, без правок | [TestDiagnosticWarnings#L124](internal/rules/rules_test.go#L124) |
| CAL004 | Локальные `TextConst` переносит в глобальный `VAR` | [input](testdata/cases/cal004_local_textconst/input/c50004%20-%20Local%20TextConst.txt) / [expected](testdata/cases/cal004_local_textconst/expected/c50004%20-%20Local%20TextConst.txt) |
| CAL005 | Удаляет `WITH ... DO`, qualified-ит все ссылки | [input](testdata/cases/cal005_no_with/input/c50005%20-%20No%20With.txt) / [expected](testdata/cases/cal005_no_with/expected/c50005%20-%20No%20With.txt) |
| CAL010 | Суффикс `L` у локальных переменных; >30 символов — только warning | [TestDiagnosticWarnings#L168](internal/rules/rules_test.go#L168) |
| CAL011 | Суффикс `P` у параметров; `[EventSubscriber]` — только warning | [TestDiagnosticWarnings#L143](internal/rules/rules_test.go#L143) |
| CAL012 | Регистр идентификаторов по объявлению | [input](testdata/cases/cal012_identifier_casing/input/c50012%20-%20Identifier%20Casing.txt) / [expected](testdata/cases/cal012_identifier_casing/expected/c50012%20-%20Identifier%20Casing.txt) |
| CAL020 | `()` к вызовам без аргументов; `EXIT()` -> `EXIT`; свойства DotNet/Automation/OCX не трогает | [TestCallParensDotNetGuard](internal/rules/dotnet_guard_test.go#L83) |
| CAL021 | Удаляет избыточные скобки, не склеивая токены | [TestFixRedundantParensNoTokenGluing#L34](internal/rules/parens_rules_test.go#L34) |
| CAL030 | Отступы блоков (`+2`, `VAR`-секции, `REPEAT`/`UNTIL`) | [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| CAL031 | Размещение `BEGIN` для однострочного и многострочного `IF` | [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| CAL032 | Пробелы вокруг операторов, запятых, скобок вызова | [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| CAL033 | Ключевые слова и встроенные функции в UPPERCASE; члены DotNet/Automation/OCX не трогаются | [TestKeywordCasingDotNetGuard](internal/rules/dotnet_guard_test.go#L25) / [input](testdata/cases/dotnet_member_guard/input/c50022%20-%20DotNet%20Guard.txt) |
| CAL034 | Убирает двойные `;;` и лишние пустые строки | [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |

---

## 1. Переменные и параметры (CAL001–CAL005, CAL010–CAL012)

### CAL001 — неиспользуемые локальные переменные

Удаляются неиспользуемые локальные переменные; пустой `VAR`-блок триггера удаляется целиком.

**Исходный код:**

```cal
    OnRun=VAR
            UnusedTrigVarL@1000 : Integer;
          BEGIN
    ...
    VAR
      UsedVarL@1000 : Integer;
      UnusedVarL@1001 : Text[50];
      UnusedLocalTxt@1002 : TextConst 'ENU=Never used';
```

**Исправленный код:**

```cal
    OnRun=BEGIN
    ...
    VAR
      UsedVarL@1000 : Integer;
```

**Тест:** [input](testdata/cases/cal001_unused_local/input/c50001%20-%20Unused%20Local%20Var.txt) / [expected](testdata/cases/cal001_unused_local/expected/c50001%20-%20Unused%20Local%20Var.txt), триггер в таблице: [input](testdata/cases/all_rules_table/input/t50101%20-%20Custom%20Entry.txt)

### CAL002 — неиспользуемые глобальные переменные

**Исходный код:**

```cal
    VAR
      UsedInVisibleGlobal@1002 : Boolean INDATASET;
      UnusedGlobal@1003 : Decimal;
      UnusedGlobal2@1004 : Code[20];
```

**Исправленный код:**

```cal
    VAR
      UsedInVisibleGlobal@1002 : Boolean INDATASET;
```

**Тест:** [input](testdata/cases/cal002_unused_global/input/p50002%20-%20Unused%20Global%20Page.txt) / [expected](testdata/cases/cal002_unused_global/expected/p50002%20-%20Unused%20Global%20Page.txt)

### CAL003 — неиспользуемый параметр (только warning)

**Исходный код (остаётся без изменений):**

```cal
    PROCEDURE Calculate@1(AmountP@1000 : Decimal;UnusedParamP@1001 : Integer) Result : Decimal;
    BEGIN
      // CAL003: UnusedParamP must produce a warning without altering signature
      Result := AmountP * 2;
    END;
```

**Исправленный код:** без изменений, выдается `warning` о неиспользуемом параметре.

**Тест:** [input](testdata/cases/cal003_unused_param/input/c50003%20-%20Unused%20Param.txt), [TestDiagnosticWarnings/CAL003_warning](internal/rules/rules_test.go#L124)

### CAL004 — локальные `TextConst` в глобальный `VAR`

**Исходный код (объявление в процедуре):**

```cal
    VAR
      ConfirmMsg@1005 : TextConst 'ENU=Proceed with %1?;RUS=Продолжить для %1?';
      TotalQty@1006 : Decimal;
```

**Исправленный код (в глобальном `VAR`, `@ID` перенумерован):**

```cal
    VAR
      UsedGlobal@1001 : Integer;
      ConfirmMsg@1002 : TextConst 'ENU=Proceed with %1?;RUS=Продолжить для %1?';
```

**Тест:** [input](testdata/cases/cal004_local_textconst/input/c50004%20-%20Local%20TextConst.txt) / [expected](testdata/cases/cal004_local_textconst/expected/c50004%20-%20Local%20TextConst.txt), [all_rules_codeunit](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt)

### CAL005 — удаление `WITH ... DO`

**Исходный код:**

```cal
      WITH ItemLedgEntryL DO BEGIN
        SETRANGE("Item No.", ItemNoP);
        SETRANGE(Open, TRUE);
        IF FINDSET() THEN
          REPEAT
            TotalQtyL += Quantity;
          UNTIL NEXT() = 0;
      END;

      // CAL005: single-statement WITH
      WITH ItemLedgEntryL DO
        TotalQtyL += "Remaining Quantity";
```

**Исправленный код:**

```cal
      ItemLedgEntryL.SETRANGE("Item No.", ItemNoP);
      ItemLedgEntryL.SETRANGE(Open, TRUE);
      IF ItemLedgEntryL.FINDSET() THEN
        REPEAT
          TotalQtyL += ItemLedgEntryL.Quantity;
        UNTIL ItemLedgEntryL.NEXT() = 0;

      TotalQtyL += ItemLedgEntryL."Remaining Quantity";
```

**Тест:** [input](testdata/cases/cal005_no_with/input/c50005%20-%20No%20With.txt) / [expected](testdata/cases/cal005_no_with/expected/c50005%20-%20No%20With.txt)

### CAL010 — суффикс `L` у локальных переменных

Переименование с `L`; имя длиннее 30 символов с учётом `L` — только warning, без переименования.

**Исходный код:**

```cal
    VAR
      Counter@1001 : Integer;
      AlreadyHasL@1002 : Decimal;
    BEGIN
      Counter := 10;
      AlreadyHasL := Counter * FactorP;
```

**Исправленный код:**

```cal
    VAR
      CounterL@1001 : Integer;
      AlreadyHasL@1002 : Decimal;
    BEGIN
      CounterL := 10;
      AlreadyHasL := CounterL * FactorP;
```

**Тест:** [input](testdata/cases/cal010_local_suffix/input/c50010%20-%20Local%20Suffix.txt) / [expected](testdata/cases/cal010_local_suffix/expected/c50010%20-%20Local%20Suffix.txt), warning по 30-символьному лимиту: [TestDiagnosticWarnings/CAL010](internal/rules/rules_test.go#L168)

### CAL011 — суффикс `P` у параметров

Параметры `[EventSubscriber]` не переименовываются — только warning.

**Исходный код:**

```cal
    PROCEDURE UpdateDoc@1(DocNo@1000 : Code[20];QtyP@1001 : Decimal);
    BEGIN
      MESSAGE('%1 %2', DocNo, QtyP);
    END;
```

**Исправленный код:**

```cal
    PROCEDURE UpdateDoc@1(DocNoP@1000 : Code[20];QtyP@1001 : Decimal);
    BEGIN
      MESSAGE('%1 %2', DocNoP, QtyP);
    END;
```

**Тест:** [input](testdata/cases/cal011_param_suffix/input/c50011%20-%20Param%20Suffix%20And%20EventSub.txt) / [expected](testdata/cases/cal011_param_suffix/expected/c50011%20-%20Param%20Suffix%20And%20EventSub.txt), [TestDiagnosticWarnings/CAL011_EventSubscriber_warning](internal/rules/rules_test.go#L143)

### CAL012 — регистр идентификаторов по объявлению

**Исходный код:**

```cal
      globalcounter := inputvaluep;
      itemledgentryl.SETRANGE("Entry No.", globalcounter);
      IF itemledgentryl.FINDFIRST() THEN
        helperproc();
```

**Исправленный код:**

```cal
      GlobalCounter := InputValueP;
      ItemLedgEntryL.SETRANGE("Entry No.", GlobalCounter);
      IF ItemLedgEntryL.FINDFIRST() THEN
        HelperProc();
```

**Тест:** [input](testdata/cases/cal012_identifier_casing/input/c50012%20-%20Identifier%20Casing.txt) / [expected](testdata/cases/cal012_identifier_casing/expected/c50012%20-%20Identifier%20Casing.txt)

---

## 2. Скобки вызовов (CAL020–CAL021)

### CAL020 — `()` для вызовов без аргументов

**Исходный код:**

```cal
      WorkDateL := TODAY;
      CompanyL := COMPANYNAME;
      DoInternal;
      ItemLedgEntryL.RESET;
      IF ItemLedgEntryL.ISEMPTY THEN
        ...
        UNTIL ItemLedgEntryL.NEXT = 0;
```

**Исправленный код:**

```cal
      WorkDateL := TODAY();
      CompanyL := COMPANYNAME();
      DoInternal();
      ItemLedgEntryL.RESET();
      IF ItemLedgEntryL.ISEMPTY() THEN
        ...
        UNTIL ItemLedgEntryL.NEXT() = 0;
```

**Тест:** [input](testdata/cases/cal020_call_parens/input/c50020%20-%20Call%20Parens.txt) / [expected](testdata/cases/cal020_call_parens/expected/c50020%20-%20Call%20Parens.txt), `SalesHeader.FINDSET` → `FINDSET()`: [all_rules_codeunit](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt)

### CAL020 — `EXIT` без пустых скобок

**Исходный код:**

```cal
        EXIT();
```

**Исправленный код** (`EXIT(val)` не меняется):

```cal
        EXIT;
```

**Тест:** [input](testdata/cases/cal020_call_parens/input/c50020%20-%20Call%20Parens.txt) / [expected](testdata/cases/cal020_call_parens/expected/c50020%20-%20Call%20Parens.txt)

### CAL021 — удаление избыточных скобок

**Исходный код:**

```cal
      IF (FlagP) THEN
        TotalL := (QtyP) + (1);

      IF ((QtyP > 0) AND (TotalL > 0)) THEN
        Result := TRUE;

      IF NOT (QtyP = 0) THEN
        EXIT((Result));
```

**Исправленный код** (обязательные скобки приоритетов и `NOT (сравнение)` сохранены):

```cal
      IF FlagP THEN
        TotalL := QtyP + 1;

      IF (QtyP > 0) AND (TotalL > 0) THEN
        Result := TRUE;

      IF NOT (QtyP = 0) THEN
        EXIT(Result);
```

**Тест:** [input](testdata/cases/cal021_redundant_parens/input/c50021%20-%20Redundant%20Parens.txt) / [expected](testdata/cases/cal021_redundant_parens/expected/c50021%20-%20Redundant%20Parens.txt)

### CAL021 — удаление скобок не склеивает токены

При удалении `(`/`)` соседние символы проверяются: если удаление склеило бы два идентификатора/ключевых слова/числа, скобка заменяется на пробел (пункт 1.1 задания).

**Исходный код:**

```cal
      IF NOT(_SalesHeaderExt.GET(TEXT('Doc No.'))) THEN
        EXIT(FALSE);
      IF (FlagL)THEN
        EXIT(FALSE);
      Result := (1)AND (2);
      IF NOT(Документ) THEN
        EXIT(FALSE);
```

**Исправленный код:**

```cal
      IF NOT _SalesHeaderExt.GET(TEXT('Doc No.')) THEN
        EXIT(FALSE);
      IF FlagL THEN
        EXIT(FALSE);
      Result := 1 AND 2;
      IF NOT Документ THEN
        EXIT(FALSE);
```

**Тест:** [TestFixRedundantParensNoTokenGluing](internal/rules/parens_rules_test.go#L34) (10 кейсов, вкл. кириллические идентификаторы, quoted-ident, вызовы), [TestFixRedundantParensOutputLexesSeparately](internal/rules/parens_rules_test.go#L103)

### CAL020 + CAL033 — защита членов DotNet / Automation / OCX (пункт 1.2)

Методы и свойства .NET/Automation/OCX регистрозависимы: CAL033 не меняет регистр членов после `.` у таких получателей (включая цепочки `_XmlDoc.Root.delete()`), CAL020 не добавляет `()` к их свойствам (`.Count`, `Quit` в позиции оператора). Область видимости переменных (`localScope`) прокидывается в `applyKeywordCasing`.

**Исходный код:**

```cal
      _ServerDocumentFile.delete(_ServerDocumentFileName);
      IF _XmlDoc.Count > 0 THEN
        ERROR('Xml count is %1', _XmlDoc.Count);
      _XmlDoc.DocumentElement.set_OuterXml(_ServerDocumentFileName);
      OutlookApp.Quit;
      ItemLedgEntryL.reset;
      ItemLedgEntryL.deleteall(FALSE);
```

**Исправленный код** (члены DotNet/Automation сохранены, record-методы исправлены):

```cal
      _ServerDocumentFile.delete(_ServerDocumentFileNameP);
      IF _XmlDoc.Count > 0 THEN
        ERROR('Xml count is %1', _XmlDoc.Count);
      _XmlDoc.DocumentElement.set_OuterXml(_ServerDocumentFileNameP);
      OutlookAppL.Quit;
      ItemLedgEntryL.RESET();
      ItemLedgEntryL.DELETEALL(FALSE);
```

**Тест:** [TestKeywordCasingDotNetGuard](internal/rules/dotnet_guard_test.go#L25), [TestCallParensDotNetGuard](internal/rules/dotnet_guard_test.go#L83), [TestChainRootVarAndInterop](internal/rules/dotnet_guard_test.go#L138), golden: [input](testdata/cases/dotnet_member_guard/input/c50022%20-%20DotNet%20Guard.txt) / [expected](testdata/cases/dotnet_member_guard/expected/c50022%20-%20DotNet%20Guard.txt)

---

## 3. Форматирование (CAL030–CAL034)

### CAL030 — отступы

Тело на `+2`, `VAR`-секции и вложенные блоки (`REPEAT`/`UNTIL`) по уровню.

**Исходный код:**

```cal
    procedure ProcessEntries@1(...) Result : Decimal;
    begin
      with itemledgentry do begin
        if findset then
        begin
            repeat
              totalqty:=totalqty+Quantity;
            until next=0;
        end;
      end;
```

**Исправленный код:**

```cal
    PROCEDURE ProcessEntries@1(...) Result : Decimal;
    VAR
      ItemLedgEntryL@1003 : Record 32;
      TotalQtyL@1006 : Decimal;
    BEGIN
      IF ItemLedgEntryL.FINDSET() THEN BEGIN
        REPEAT
          TotalQtyL := TotalQtyL + ItemLedgEntryL.Quantity;
        UNTIL ItemLedgEntryL.NEXT() = 0;
      END;
```

**Тест:** [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt) / [expected](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt)

### CAL031 — однострочный `IF`: `BEGIN` на той же строке

**Исходный код:**

```cal
      if TotalL>0 then
      begin
          TotalL:=TotalL+1;
      end;
```

**Исправленный код:**

```cal
      IF TotalL > 0 THEN BEGIN
        TotalL := TotalL + 1;
      END;
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt)

### CAL031 — многострочный `IF`: `THEN` под `IF`, `BEGIN` отдельной строкой

**Исходный код:**

```cal
      if (QtyP>0) and
         (PriceP>0) then begin
        TotalL:=TotalL+PriceP;
      end;
```

**Исправленный код:**

```cal
      IF (QtyP > 0) AND
         (PriceP > 0) THEN
      BEGIN
        TotalL := TotalL + PriceP;
      END;
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt)

### CAL032 — пробелы вокруг операторов и запятых

**Исходный код:**

```cal
      TotalL:=QtyP*PriceP;
      MESSAGE( '%1, %2' ,TotalL ,IdxL );
```

**Исправленный код:**

```cal
      TotalL := QtyP * PriceP;
      MESSAGE('%1, %2', TotalL, IdxL);
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), `Factor:=2` → `FactorL := 2`: [all_rules_report](testdata/cases/all_rules_report/input/r50102%20-%20Custom%20Report.txt)

### CAL033 — ключевые слова в UPPERCASE

**Исходный код:**

```cal
    local procedure FormatDemo@1(QtyP@1000 : Decimal;PriceP@1001 : Decimal);
    var
    begin
      if (QtyP>0) and (PriceP>0) then
      if confirm(confirmmsg,false,companyname) then
```

**Исправленный код:**

```cal
    LOCAL PROCEDURE FormatDemo@1(QtyP@1000 : Decimal;PriceP@1001 : Decimal);
    VAR
    BEGIN
      IF (QtyP > 0) AND (PriceP > 0) THEN
      IF CONFIRM(ConfirmMsg, FALSE, COMPANYNAME()) THEN
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [all_rules_codeunit](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt)

### CAL034 — двойные `;;` и лишние пустые строки

**Исходный код:**

```cal
      usedglobal:=1;;


      TotalL:=QtyP*PriceP;;
```

**Исправленный код:**

```cal
      UsedGlobal := 1;

      TotalL := QtyP * PriceP;
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt)

---

## 4. Комбинированные и режимные тесты

### Все правила сразу (Codeunit целиком)

**Исходный код:**

```cal
    procedure ProcessEntries@1(ItemNo@1000 : Code[20];PostingDate@1001 : Date) Result : Decimal;
    begin
      with itemledgentry do begin
        setrange("Item No.",itemno);
        if findset then
        begin
            repeat
              totalqty:=totalqty+Quantity;
            until next=0;
        end;
      end;
    end;
```

**Исправленный код:**

```cal
    PROCEDURE ProcessEntries@1(ItemNoP@1000 : Code[20];PostingDateP@1001 : Date) Result : Decimal;
    VAR
      ItemLedgEntryL@1003 : Record 32;
      TotalQtyL@1006 : Decimal;
    BEGIN
      IF ItemLedgEntryL.FINDSET() THEN BEGIN
        REPEAT
          TotalQtyL := TotalQtyL + ItemLedgEntryL.Quantity;
        UNTIL ItemLedgEntryL.NEXT() = 0;
      END;
    END;
```

**Тест:** [TestGoldenCases/all_rules_codeunit](internal/rules/rules_test.go#L24), [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt) / [expected](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt)

### Table / Report / XMLport — вложенные триггеры

`OnValidate` в поле таблицы, `OnAfterGetRecord` в DataItem отчёта, `OnPreXMLport`/`OnPostXMLport` XMLport — все правила применяются к телам триггеров.

**Исходный код:**

```cal
    OnValidate=VAR
                 ItemLedg@1000 : Record 32;
               BEGIN
                 with itemledg do begin
                     setrange("Item No.",Rec."Item No.");
                     if findfirst then
                       Rec.Quantity:=Quantity;
                 end;
               END;
```

**Исправленный код:**

```cal
    OnValidate=VAR
                 ItemLedgL@1000 : Record 32;
               BEGIN
                 ItemLedgL.SETRANGE("Item No.", Rec."Item No.");
                 IF ItemLedgL.FINDFIRST() THEN
                   Rec.Quantity := ItemLedgL.Quantity;
               END;
```

**Тест:** [all_rules_table](testdata/cases/all_rules_table/input/t50101%20-%20Custom%20Entry.txt), [all_rules_report](testdata/cases/all_rules_report/input/r50102%20-%20Custom%20Report.txt), [all_rules_xmlport](testdata/cases/all_rules_xmlport/input/x50103%20-%20Custom%20Export.txt)

### Отключение правил в конфиге

**Вход (CAL005/CAL010/CAL020 выключены, остальные работают):**

```cal
      WITH ItemLedgEntry DO BEGIN
        TotalQty := TODAY;
```

**Результат:** `WITH` сохранён, `TotalQty` без `L`, `TODAY` без `()`; при этом `ItemNo` → `ItemNoP`, `ConfirmMsg` → в глобальный `VAR`.

**Тест:** [TestConfigToggleDisablesSpecificRules](internal/rules/rules_test.go#L175)

### Diff-режим: `MaxOldFunctionLines=10`

Неизменённые строки старых процедур не трогаются; изменённая строка внутри старого `WITH` форматируется по anchored-отступам, но `WITH` не раскрывается.

**Фрагмент (строка изменена, старый `WITH` сохранён):**

```cal
        with oldrec do begin
          ...
          // CAL020, CAL032, CAL033: new line inside old WITH (old WITH must NOT be removed!)
          NewFlagL := (TODAY() <> 0D);
        end;
```

**Тест:** [TestDiffModeLimitAndVariantAIndentation](internal/rules/diff_mode_test.go#L254)

### Diff-режим: `MaxOldFunctionLines=0` — изменённая старая процедура чистится целиком

**Исходный код:**

```cal
    procedure LargeOldProc@3(OldParam@1000 : Integer);
    begin
      with oldrec do begin
        setrange("Item No.",'OLD');
      end;
```

**Исправленный код:**

```cal
    PROCEDURE LargeOldProc@3(OldParamP@1000 : Integer);
    begin
      OldRecL.SETRANGE("Item No.", 'OLD');
```

(неиспользуемое удалено, `WITH` раскрыт; нетронутые процедуры остаются без изменений)

**Тест:** [TestDiffModeUnlimitedZeroTransformsLargeOldFunctionWholly](internal/rules/diff_mode_test.go#L279)

### Идемпотентность и побайтовая сохранность

Повторный прогон по исправленному коду = 0 изменений; при всех выключенных правилах вход = выход байт в байт; CRLF сохраняется.

**Тест:** [TestGoldenCases, шаги 3–5](internal/rules/rules_test.go#L84)

---

## 5. Инфраструктура (не правила)

| Аспект | Что проверяется | Тест |
|---|---|---|
| Лексер | Lossless round-trip CRLF, комментарии, строки с `''`, quoted-ident, `0D` | [TestLexRoundTrip](internal/cal/parser_test.go#L5) |
| Парсер секций | `VAR` / `PROCEDURE` / `[EventSubscriber]` | [TestParseCodeSection](internal/cal/parser_test.go#L20) |
| CLI | Режимы fix/check/diff | [TestCLIFixCheckAndDiff](internal/cli/cli_test.go#L11) |
| Git-режимы | Три режима диффа | [TestGitWorkflowsThreeModes](internal/cli/git_workflow_test.go#L39) |
