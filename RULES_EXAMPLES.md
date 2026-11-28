# Примеры исполнения правил PreReviewCAL

Документ сгенерирован по всем существующим тестам: golden-кейсы `testdata/cases/` (проверяются `TestGoldenCases` — побайтовое сравнение, идемпотентность, round-trip, CRLF), unit-тесты и дифф-тесты режимов.

Правила, помеченные «только warning», не изменяют код. Комментарии `//` в примерах показывают: **НАРУШЕНИЕ** — что нарушало правило; **Исправлено** — что изменил процессор; **НЕ трогать** — что выглядело как нарушение, но менять его нельзя.

## Сводная таблица

| Правило | Что делает | Тест |
|---|---|---|
| [CAL001](#cal001) | Удаляет неиспользуемые локальные переменные и пустые `VAR`-блоки | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal001_unused_local/input/c50001%20-%20Unused%20Local%20Var.txt) / [expected](testdata/cases/cal001_unused_local/expected/c50001%20-%20Unused%20Local%20Var.txt) |
| [CAL002](#cal002) | Удаляет неиспользуемые глобальные переменные | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal002_unused_global/input/p50002%20-%20Unused%20Global%20Page.txt) / [expected](testdata/cases/cal002_unused_global/expected/p50002%20-%20Unused%20Global%20Page.txt) |
| [CAL003](#cal003) | Неиспользуемый параметр — только warning, без правок | [TestDiagnosticWarnings#L124](internal/rules/rules_test.go#L124), [input](testdata/cases/cal003_unused_param/input/c50003%20-%20Unused%20Param.txt) / [expected](testdata/cases/cal003_unused_param/expected/c50003%20-%20Unused%20Param.txt) |
| [CAL004](#cal004) | Локальные `TextConst` переносит в глобальный `VAR` | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal004_local_textconst/input/c50004%20-%20Local%20TextConst.txt) / [expected](testdata/cases/cal004_local_textconst/expected/c50004%20-%20Local%20TextConst.txt) |
| [CAL005](#cal005) | Удаляет `WITH ... DO`, квалифицирует все ссылки; [строгая схема + обязательный `--symbols-dir`](#cal005-strict) | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal005_no_with/input/c50005%20-%20No%20With.txt) / [expected](testdata/cases/cal005_no_with/expected/c50005%20-%20No%20With.txt) |
| [CAL010](#cal010) | Суффикс `L` у локальных переменных; >30 символов — только warning | [TestDiagnosticWarnings#L168](internal/rules/rules_test.go#L168), [input](testdata/cases/cal010_local_suffix/input/c50010%20-%20Local%20Suffix.txt) / [expected](testdata/cases/cal010_local_suffix/expected/c50010%20-%20Local%20Suffix.txt) |
| [CAL011](#cal011) | Суффикс `P` у параметров; `[EventSubscriber]` — только warning | [TestDiagnosticWarnings#L143](internal/rules/rules_test.go#L143), [input](testdata/cases/cal011_param_suffix/input/c50011%20-%20Param%20Suffix%20And%20EventSub.txt) / [expected](testdata/cases/cal011_param_suffix/expected/c50011%20-%20Param%20Suffix%20And%20EventSub.txt) |
| [CAL012](#cal012) | Регистр идентификаторов по объявлению | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal012_identifier_casing/input/c50012%20-%20Identifier%20Casing.txt) / [expected](testdata/cases/cal012_identifier_casing/expected/c50012%20-%20Identifier%20Casing.txt) |
| [CAL010+CAL011+CAL033](#cal010-011-033-builtin-conflict) | Конфликт имени с системной функцией: `+L`/`+P`/`+G` + warning (пункт 1.5) | [TestGlobalBuiltinConflictRename#L143](internal/rules/builtin_conflict_test.go#L143), [input](testdata/cases/builtin_name_conflict/input/c50024%20-%20Builtin%20Name%20Conflict.txt) / [expected](testdata/cases/builtin_name_conflict/expected/c50024%20-%20Builtin%20Name%20Conflict.txt) |
| [CAL020](#cal020) | `()` к вызовам без аргументов; [EXIT](#cal020-exit) | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal020_call_parens/input/c50020%20-%20Call%20Parens.txt) / [expected](testdata/cases/cal020_call_parens/expected/c50020%20-%20Call%20Parens.txt) |
| [CAL021](#cal021) | Удаляет избыточные скобки; [без склейки токенов](#cal021-glue) | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal021_redundant_parens/input/c50021%20-%20Redundant%20Parens.txt) / [expected](testdata/cases/cal021_redundant_parens/expected/c50021%20-%20Redundant%20Parens.txt) |
| [CAL020+CAL033](#cal020-cal033-dotnet) | Защита членов DotNet/Automation/OCX (пункт 1.2) | [TestKeywordCasingDotNetGuard#L25](internal/rules/dotnet_guard_test.go#L25), [input](testdata/cases/dotnet_member_guard/input/c50022%20-%20DotNet%20Guard.txt) / [expected](testdata/cases/dotnet_member_guard/expected/c50022%20-%20DotNet%20Guard.txt) |
| [CAL003+CAL011](#cal003-cal011-nav2017) | События NAV 2017 `[Integration]`/`[Business]` (пункт 1.4) | [TestNAV2017EventPublisherExclusion#L65](internal/rules/events_rules_test.go#L65), [input](testdata/cases/nav2017_events/input/c50023%20-%20NAV2017%20Events.txt) / [expected](testdata/cases/nav2017_events/expected/c50023%20-%20NAV2017%20Events.txt) |
| [CAL030](#cal030) | Отступы блоков (`+2`, `VAR`-секции, `REPEAT`/`UNTIL`) | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| [CAL031](#cal031-single) | Размещение `BEGIN` для однострочного и [многострочного](#cal031-multi) `IF` | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| [CAL032](#cal032) | Пробелы вокруг операторов, запятых, скобок вызова | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| [CAL033](#cal033) | Ключевые слова и встроенные функции в UPPERCASE | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |
| [CAL034](#cal034) | Убирает двойные `;;` и лишние пустые строки | [TestGoldenCases#L24](internal/rules/rules_test.go#L24), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt) |

---

## 1. Переменные и параметры (CAL001–CAL005, CAL010–CAL012)

<a id="cal001"></a>
### CAL001 — неиспользуемые локальные переменные

**Исходный код:**

```cal
    OnRun=VAR
            UnusedTrigVarL@1000 : Integer;   // НАРУШЕНИЕ: переменная нигде не используется
          BEGIN
    ...
    VAR
      UsedVarL@1000 : Integer;               // НЕ трогать: используется в теле
      UnusedVarL@1001 : Text[50];            // НАРУШЕНИЕ: переменная нигде не используется
      UnusedLocalTxt@1002 : TextConst 'ENU=Never used';  // НАРУШЕНИЕ: нигде не используется
```

**Исправленный код:**

```cal
    OnRun=BEGIN          // Исправлено: пустой VAR-блок удалён вместе с последней переменной
    ...
    VAR
      UsedVarL@1000 : Integer;   // НЕ трогать: осталась (используется)
```

**Тест:** [input](testdata/cases/cal001_unused_local/input/c50001%20-%20Unused%20Local%20Var.txt) / [expected](testdata/cases/cal001_unused_local/expected/c50001%20-%20Unused%20Local%20Var.txt), триггер в таблице: [input](testdata/cases/all_rules_table/input/t50101%20-%20Custom%20Entry.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal002"></a>
### CAL002 — неиспользуемые глобальные переменные

**Исходный код:**

```cal
    VAR
      UsedInVisibleGlobal@1002 : Boolean INDATASET;  // НЕ трогать: используется
      UnusedGlobal@1003 : Decimal;                   // НАРУШЕНИЕ: нигде не используется
      UnusedGlobal2@1004 : Code[20];                 // НАРУШЕНИЕ: нигде не используется
```

**Исправленный код:**

```cal
    VAR
      UsedInVisibleGlobal@1002 : Boolean INDATASET;  // неиспользуемые удалены
```

**Тест:** [input](testdata/cases/cal002_unused_global/input/p50002%20-%20Unused%20Global%20Page.txt) / [expected](testdata/cases/cal002_unused_global/expected/p50002%20-%20Unused%20Global%20Page.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal003"></a>
### CAL003 — неиспользуемый параметр (только warning)

**Исходный код (остаётся без изменений):**

```cal
    PROCEDURE Calculate@1(AmountP@1000 : Decimal;     // НЕ трогать: параметр используется
                          UnusedParamP@1001 : Integer) Result : Decimal;
    BEGIN
      Result := AmountP * 2;
    END;
```

**Исправленный код:** без изменений — по `UnusedParamP` выдаётся только `warning` (автоудаление параметров запрещено, это меняло бы сигнатуру).

**Тест:** [input](testdata/cases/cal003_unused_param/input/c50003%20-%20Unused%20Param.txt) / [expected](testdata/cases/cal003_unused_param/expected/c50003%20-%20Unused%20Param.txt), [TestDiagnosticWarnings/CAL003_warning](internal/rules/rules_test.go#L124)

<a id="cal004"></a>
### CAL004 — локальные `TextConst` в глобальный `VAR`

**Исходный код (объявление в процедуре):**

```cal
    VAR
      ConfirmMsg@1005 : TextConst 'ENU=Proceed with %1?;RUS=Продолжить для %1?';  // НАРУШЕНИЕ: TextConst должен быть глобальным
      TotalQty@1006 : Decimal;                                       // НЕ трогать: не TextConst
```

**Исправленный код (в глобальном `VAR`, `@ID` перенумерован):**

```cal
    VAR
      UsedGlobal@1001 : Integer;                                     // НЕ трогать: уже глобальная, используется
      ConfirmMsg@1002 : TextConst 'ENU=Proceed with %1?;RUS=Продолжить для %1?';  // Исправлено: перенесён в глобальный VAR
```

**Тест:** [input](testdata/cases/cal004_local_textconst/input/c50004%20-%20Local%20TextConst.txt) / [expected](testdata/cases/cal004_local_textconst/expected/c50004%20-%20Local%20TextConst.txt), [all_rules_codeunit](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal005"></a>
### CAL005 — удаление `WITH ... DO`

**Исходный код:**

```cal
      WITH ItemLedgEntryL DO BEGIN       // НАРУШЕНИЕ: WITH запрещена
        SETRANGE("Item No.", ItemNoP);   // НАРУШЕНИЕ: метод без префикса таблицы
        SETRANGE(Open, TRUE);            // НАРУШЕНИЕ: поле Open без префикса таблицы
        IF FINDSET() THEN                // НАРУШЕНИЕ: метод без префикса таблицы
          REPEAT
            TotalQtyL += Quantity;       // НАРУШЕНИЕ: поле Quantity без префикса (TotalQtyL — локальная, НЕ трогать)
          UNTIL NEXT() = 0;              // НАРУШЕНИЕ: метод NEXT без префикса
      END;

      // CAL005: single-statement WITH
      WITH ItemLedgEntryL DO             // НАРУШЕНИЕ: однострочный WITH тоже раскрывается
        TotalQtyL += "Remaining Quantity";  // НАРУШЕНИЕ: quoted-поле без префикса
```

**Исправленный код:**

```cal
      ItemLedgEntryL.SETRANGE("Item No.", ItemNoP);  // Исправлено: добавлен префикс ItemLedgEntryL.
      ItemLedgEntryL.SETRANGE(Open, TRUE);           // Исправлено: добавлен префикс ItemLedgEntryL.
      IF ItemLedgEntryL.FINDSET() THEN               // Исправлено: добавлен префикс ItemLedgEntryL.
        REPEAT
          TotalQtyL += ItemLedgEntryL.Quantity;      // Исправлено: поле квалифицировано, TotalQtyL не тронута
        UNTIL ItemLedgEntryL.NEXT() = 0;             // Исправлено: добавлен префикс ItemLedgEntryL.

      TotalQtyL += ItemLedgEntryL."Remaining Quantity";  // Исправлено: WITH удалена, поле квалифицировано
```

**Тест:** [input](testdata/cases/cal005_no_with/input/c50005%20-%20No%20With.txt) / [expected](testdata/cases/cal005_no_with/expected/c50005%20-%20No%20With.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal005-strict"></a>
### CAL005 — строгая анализ схемы + обязательный `--symbols-dir` (пункт 1.3)

С 1.3 CAL005 работает строго: CLI отказывается запускаться без `--symbols-dir` (или `symbolsDir` в конфиге) при включённом CAL005; `WITH` раскрывается **только** если цель резолвится в схему объекта из `--symbols-dir` (таблицы — поля, все объекты — публичные процедуры), и каждый идентификатор в блоке классифицирован. Неизвестная схема или неизвестный идентификатор → блок **не трогается**, выдаётся `warning`. Системные enum-типы NAV 2017 (`ACTION`, `DATATYPE`, `TRANSACTIONTYPE`, `TESTPERMISSIONS`, ...) добавлены в `GlobalBuiltins` и никогда не префиксируются перед `::`.

**Пример 1 — неизвестная схема: WITH остаётся + warning:**

```cal
    PROCEDURE UseUnknown@1();
    VAR
      SomeRec@1000 : Record 99999;              // НЕ трогать: таблицы нет в --symbols-dir
      TotalL@1001 : Decimal;
    BEGIN
      WITH SomeRec DO                           // НЕ раскрывать (было: слепой префикс всего подряд)
        TotalL += Quantity;                     // Quantity НЕ получит SomeRecL.Quantity — схема неизвестна
    END;
```

**Пример 2 — публичные процедуры индексируются; enum перед `::` не префиксируется:**

```cal
    PROCEDURE ApplyAdo@1(UseAdoP@1000 : Boolean);   // Исходный код
    VAR
      ItemLedgEntryL@1001 : Record 32;              // схема t32 из --symbols-dir (поля + SetUseADO)
    BEGIN
      WITH ItemLedgEntryL DO
        SetUseADO(UseAdoP);                         // НАРУШЕНИЕ: публичная процедура таблицы -> префикс нужен
    END;
```

```cal
    PROCEDURE ApplyAdo@1(UseAdoP@1000 : Boolean);   // Исправленный код
    VAR
      ItemLedgEntryL@1001 : Record 32;
    BEGIN
      ItemLedgEntryL.SetUseADO(UseAdoP);            // Исправлено: () раскрыт, SetUseADO префиксован из схемы (п.3)
    END;
```

```cal
      // enum-гард (п.2): в теле WITH ...
      IF FORM.RUNMODAL(0, ItemLedgEntryL) = ACTION::LookupOK THEN
        TotalQtyL := Quantity;                      // ACTION — НЕ трогать (enum), Quantity — поле, префикс нужен
      // результат: IF FORM.RUNMODAL(...) = ACTION::LookupOK THEN TotalQtyL := ItemLedgEntryL.Quantity;
```

**Пример 3 — CLI-гейт (п.1):**

```
prereviewcal fix c50005 - No With.txt            // error: --symbols-dir is required when rule CAL005 (no-with) is enabled (exit 2)
prereviewcal fix --symbols-dir <dir> c50005 ...  // ОК; при "CAL005": {"enabled": false} флаг не требуется
```

**Тест:** [TestWithUnknownSchemaNotExpanded](internal/rules/with_strict_test.go#L35), [TestWithKnownSchemaUnknownIdentifier](internal/rules/with_strict_test.go#L67), [TestWithSchemaProcedureQualified](internal/rules/with_strict_test.go#L100), [TestWithEnumScopeNotPrefixed](internal/rules/with_strict_test.go#L120), [TestWithNestedInnerBlocked](internal/rules/with_strict_test.go#L149), [TestWorkspaceIndexObjectProcedures](internal/objectfile/symbols_dir_test.go#L13), [TestSystemEnumTypesAreGlobalBuiltins](internal/cal/enums_builtins_test.go#L7), [TestCLIFixCheckAndDiff](internal/cli/cli_test.go#L11) (шаги 0 и 5), [input](testdata/cases/cal005_no_with/input/c50005%20-%20No%20With.txt) / [expected](testdata/cases/cal005_no_with/expected/c50005%20-%20No%20With.txt)

<a id="cal010"></a>
### CAL010 — суффикс `L` у локальных переменных

**Исходный код:**

```cal
    VAR
      Counter@1001 : Integer;                       // НАРУШЕНИЕ: локальной переменной нужен суффикс L
      AlreadyHasL@1002 : Decimal;                   // НЕ трогать: суффикс L уже есть
      VeryLongLocalVariableName30Chr@1003 : Decimal; // НЕ переименовывать: с 'L' будет >30 символов — только warning
    BEGIN
      Counter := 10;                                // ссылка будет переименована вместе с объявлением
      AlreadyHasL := Counter * FactorP;             // FactorP — параметр, НЕ трогать
```

**Исправленный код:**

```cal
    VAR
      CounterL@1001 : Integer;                      // Исправлено: добавлен суффикс L
      AlreadyHasL@1002 : Decimal;                   // НЕ трогать: уже с суффиксом
    BEGIN
      CounterL := 10;                               // Исправлено: ссылка обновлена
      AlreadyHasL := CounterL * FactorP;            // Исправлено: только ссылка на CounterL
```

**Тест:** [input](testdata/cases/cal010_local_suffix/input/c50010%20-%20Local%20Suffix.txt) / [expected](testdata/cases/cal010_local_suffix/expected/c50010%20-%20Local%20Suffix.txt), warning по 30-символьному лимиту: [TestDiagnosticWarnings/CAL010](internal/rules/rules_test.go#L168)

<a id="cal011"></a>
### CAL011 — суффикс `P` у параметров

**Исходный код:**

```cal
    PROCEDURE UpdateDoc@1(DocNo@1000 : Code[20];   // НАРУШЕНИЕ: параметру нужен суффикс P
                          QtyP@1001 : Decimal);    // НЕ трогать: суффикс P уже есть
    BEGIN
      MESSAGE('%1 %2', DocNo, QtyP);               // ссылка будет переименована
    END;
```

**Исправленный код:**

```cal
    PROCEDURE UpdateDoc@1(DocNoP@1000 : Code[20];  // Исправлено: добавлен суффикс P
                          QtyP@1001 : Decimal);
    BEGIN
      MESSAGE('%1 %2', DocNoP, QtyP);              // Исправлено: ссылка обновлена
    END;
```

Параметры `[EventSubscriber]` не переименовываются — только warning (см. [NAV 2017 события](#cal003-cal011-nav2017)).

**Тест:** [input](testdata/cases/cal011_param_suffix/input/c50011%20-%20Param%20Suffix%20And%20EventSub.txt) / [expected](testdata/cases/cal011_param_suffix/expected/c50011%20-%20Param%20Suffix%20And%20EventSub.txt), [TestDiagnosticWarnings/CAL011_EventSubscriber_warning](internal/rules/rules_test.go#L143)

<a id="cal012"></a>
### CAL012 — регистр идентификаторов по объявлению

**Исходный код:**

```cal
      globalcounter := inputvaluep;                              // НАРУШЕНИЕ: регистр не совпадает с объявлениями GlobalCounter / InputValueP
      itemledgentryl.SETRANGE("Entry No.", globalcounter);       // НАРУШЕНИЕ: не ItemLedgEntryL
      IF itemledgentryl.FINDFIRST() THEN
        helperproc();                                            // НАРУШЕНИЕ: не HelperProc
```

**Исправленный код:**

```cal
      GlobalCounter := InputValueP;                              // Исправлено: регистр по объявлению
      ItemLedgEntryL.SETRANGE("Entry No.", GlobalCounter);
      IF ItemLedgEntryL.FINDFIRST() THEN
        HelperProc();
```

**Тест:** [input](testdata/cases/cal012_identifier_casing/input/c50012%20-%20Identifier%20Casing.txt) / [expected](testdata/cases/cal012_identifier_casing/expected/c50012%20-%20Identifier%20Casing.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal010-011-033-builtin-conflict"></a>
### CAL010 + CAL011 + CAL033 — конфликт имени переменной с системной функцией (пункт 1.5)

Имя переменной, совпадающее с системной функцией (`Time` vs `TIME()`), переименовывается **только при коллизии**: локальная переменная / Return Value -> `+L`, параметр -> `+P`, глобальная переменная -> `+G`, всегда с `warning`. CAL033 никогда не переводит в UPPERCASE объявленные переменные/параметры/Return Value и левую часть `:=`; ссылки на встроенную функцию в точном UPPERCASE (`WORKDATE`) при переименовании не затрагиваются. Без коллизии суффиксы `L/G` не добавляются, Return Value и глобальные переменные не трогаются.

**Исходный код:**

```cal
    VAR
      WorkDate@1000 : Date;                     // НАРУШЕНИЕ: коллизия с системной функцией WORKDATE() -> переименовать в WorkDateG

    PROCEDURE GetValueAsTime@1(Value@1000 : DateTime) Time : Time;
                                                // НАРУШЕНИЕ: параметр без P; Return Value 'Time' коллизируют с TIME()
    VAR
      Counter@1001 : Integer;                   // НАРУШЕНИЕ: локальная без L (не builtin, обычный путь CAL010)
    BEGIN
      Time := DT2TIME(Value);                   // НАРУШЕНИЕ: до фикса превращалось в "TIME := ..." (CAL033)
      WorkDate := WORKDATE;                     // глобальная ссылка -> WorkDateG; сама WORKDATE — НЕ трогать (builtin)
      Counter := 1;
      MESSAGE('%1 %2', Time, Counter);
    END;
```

**Исправленный код:**

```cal
    VAR
      WorkDateG@1000 : Date;                    // Исправлено: +G из-за коллизии с WORKDATE() (warning)

    PROCEDURE GetValueAsTime@1(ValueP@1000 : DateTime) TimeL : Time;
                                                // Исправлено: Value -> ValueP (CAL011); Return Value Time -> TimeL (коллизия, warning); тип Time НЕ трогать
    VAR
      CounterL@1001 : Integer;                  // Исправлено: обычный суффикс L (без warning коллизии)
    BEGIN
      TimeL := DT2TIME(ValueP);                 // Исправлено: ссылка переименована; TIME := не возникло (CAL033 guard)
      WorkDateG := WORKDATE();                  // Исправлено: глобал -> WorkDateG; builtin WORKDATE сохранён и получил () от CAL020
      CounterL := 1;
      MESSAGE('%1 %2', TimeL, CounterL);
    END;
```

Case с тенью: глобальная `WorkDate` **не** переименовывается (только warning), если локальная/параметр с тем же именем перекрывает её в какой-либо процедуре.

**Тест:** [TestKeywordCasingDeclaredNameGuard](internal/rules/builtin_conflict_test.go#L23), [TestReturnVarBuiltinConflictRename](internal/rules/builtin_conflict_test.go#L99), [TestGlobalBuiltinConflictRename](internal/rules/builtin_conflict_test.go#L143), [TestGlobalBuiltinConflictShadowedWarnsOnly](internal/rules/builtin_conflict_test.go#L199), [TestReturnVarBuiltinConflictCollisionWarnsOnly](internal/rules/builtin_conflict_test.go#L250), golden: [input](testdata/cases/builtin_name_conflict/input/c50024%20-%20Builtin%20Name%20Conflict.txt) / [expected](testdata/cases/builtin_name_conflict/expected/c50024%20-%20Builtin%20Name%20Conflict.txt)

---

## 2. Скобки вызовов (CAL020–CAL021)

<a id="cal020"></a>
### CAL020 — `()` для вызовов без аргументов

**Исходный код:**

```cal
      WorkDateL := TODAY;              // НАРУШЕНИЕ: встроенная функция без ()
      CompanyL := COMPANYNAME;         // НАРУШЕНИЕ: встроенная функция без ()
      DoInternal;                      // НАРУШЕНИЕ: вызов процедуры без ()
      ItemLedgEntryL.RESET;            // НАРУШЕНИЕ: метод Record без ()
      IF ItemLedgEntryL.ISEMPTY THEN   // НАРУШЕНИЕ: метод Record без ()
        ...
        UNTIL ItemLedgEntryL.NEXT = 0; // НАРУШЕНИЕ: метод Record без ()
```

**Исправленный код:**

```cal
      WorkDateL := TODAY();            // Исправлено: добавлено ()
      CompanyL := COMPANYNAME();       // Исправлено: добавлено ()
      DoInternal();                    // Исправлено: добавлено ()
      ItemLedgEntryL.RESET();          // Исправлено: добавлено ()
      IF ItemLedgEntryL.ISEMPTY() THEN // Исправлено: добавлено ()
        ...
        UNTIL ItemLedgEntryL.NEXT() = 0;  // Исправлено: добавлено ()
```

**Тест:** [input](testdata/cases/cal020_call_parens/input/c50020%20-%20Call%20Parens.txt) / [expected](testdata/cases/cal020_call_parens/expected/c50020%20-%20Call%20Parens.txt), `SalesHeader.FINDSET` → `FINDSET()`: [all_rules_codeunit](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal020-exit"></a>
### CAL020 — `EXIT` без пустых скобок

**Исходный код:**

```cal
        EXIT();                        // НАРУШЕНИЕ: у EXIT не должно быть пустых ()
```

**Исправленный код:**

```cal
        EXIT;                          // Исправлено: пустые () убраны; EXIT(val) не меняется
```

**Тест:** [input](testdata/cases/cal020_call_parens/input/c50020%20-%20Call%20Parens.txt) / [expected](testdata/cases/cal020_call_parens/expected/c50020%20-%20Call%20Parens.txt)

<a id="cal021"></a>
### CAL021 — удаление избыточных скобок

**Исходный код:**

```cal
      IF (FlagP) THEN                  // НАРУШЕНИЕ: всё условие в лишних скобках
        TotalL := (QtyP) + (1);        // НАРУШЕНИЕ: атомарные выражения в лишних скобках

      IF ((QtyP > 0) AND (TotalL > 0)) THEN  // НАРУШЕНИЕ: только внешняя пара; внутренние — НЕ трогать (приоритет AND)
        Result := TRUE;

      IF NOT (QtyP = 0) THEN           // НЕ трогать: скобки вокруг сравнения после NOT обязательны
        EXIT((Result));                // НАРУШЕНИЕ: дублирующиеся скобки в аргументе
```

**Исправленный код:**

```cal
      IF FlagP THEN                    // Исправлено: внешние скобки удалены
        TotalL := QtyP + 1;            // Исправлено: скобки вокруг атомов удалены

      IF (QtyP > 0) AND (TotalL > 0) THEN  // Исправлена только внешняя пара, обязательные скобки сравнений сохранены
        Result := TRUE;

      IF NOT (QtyP = 0) THEN           // НЕ тронуто
        EXIT(Result);                  // Исправлено: дубликат скобок удалён
```

**Тест:** [input](testdata/cases/cal021_redundant_parens/input/c50021%20-%20Redundant%20Parens.txt) / [expected](testdata/cases/cal021_redundant_parens/expected/c50021%20-%20Redundant%20Parens.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal021-glue"></a>
### CAL021 — удаление скобок не склеивает токены (пункт 1.1)

При удалении `(`/`)` соседние символы проверяются: если удаление склеило бы два идентификатора/ключевых слова/числа, скобка заменяется на пробел.

**Исходный код:**

```cal
      IF NOT(_SalesHeaderExt.GET(TEXT('Doc No.'))) THEN
                                         // НАРУШЕНИЕ: скобки вокруг атома; при простом удалении NOT слился бы с _SalesHeaderExt
        EXIT(FALSE);
      IF (FlagL)THEN                     // НАРУШЕНИЕ: скобки + отсутствие пробела перед THEN
        EXIT(FALSE);
      Result := (1)AND (2);              // НАРУШЕНИЕ: лишние скобки, склейка 1AND
      IF NOT(Документ) THEN              // НАРУШЕНИЕ: кириллический идентификатор сливается с NOT
        EXIT(FALSE);
```

**Исправленный код:**

```cal
      IF NOT _SalesHeaderExt.GET(TEXT('Doc No.')) THEN
                                         // Исправлено: скобки удалены, вставлен пробел вместо пустой строки
        EXIT(FALSE);
      IF FlagL THEN                      // Исправлено: скобки удалены, пробел перед THEN сохранён склейкой
        EXIT(FALSE);
      Result := 1 AND 2;                 // Исправлено: без склейки токенов
      IF NOT Документ THEN               // Исправлено: пробел между NOT и Документ
        EXIT(FALSE);
```

**Тест:** [TestFixRedundantParensNoTokenGluing](internal/rules/parens_rules_test.go#L34) (10 кейсов, вкл. кириллические идентификаторы, quoted-ident, вызовы), [TestFixRedundantParensOutputLexesSeparately](internal/rules/parens_rules_test.go#L103)

<a id="cal020-cal033-dotnet"></a>
### CAL020 + CAL033 — защита членов DotNet / Automation / OCX (пункт 1.2)

Методы и свойства .NET/Automation/OCX регистрозависимы: CAL033 не меняет регистр членов после `.` у таких получателей (включая цепочки), CAL020 не добавляет `()` к их свойствам. Область видимости переменных (`localScope`) прокидывается в `applyKeywordCasing`.

**Исходный код:**

```cal
      _ServerDocumentFile.delete(_ServerDocumentFileName);  // НЕ трогать: .NET-метод регистрозависим (не DELETE!)
      IF _XmlDoc.Count > 0 THEN                             // НЕ трогать: .NET-свойство, () добавлять нельзя
        ERROR('Xml count is %1', _XmlDoc.Count);
      _XmlDoc.DocumentElement.set_OuterXml(_ServerDocumentFileName);  // НЕ трогать: цепочка корня _XmlDoc (DotNet)
      OutlookApp.Quit;                                      // НЕ трогать: Automation-член, () не добавляем
      ItemLedgEntryL.reset;                                 // НАРУШЕНИЕ: Record-метод без () — скобки НУЖНЫ
      ItemLedgEntryL.deleteall(FALSE);                      // НАРУШЕНИЕ: Record-метод в нижнем регистре
```

**Исправленный код:**

```cal
      _ServerDocumentFile.delete(_ServerDocumentFileNameP); // НЕ тронуто: регистр delete сохранён
      IF _XmlDoc.Count > 0 THEN                             // НЕ тронуто: Count без ()
        ERROR('Xml count is %1', _XmlDoc.Count);
      _XmlDoc.DocumentElement.set_OuterXml(_ServerDocumentFileNameP);  // НЕ тронуто
      OutlookAppL.Quit;                                     // НЕ тронуто: Quit без () (сам OutlookApp переименован CAL010)
      ItemLedgEntryL.RESET();                               // Исправлено: добавлены () и UPPERCASE
      ItemLedgEntryL.DELETEALL(FALSE);                      // Исправлено: UPPERCASE record-метода
```

**Тест:** [TestKeywordCasingDotNetGuard](internal/rules/dotnet_guard_test.go#L25), [TestCallParensDotNetGuard](internal/rules/dotnet_guard_test.go#L83), [TestChainRootVarAndInterop](internal/rules/dotnet_guard_test.go#L138), golden: [input](testdata/cases/dotnet_member_guard/input/c50022%20-%20DotNet%20Guard.txt) / [expected](testdata/cases/dotnet_member_guard/expected/c50022%20-%20DotNet%20Guard.txt)

<a id="cal003-cal011-nav2017"></a>
### CAL003 + CAL011 — события NAV 2017 `[Integration]` / `[Business]` (пункт 1.4)

Издатели событий распознаются по атрибутам `Integration`, `Business`, `IntegrationEvent`, `BusinessEvent` (без учёта регистра, с аргументами и без) и **полностью исключены** из `CAL003` и `CAL011`. Для `[EventSubscriber]`: неиспользуемый параметр даёт `warning` CAL003 (без удаления), параметры не переименовываются — только `warning` CAL011.

**Исходный код:**

```cal
    [Integration]                                    // издатель NAV 2017
    LOCAL PROCEDURE OnAfterCalcDiscount@10(DiscountGroup@1000 : Code[20];DiscValue@1001 : Decimal);
                                                     // НЕ трогать: издатели исключены из CAL003/CAL011, хоть параметры и без P и не используются
    BEGIN
    END;

    [Business(Scope='Instance', IntegrationKey='SaleKey')]   // издатель с параметрами атрибута, любой регистр
    LOCAL PROCEDURE OnScopedSale@13(BizData@1000 : Record 36);   // НЕ трогать: исключён
    BEGIN
    END;

    [EventSubscriber(Codeunit,50023,OnAfterCalcDiscount)]
    LOCAL PROCEDURE HandleDiscount@14(UsedDocNo@1000 : Code[20];UnusedSub@1001 : Integer);
                                                     // НЕ переименовывать: только warning; по UnusedSub — warning CAL003
    BEGIN
      MESSAGE('%1', UsedDocNo);
    END;

    PROCEDURE RegularProc@1(Amount@1000 : Decimal);  // НАРУШЕНИЕ: обычный параметр без суффикса P
    VAR
      TotalL@1001 : Decimal;
    BEGIN
      TotalL := Amount;
    END;
```

**Исправленный код:**

```cal
    [Integration]
    LOCAL PROCEDURE OnAfterCalcDiscount@10(DiscountGroup@1000 : Code[20];DiscValue@1001 : Decimal);
                                                     // без изменений, диагностик нет
    ...
    [EventSubscriber(Codeunit,50023,OnAfterCalcDiscount)]
    LOCAL PROCEDURE HandleDiscount@14(UsedDocNo@1000 : Code[20];UnusedSub@1001 : Integer);
                                                     // без изменений (только CAL003/CAL011 warning)
    ...
    PROCEDURE RegularProc@1(AmountP@1000 : Decimal); // Исправлено: добавлен суффикс P
    VAR
      TotalL@1001 : Decimal;
    BEGIN
      TotalL := AmountP;
    END;
```

**Тест:** [TestProcDeclEventAttributes](internal/cal/events_test.go#L8), [TestNAV2017EventPublisherExclusion](internal/rules/events_rules_test.go#L65), golden: [input](testdata/cases/nav2017_events/input/c50023%20-%20NAV2017%20Events.txt) / [expected](testdata/cases/nav2017_events/expected/c50023%20-%20NAV2017%20Events.txt)

---

## 3. Форматирование (CAL030–CAL034)

<a id="cal030"></a>
### CAL030 — отступы

Тело на `+2` от открывающей конструкции, `VAR`-секции и вложенные блоки (`REPEAT`/`UNTIL`) по уровню.

**Исходный код:**

```cal
    procedure ProcessEntries@1(...) Result : Decimal;
    begin
      with itemledgentry do begin
        if findset then
        begin
            repeat                       // НАРУШЕНИЕ: смещение в 4 пробела вместо уровня
              totalqty:=totalqty+Quantity;   // НАРУШЕНИЕ: лишние смещения и отсутствующий уровень +2
            until next=0;                // НАРУШЕНИЕ: не выровнен с repeat
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
        REPEAT                           // Исправлено: уровень +2
          TotalQtyL := TotalQtyL + ItemLedgEntryL.Quantity;  // Исправлено: уровень +2 от REPEAT
        UNTIL ItemLedgEntryL.NEXT() = 0; // Исправлено: UNTIL выровнен под REPEAT
      END;
```

**Тест:** [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt) / [expected](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt), [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal031-single"></a>
### CAL031 — однострочный `IF`: `BEGIN` на той же строке

**Исходный код:**

```cal
      if TotalL>0 then
      begin                              // НАРУШЕНИЕ: BEGIN на отдельной строке при однострочном условии
          TotalL:=TotalL+1;
      end;
```

**Исправленный код:**

```cal
      IF TotalL > 0 THEN BEGIN           // Исправлено: BEGIN присоединён к строке THEN
        TotalL := TotalL + 1;
      END;
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal031-multi"></a>
### CAL031 — многострочный `IF`: `THEN` под `IF`, `BEGIN` отдельной строкой

**Исходный код:**

```cal
      if (QtyP>0) and
         (PriceP>0) then begin           // НАРУШЕНИЕ: BEGIN на строке с THEN при многострочном условии
        TotalL:=TotalL+PriceP;
      end;
```

**Исправленный код:**

```cal
      IF (QtyP > 0) AND                  // продолжение условия выровнено под первым выражением
         (PriceP > 0) THEN               // THEN на отдельной строке под IF
      BEGIN                              // Исправлено: BEGIN отдельной строкой под THEN
        TotalL := TotalL + PriceP;
      END;
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal032"></a>
### CAL032 — пробелы вокруг операторов и запятых

**Исходный код:**

```cal
      TotalL:=QtyP*PriceP;               // НАРУШЕНИЕ: нет пробелов вокруг := и *
      MESSAGE( '%1, %2' ,TotalL ,IdxL ); // НАРУШЕНИЕ: пробелы внутри скобок и перед запятыми
```

**Исправленный код:**

```cal
      TotalL := QtyP * PriceP;           // Исправлено: пробелы вокруг операторов
      MESSAGE('%1, %2', TotalL, IdxL);   // Исправлено: пробелы после запятых, убраны внутри скобок
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), `Factor:=2` → `FactorL := 2`: [input](testdata/cases/all_rules_report/input/r50102%20-%20Custom%20Report.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

<a id="cal033"></a>
### CAL033 — ключевые слова в UPPERCASE

**Исходный код:**

```cal
    local procedure FormatDemo@1(QtyP@1000 : Decimal;PriceP@1001 : Decimal);
                                                     // НАРУШЕНИЕ: ключевые слова lower-case
    var
    begin
      if (QtyP>0) and (PriceP>0) then    // НАРУШЕНИЕ: if/and/then в нижнем регистре
      if confirm(confirmmsg,false,companyname) then
                                                     // НАРУШЕНИЕ: confirm/companyname — встроенные функции;
                                                     // НЕ трогать: QtyP/confirmmsg — переменные, регистр по объявлению
```

**Исправленный код:**

```cal
    LOCAL PROCEDURE FormatDemo@1(QtyP@1000 : Decimal;PriceP@1001 : Decimal);
                                                     // Исправлено: ключевые слова в UPPERCASE
    VAR
    BEGIN
      IF (QtyP > 0) AND (PriceP > 0) THEN
      IF CONFIRM(ConfirmMsg, FALSE, COMPANYNAME()) THEN
                                                     // Исправлено: встроенные функции в UPPERCASE;
                                                     // ConfirmMsg не изменён — это переменная (CAL012/CAL004)
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt), [TestGoldenCases](internal/rules/rules_test.go#L24); защита .NET-членов: [TestKeywordCasingDotNetGuard](internal/rules/dotnet_guard_test.go#L25)

<a id="cal034"></a>
### CAL034 — двойные `;;` и лишние пустые строки

**Исходный код:**

```cal
      usedglobal:=1;;                    // НАРУШЕНИЕ: двойная точка с запятой


                                         // НАРУШЕНИЕ: два подряд идущих пустых ряда
      TotalL:=QtyP*PriceP;;              // НАРУШЕНИЕ: двойная точка с запятой
```

**Исправленный код:**

```cal
      UsedGlobal := 1;                   // Исправлено: одинарный ;

      TotalL := QtyP * PriceP;           // Исправлено: одинарный ;, пустые строки схлопнуты до одной
```

**Тест:** [input](testdata/cases/cal030_034_formatting/input/c50030%20-%20Formatting%20Guidelines.txt) / [expected](testdata/cases/cal030_034_formatting/expected/c50030%20-%20Formatting%20Guidelines.txt), [TestGoldenCases](internal/rules/rules_test.go#L24)

---

## 4. Комбинированные и режимные тесты

<a id="combined-codeunit"></a>
### Все правила сразу (Codeunit целиком)

**Исходный код:**

```cal
    procedure ProcessEntries@1(ItemNo@1000 : Code[20];PostingDate@1001 : Date) Result : Decimal;
                                                     // НАРУШЕНИЯ: CAL011 (нет P), CAL033 (procedure), CAL003 (UnusedParam — только warning)
    begin
      with itemledgentry do begin        // НАРУШЕНИЕ: CAL005
        setrange("Item No.",itemno);     // НАРУШЕНИЕ: CAL005/CAL012/CAL033
        if findset then                  // НАРУШЕНИЕ: CAL020 (нет ()), CAL031, CAL033
        begin
            repeat                       // НАРУШЕНИЕ: CAL030
              totalqty:=totalqty+Quantity;
            until next=0;
        end;
      end;
    end;
```

**Исправленный код:**

```cal
    PROCEDURE ProcessEntries@1(ItemNoP@1000 : Code[20];PostingDateP@1001 : Date) Result : Decimal;
                                                     // Исправлено: P-суффиксы, ключевые слова; UnusedParam → только warning
    VAR
      ItemLedgEntryL@1003 : Record 32;   // Исправлено: L-суффикс (CAL010)
      TotalQtyL@1006 : Decimal;
    BEGIN
      IF ItemLedgEntryL.FINDSET() THEN BEGIN
        REPEAT                           // Исправлено: CAL005 (WITH удалена), CAL030/031/032/033
          TotalQtyL := TotalQtyL + ItemLedgEntryL.Quantity;
        UNTIL ItemLedgEntryL.NEXT() = 0;
      END;
    END;
```

**Тест:** [TestGoldenCases](internal/rules/rules_test.go#L24), [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt) / [expected](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt)

<a id="combined-nested"></a>
### Table / Report / XMLport — вложенные триггеры

`OnValidate` в поле таблицы, `OnAfterGetRecord` в DataItem отчёта, `OnPreXMLport` XMLport — все правила применяются к телам вложенных триггеров.

**Исходный код:**

```cal
    OnValidate=VAR
                 ItemLedg@1000 : Record 32;          // НАРУШЕНИЕ: CAL010 (нет L)
               BEGIN
                 with itemledg do begin              // НАРУШЕНИЕ: CAL005
                     setrange("Item No.",Rec."Item No.");   // НАРУШЕНИЕ: CAL033
                     if findfirst then               // НАРУШЕНИЕ: CAL020 (нет ())
                       Rec.Quantity:=Quantity;       // Quantity без префикса — CAL005
                 end;
               END;
```

**Исправленный код:**

```cal
    OnValidate=VAR
                 ItemLedgL@1000 : Record 32;         // Исправлено: суффикс L
               BEGIN
                 ItemLedgL.SETRANGE("Item No.", Rec."Item No.");  // Исправлено: WITH удалена, UPPERCASE
                 IF ItemLedgL.FINDFIRST() THEN       // Исправлено: () добавлены
                   Rec.Quantity := ItemLedgL.Quantity;  // Исправлено: поле квалифицировано
               END;
```

**Тест:** [TestGoldenCases](internal/rules/rules_test.go#L24), [input](testdata/cases/all_rules_table/input/t50101%20-%20Custom%20Entry.txt) / [expected](testdata/cases/all_rules_table/expected/t50101%20-%20Custom%20Entry.txt), [input](testdata/cases/all_rules_report/input/r50102%20-%20Custom%20Report.txt), [input](testdata/cases/all_rules_xmlport/input/x50103%20-%20Custom%20Export.txt)

<a id="config-toggle"></a>
### Отключение правил в конфиге

**Исходный код (CAL005/CAL010/CAL020 выключены, остальные работают):**

```cal
      WITH ItemLedgEntry DO BEGIN        // НЕ трогать: CAL005 выключён в конфиге
        TotalQty := TODAY;               // НЕ трогать: TotalQty без L (CAL010 выключён), TODAY без () (CAL020 выключён)
```

**Результат:** `WITH` сохранена, `TotalQty` не переименована, `TODAY` без `()`; при этом включённые правила сработали: `ItemNo` → `ItemNoP` (CAL011), `ConfirmMsg` → в глобальный `VAR` (CAL004).

**Тест:** [TestConfigToggleDisablesSpecificRules](internal/rules/rules_test.go#L175), [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt)

<a id="diff-limit"></a>
### Diff-режим: `MaxOldFunctionLines=10`

Неизменённые строки старых процедур не трогаются; изменённая строка внутри старого `WITH` форматируется по anchored-отступам, но `WITH` не раскрывается.

**Исходный код (фрагмент, новые строки помечены):**

```cal
        with oldrec do begin             // СТАРАЯ неизменённая строка: НЕ трогать (и WITH не раскрывать)
          setrange("Item No.",'OLD');    // СТАРАЯ: НЕ трогать
          NewFlagL := (TODAY() <> 0D);   // НОВАЯ строка внутри старого WITH: форматируется (CAL020/032/033)
        end;
```

**Результат:** старая строка `with oldrec do begin` сохранена как есть; новая строка отформатирована по anchored-отступам (вариант A).

**Тест:** [TestDiffModeLimitAndVariantAIndentation](internal/rules/diff_mode_test.go#L254)

<a id="diff-unlimited"></a>
### Diff-режим: `MaxOldFunctionLines=0` — изменённая старая процедура чистится целиком

**Исходный код:**

```cal
    procedure LargeOldProc@3(OldParam@1000 : Integer);   // НАРУШЕНИЯ в изменённой процедуре: CAL011, CAL033
    begin
      with oldrec do begin               // НАРУШЕНИЕ: CAL005 (процедура чистится целиком)
        setrange("Item No.",'OLD');
      end;
```

**Исправленный код:**

```cal
    PROCEDURE LargeOldProc@3(OldParamP@1000 : Integer);  // Исправлено: P-суффикс, ключевые слова
    begin
      OldRecL.SETRANGE("Item No.", 'OLD');  // Исправлено: WITH раскрыта, неиспользуемые переменные удалены
```

Нетронутые процедуры (например, `UntouchedLegacyProc` с `dirtyvar:=oldarg+1;;`) остаются без изменений байт в байт.

**Тест:** [TestDiffModeUnlimitedZeroTransformsLargeOldFunctionWholly](internal/rules/diff_mode_test.go#L279)

<a id="idempotency"></a>
### Идемпотентность и побайтовая сохранность

Повторный прогон по исправленному коду = 0 изменений; при всех выключенных правилах вход = выход байт в байт; CRLF сохраняется.

**Тест:** [TestGoldenCases, шаги 3–5](internal/rules/rules_test.go#L84), [input](testdata/cases/all_rules_codeunit/input/c50100%20-%20All%20Rules%20Combined.txt) / [expected](testdata/cases/all_rules_codeunit/expected/c50100%20-%20All%20Rules%20Combined.txt)

---

## 5. Инфраструктура (не правила)

| Аспект | Что проверяется | Тест |
|---|---|---|
| Лексер | Lossless round-trip CRLF, комментарии, строки с `''`, quoted-ident, `0D` | [TestLexRoundTrip](internal/cal/parser_test.go#L5) |
| Парсер секций | `VAR` / `PROCEDURE` / `[EventSubscriber]` | [TestParseCodeSection](internal/cal/parser_test.go#L20), [TestProcDeclEventAttributes](internal/cal/events_test.go#L8) |
| CLI | Режимы fix/check/diff | [TestCLIFixCheckAndDiff](internal/cli/cli_test.go#L11) |
| Git-режимы | Три режима диффа | [TestGitWorkflowsThreeModes](internal/cli/git_workflow_test.go#L39) |
