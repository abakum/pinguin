# AGENTS.md

## Проверки

Для проверки сборки использовать `make win` (пересоздаёт versioninfo.json, resource_windows_amd64.syso и собирает pinguin.exe). Перед ним: `go vet ./...` и `go mod tidy`.
