# План: повышение прав pinguin.exe на Windows (UAC)

Цель: `pinguin.exe` должен работать с правами администратора, т.к. `ping()` использует
`pinger.SetPrivileged(true)` на Windows (ping.go:17), а raw-сокеты требуют повышения прав.

Решение: ДВА механизма одновременно (по договорённости с пользователем):
1. Манифест `requireAdministrator` — UAC-запрос при запуске exe.
2. Рантайм-проверка `isAdmin()` с самоперезапуском через `runas` (как в
   `../crocson/cmd/I_trust_the_signer_of_this/main.go`) — страховка, если exe каким-то
   образом запустился без прав (например, .syso не попал в сборку, UAC отключён
   политикой и т.п.). Конфликта нет: при рабочем манифесте процесс всегда повышен,
   `runas`-ветка не выполняется никогда.

## Этап 1 — Манифест requireAdministrator

Ключевой факт: `versioninfo.json` генерируется заново при каждой сборке целью
`make versioninfo` (Makefile:55-83), поэтому править только сам JSON-файл бесполезно —
изменения нужно вносить в блок `printf` Makefile.

1. Создать файл `app.manifest` в корне репозитория:

   ```xml
   <?xml version="1.0" encoding="UTF-8" standalone="yes"?>
   <assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
     <assemblyIdentity version="1.0.0.0" processorArchitecture="*" name="pinguin" type="win32"/>
     <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
       <security>
         <requestedPrivileges>
           <requestedExecutionLevel level="requireAdministrator" uiAccess="false"/>
         </requestedPrivileges>
       </security>
     </trustInfo>
   </assembly>
   ```

2. В Makefile в цели `versioninfo` добавить строку в генерируемый JSON (после `"IconPath"`):
   `"ManifestPath": "app.manifest"`. Не забыть запятую после `"IconPath": "$(ICO)"`.
   goversioninfo встроит манифест в `resource_windows_amd64.syso`.

3. Добавить `app.manifest` в зависимость цели `$(SYSO)`, чтобы файл не потерялся:
   `$(SYSO): versioninfo $(ICO) app.manifest`.

## Этап 2 — Рантайм-проверка и самоперезапуск (страховка, НЕ fallback — ничего не убираем)

1. Создать `elevate_windows.go` (`//go:build windows`): перенести из crocson
   `isAdmin()` и `runMeElevated()` (ShellExecuteW, глагол "runas",
   lpDirectory=os.Getwd()). Использовать `golang.org/x/sys/windows` — уже в go.mod
   (indirect, станет direct после `go mod tidy`).
2. Создать `elevate_other.go` (`//go:build !windows`): `func elevateIfNeeded() {}`.
3. В начале `main()` (main.go:19), до создания контекстов: `elevateIfNeeded()` —
   при отсутствии прав печатает "Requesting administrator privileges...",
   запускает копию себя повышенной и делает `os.Exit(0)` (как в crocson main.go:120-123).

Отличие от crocson: там `defer` с "Press Enter to exit..." для интерактивного просмотра;
pinguin — демон с closer.Hold() и логами в файлы, такой defer не нужен и вреден
(блокировал бы неинтерактивный запуск). Просто `os.Exit(0)` после успешного ShellExecuteW.

Защита от бесконечного цикла перезапусков: при отказе пользователя от UAC
`ret <= 32` → печать ошибки и `os.Exit(1)` (как в crocson). Цикл невозможен:
успешный runas либо даёт повышенный процесс (isAdmin=true, ветка не выполняется),
либо завершается кодом >32 с выходом.

## Этап 3 — Сборка и проверки на Linux

1. `go mod tidy` (x/sys/windows станет прямой зависимостью).
2. `go vet ./...`, `go build` — linux-сборка не ломается (elevate_other.go).
3. `make win` — пересоздаёт versioninfo.json, .syso и pinguin.exe.
4. Проверить, что .syso/exe содержит манифест:
   `strings pinguin.exe | grep -i requireAdministrator`.

## Этап 4 — Проверка на реальной Windows

1. Запуск `pinguin.exe` двойным кликом → окно UAC → после подтверждения пинг работает
   без ошибки доступа к сокету.
2. Если UAC-запрос при запуске НЕ появился (манифест не применился), программа должна
   сама перезапуститься повышенно — наблюдаем "Requesting administrator privileges..."
   в исходном окне.
3. Запуск из cmd: повышенный процесс откроется в НОВОМ окне консоли (ограничение UAC).
   Логи пишутся в файлы (log.go), работа демона не пострадает; исходное окно завершится.

## Out of scope

- Переход на unprivileged UDP-ping (`SetPrivileged(false)`).
- Подпись exe, автозапуск через планировщик задач с повышенными правами.
