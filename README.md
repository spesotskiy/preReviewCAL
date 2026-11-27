<img src="assets/icon.svg" width="64" height="64" alt="">

# PreReviewCAL
## Каталог правил (с кодами для конфига и тестов)
Каждое правило можно включать/выключать и настраивать в `.prereviewcal.json`.

### Переменные и область видимости
| Код | Название | Поведение |
| --- | --- | --- |
| **CAL001** | `unused-local-var` | Удаление неиспользуемых локальных переменных (и пустой секции `VAR`). |
| **CAL002** | `unused-global-var` | Удаление неиспользуемых глобальных переменных (с проверкой `PROPERTIES`, `FIELDS`, `CONTROLS`, `DATASET`, `REQUESTPAGE`; пропуск `WITHEVENTS`/`INDATASET`). |
| **CAL003** | `unused-param` | Только предупреждение (diagnostic) о неиспользуемом параметре процедуры. |
| **CAL004** | `local-textconst` | Запрет локальных `TextConst`: автоперенос в глобальный `VAR` (если нет конфликта имён; при конфликте — предупреждение). |
| **CAL005** | `no-with` | Запрет `WITH ... DO`: удаление конструкции `WITH` (включая вложенные) с подстановкой `VarName.` перед полями и методами записи. |

### Именование
| Код | Название | Поведение |
| --- | --- | --- |
| **CAL010** | `local-var-suffix` | Суффикс `L` для локальных переменных (`Cust` -> `CustL`). **Исключения:** `Return Value`, `TextConst` (обрабатывается `CAL004`). Проверка лимита 30 символов и коллизий (при нарушении — предупреждение). |
| **CAL011** | `param-suffix` | Суффикс `P` для параметров процедур (`DocNo` -> `DocNoP`). **Исключения:** системные триггеры NAV и процедуры с `[EventSubscriber]`/`[IntegrationEvent]`/`[BusinessEvent]` — для `[EventSubscriber]` выдаётся **предупреждение без автоправки**. |
| **CAL012** | `identifier-casing` | Приведение регистра обращений к переменным/процедурам к регистру их объявления. |

### Скобки и вызовы
| Код | Название | Поведение |
| --- | --- | --- |
| **CAL020** | `call-parens` | Добавление `()` к вызовам процедур и встроенных функций без параметров (включая `TODAY()`, `COMPANYNAME()`, `FINDSET()`, `INIT()`, `INSERT()`, `MODIFY()` и др.). `EXIT` остаётся без скобок (`EXIT;`), а `EXIT()` исправляется на `EXIT`. |
| **CAL021** | `redundant-parens` | Удаление лишних скобок (`((A = 1))` -> `(A = 1)`, `IF (FlagL) THEN` -> `IF FlagL THEN`) со строгим сохранением обязательных скобок вокруг сравнений при `AND`/`OR`. |

### Форматирование (C/AL Coding Guidelines)
| Код | Название | Поведение |
| --- | --- | --- |
| **CAL030** | `indentation` | Отступы по 2 пробела (с сохранением базового отступа C/SIDE для встроенных триггеров и `CODE`), выравнивание `IF/THEN/ELSE`, `CASE/OF`, `REPEAT/UNTIL`, `FOR`, `WHILE`. |
| **CAL031** | `begin-placement` | Однострочное условие: `IF CondL THEN BEGIN`; **многострочное условие:** `BEGIN` на следующей строке ровно под `IF`. |
| **CAL032** | `spacing` | Пробелы вокруг бинарных операторов (`:=`, `=`, `<>`, `+`, `-`, `*`, `/`, `AND`, `OR`), пробел после `,`, без пробелов перед `,`/`;`, без лишних пробелов внутри `(...)`/`[...]` и вокруг `..`, удаление trailing spaces. |
| **CAL033** | `keyword-casing` | Ключевые слова и булевы константы в `UPPERCASE` (`IF`, `THEN`, `ELSE`, `BEGIN`, `END`, `EXIT`, `TRUE`, `FALSE`, `DIV`, `MOD`, `IN` и т. д.). |
| **CAL034** | `blank-lines-and-semicolons` | Удаление лишних пустых строк (не более 1 подряд) и пустых `;` (`;;`). |

## Команды
### Сборка бинарника
```powershell
go build -o prereviewcal.exe ./cmd/prereviewcal
```
### Обновить файл или папку на месте
```powershell
.\prereviewcal.exe fix "path\to\c50000 - My Object.txt"
.\prereviewcal.exe fix ./objects/
```
### Посмотреть diff без изменения файла
```powershell
.\prereviewcal.exe fix --diff --symbols-dir "path\to\objects" "path\to\c50000 - My Object.txt"
```
### Проверка без изменений (exit code 1 при нарушениях/предупреждениях)
```powershell
.\prereviewcal.exe check "path\to\objects"
```
### Своя конфигурация правил
```powershell
.\prereviewcal.exe fix --config .prereviewcal.json "./objects/"
```
### Режимы
1. Внутри разработки (до коммита): staged против HEAD
```powershell
.\prereviewcal.exe fix --staged
.\prereviewcal.exe check --staged
```
2. Преревью: полировка локальной ветки относительно базовой ветки (например origin/main)
```powershell
.\prereviewcal.exe fix --git-base origin/main
.\prereviewcal.exe fix --git-base origin/main --max-old-func-lines 0   # править затронутые старые функции любого размера
```
3. CI/CD (Merge Request): только проверка двух коммитов без изменения файлов (exit code 1 при нарушениях)
```powershell
.\prereviewcal.exe check --git-base origin/main --git-head HEAD --diff
```

Created in [T3 Code](https://t3.codes).
