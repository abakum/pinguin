# Makefile: цель `syso` для Windows-ресурсов

## Контекст
- Makefile уже определяет `VERSION`, `BUILD_TIME`, `GIT_COMMIT`, `LDFLAGS`, цели `install`, `win`, `clean`.
- `pinguin.png` (1024x1024 RGBA) и закоммиченный `pinguin.ico` (7 размеров) уже существуют в корне.
- go.mod: module `github.com/abakum/pinguin`, go 1.25.0, tool-директив пока нет.
- `.gitignore`: `*.exe`, `*.json`, `*.bat`, `pinguin` — `versioninfo.json` и `.syso` (нет в списке; добавить).
- Инструменты (выбраны пользователем): `github.com/Kodeworks/golang-image-ico` (PNG→ICO) и `github.com/josephspurrier/goversioninfo` (versioninfo.json→resource.syso).

## Решения
- Оба инструмента подключаются через `go get -tool` (go 1.24+), вызываются из Makefile как `go -tool github.com/... tool` — без установки в систему и без vendor-скриптов.
- ICO генерируется из `pinguin.png` в нескольких размерах (256/128/64/48/32/16), вывод: `pinguin.ico` (заменяет существующий, становится артефактом).
- `versioninfo.json` генерируется целью Makefile из heredoc-шаблона с подстановкой `$(VERSION)`, `$(GIT_COMMIT)`, `$(BUILD_TIME)`, `$(AUTHOR)` (FileVersion/FileDescription/ProductName/CompanyName и т.д.).
- Автор берётся из git: `AUTHOR:=$(shell git config user.name 2>/dev/null || echo "unknown")` (при желании дополнить email: `git config user.email`). В versioninfo.json идёт в `CompanyName` (и опционально в `LegalCopyright` как `Copyright (c) $(AUTHOR)`).
- `goversioninfo` запускается с `-o windows_amd64.syso` — GOOS/GOARCH-префикс в имени гарантирует, что файл включается только в сборки `GOOS=windows GOARCH=amd64` и не влияет на `make install` и другие платформы.
- Цели: `ico` (PNG→ICO), `syso: ico` (ICO + versioninfo.json → windows_amd64.syso), `win: syso`. `clean` удаляет `pinguin.ico`, `versioninfo.json`, `windows_amd64.syso`. `help` обновить.

## Задачи
1. `go get -tool github.com/Kodeworks/golang-image-ico@latest github.com/josephspurrier/goversioninfo@latest` — появятся tool-директивы в go.mod (+ go.sum).
   - Если `-tool` для ico-библиотеки не сработает (это библиотека, а не main): написать маленький генератор `cmd/mkico/main.go`, который читает `pinguin.png`, масштабирует (golang.org/x/image/draw) и пишет `pinguin.ico` через `ico.Encode`; тогда ico подключается обычным `go get`.
2. Makefile:
   - Переменные: `ICON=pinguin.png`, `ICO=pinguin.ico`, `VERSIONINFO=versioninfo.json`, `SYSO=windows_amd64.syso`, `AUTHOR:=$(shell git config user.name 2>/dev/null || echo "unknown")` (рядом с `GIT_COMMIT`).
   - Цель `ico`: `go tool ...` или `go run ./cmd/mkico` → `$(ICO)`.
       - Цель `syso: $(ICO)`: генерирует `$(VERSIONINFO)` (heredoc с версией/коммитом/автором), затем `go tool github.com/josephspurrier/goversioninfo -o $(SYSO) $(VERSIONINFO)` (goversioninfo по умолчанию читает `versioninfo.json`, ключ `-o` задаёт выходной файл).
    - `win: syso` — существующая сборка GOOS=windows GOARCH=amd64 автоматически включит `$(SYSO)`.
   - `clean`: добавить `rm -f $(ICO) $(VERSIONINFO) $(SYSO)`.
   - `help`: добавить строку `make syso`.
3. `.gitignore`: добавить `*.ico` (существующий `pinguin.ico` станет генерируемым артефактом) и `*.syso` (`*.json` уже покрывает versioninfo.json).
4. Если `pinguin.ico` закоммичен в git — удалить из индекса (`git rm --cached pinguin.ico`), не коммитить (коммит только по запросу пользователя).

## Проверка
- `make syso` — создаются `pinguin.ico` (несколько размеров, `file pinguin.ico`) и `windows_amd64.syso`.
- `make win` — `pinguin.exe` содержит иконку и метаданные: проверить `file pinguin.exe` и в Properties на Windows (или `wrestool`/`python pefile`, если доступно).
- `make clean` — артефакты удаляются; `make win` снова собирается.
- `make install` / `go build` на linux не затронуты: `windows_amd64.syso` имеет GOOS/GOARCH-префикс и игнорируется не-Windows сборками.

## Риски / открытые вопросы
- `golang-image-ico` — библиотека, цель `ico` реализуется через `go run ./cmd/mkico` (fallback согласно задаче 1).
