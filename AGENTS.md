# AGENTS.md

## Проверки

Для проверки сборки использовать `make win` (пересоздаёт versioninfo.json, resource_windows_amd64.syso и собирает pinguin.exe). Перед ним: `go vet ./...` и `go mod tidy`.

## Worktrees

После использования git worktrees подчищать за собой: удалять worktree (`git worktree remove`) и его ветку, если она не нужна (`git branch -d`), не оставлять висящих каталогов и веток.
