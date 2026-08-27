# Отчёт: «промельки» pinguin.exe на Windows 11 (после UAC-плана)

Дата: 2026-08-27. Исходный план: 20260827-uac-elevation-plan.md.

## Симптом

После внедрения UAC-повышения pinguin.exe запускается на Windows 10, но на Windows 11
«что-то промелькивает» и исчезает.

## Диагностика

1. Запуск из cmd на Win11 → UAC-запрос → **отказ** → «Отказано в доступе.» — ответ cmd
   на отклонённое создание повышенного процесса (ERROR_ELEVATION_REQUIRED).
   Вывод: манифест `requireAdministrator` на Win11 **работает**, elevation не при чём.
2. Запуск из cmd от администратора (`cd c:\Users\KAbak\go\bin && pinguin.exe`):
   ids и pinguin.json находятся, единственная фатальная ошибка —
   `longpoll-bot: api: User authorization failed: invalid access_token (4)` → exit(1).
3. «Промельки» идентифицированы как мгновенные фатальные выходы:
   - без `pinguin.json` — `loader()` (json.go) ReadFile ошибка;
   - с токеном-заглушкой — `lp.NewLongPollCommunity` (валидация токена через API VK).
   Консоль повышенного процесса закрывалась сразу — текст ошибки не был виден.
4. Доп. находка: пути `.env`/`pinguin.json` разрешались от **cwd**, а повышенный
   перезапуск получает cwd=`C:\Windows\System32`.

## Изменения в коде

- log.go: `fatalWait(err)` — печатает `fatal: <ошибка>` и ждёт Enter, только если
  stdout — терминал (`os.ModeCharDevice`); неинтерактивный запуск не блокируется.
  (Первая версия ждала Enter до печати ошибки — вывод глушлся `logOff()`; исправлено.)
- env.go: `exeDir()` (os.Executable, fallback cwd); `.env` ищется рядом с exe.
- main.go: `pinguin.json` привязан к каталогу exe; `fatalWait(err)` перед фатальными
  `return`: нет pinguin_ids, ошибка CreateBot, ошибка startH/longpoll, ошибка loader.
- elevate_windows.go: `fatalWait(nil)` при отказе runas (`ret <= 32`).

## Проверки

- `go vet ./...`, `go mod tidy`, `make win` — успешно; манифест в exe
  (`grep -c requireAdministrator pinguin.exe` → 1).
- На Win11 (пользователь): без данных окно теперь остаётся с «Press Enter>»;
  после исправления fatalWait выводит и текст ошибки.

