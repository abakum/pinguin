# План: pinguin_token / pinguin_ids через среду, .env и Windows Credential Manager

## Цель
Убрать передачу параметров командной строкой и переменную `TOKEN`:
- токен и peer ID берутся из переменных среды `pinguin_token` и `pinguin_ids`;
- если их нет в среде — из файла `.env` в рабочей директории (github.com/joho/godotenv);
- если снова нет — на Windows из Windows Credential Manager (имена `pinguin_token`, `pinguin_ids`).

## Изменения

1. **go.mod**: `go get github.com/joho/godotenv` и `go get github.com/danieljoos/wincred`.

2. **Новый файл `env.go`** (или логика в global.go):
   - `func envValue(key string) string`:
     1. `v := os.Getenv(key)`; если непусто — вернуть;
     2. `_ = godotenv.Load()` (один раз, например в main до чтения), затем снова `os.Getenv(key)`; godotenv не перезаписывает уже установленные переменные, поэтому можно просто вызвать `Load()` в начале main и далее читать `os.Getenv`;
     3. на Windows (`runtime.GOOS == "windows"` + build-тег или просто вызов через файл `creds_windows.go` с тегом `//go:build windows` и заглушку `creds_other.go` с `//go:build !windows`): `wincred.GetGeneric("pinguin_token" / "pinguin_ids")`, вернуть `string(cred.CredentialBlob)`;
     4. иначе пустая строка.
   - Разбор `pinguin_ids`: разделители — пробел и/или запятая (`strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })`), каждый элемент через `strconv.Atoi`, нечисловые пропускаются (как сейчас).

3. **global.go — `NewAAA()`**: вместо `os.Args[1:]` парсить `envValue("pinguin_ids")` (срез по пробелам/запятым). Сигнатуру оставить без изменений.

4. **main.go**:
   - в начале `main()`: `_ = godotenv.Load()`;
   - `bot, err = CreateBot(envValue("pinguin_token"))` вместо `os.Getenv("TOKEN")`;
   - сообщение Usage (main.go:48-53) переписать: вместо «Usage: %s AllowedPeerID1 ...» — указать, что нужно задать `pinguin_ids` (и `pinguin_token`) в среде, `.env` или Windows Credential Manager. Тексты en/ru.

5. **vk.go:17**: текст ошибки `set TOKEN=VK_COMMUNITY_TOKEN` → «set pinguin_token=VK_COMMUNITY_TOKEN».

6. **Удалить `run.cmd`** полностью — источник настроек теперь среда / `.env` / Windows Credential Manager, сценарий запуска больше не нужен.

7. **Новый файл `.env` в корне репо** (пример, коммитим):
   ```
   pinguin_token=VK_COMMUNITY_TOKEN
   pinguin_ids=123456789 2000000001
   ```
   Именно это имя грузит `godotenv.Load()`, файл работает сразу после правки значений.

8. **README.md**:
   - пункт 4 (получение токена): «скопируйте его в `run.cmd`» → «в `.env` (переменная `pinguin_token`) или Windows Credential Manager»;
   - пункт 6 (чат сообщества): peer ID указывать в `pinguin_ids`;
   - Usage (README.md:13-21): `pinguin_ids` и `pinguin_token` вместо `TOKEN` и аргументов, перечислить порядок источников: среда → `.env` → Windows Credential Manager (только Windows);
   - раздел «Запуск на Windows» (README.md:23-41): переписать — убрать листинг `run.cmd` и упоминание сборки; описать только источники настроек: пример `.env` в корне репо; для Windows — как положить значения в Credential Manager (generic-записи `pinguin_token`, `pinguin_ids`): через GUI (Панель управления → Диспетчер учётных данных → «Общие учётные данные» → «Добавить общие учётные данные», адрес/имя — `pinguin_token`, пароль — значение) или через `windows_credential.bat` из корня репо;

9. **Новый файл `windows_credential.bat` в корне репо** — пример CLI-команды для Windows Credential Manager:
   ```bat
   @echo off
   rem Example: store pinguin settings in Windows Credential Manager
   cmdkey /generic:pinguin_token /pass:VK_COMMUNITY_TOKEN
   cmdkey /generic:pinguin_ids   /pass:"123456789 2000000001"
   ```
   - имена записей (`/generic:`) совпадают с ключами `wincred.GetGeneric`;
   - `cmdkey` сохраняет запись в хранилище текущего пользователя Windows (параметр `/user:` не нужен — `wincred.GetGeneric` ищет только по имени из `/generic:`);
   - значения с пробелами брать в кавычки (`/pass:"..."`);
   - `.gitignore` содержит `*.bat` — удалить эту строку (или заменить на конкретное исключение), чтобы `windows_credential.bat` попал в репо; `run.cmd` удаляется явно через `git rm`, поэтому правило `*.bat` больше ни на что не влияет.
   - упомянуть риск: `.env` с реальным токеном не коммитить (в репо только пример с заглушками).

## Порядок источников (документировать в коде кратко в envValue)
среда → `.env` → Windows Credential Manager (только windows).

## Риски
- `.env` коммитится в репо с заглушками; при редактировании на реальный токен есть риск случайно закоммитить секрет. В README предупредить. `.gitignore` не трогаем, иначе пример не попадёт в репо.
- `run.cmd` удалён — у пользователей, запускавших через него, ничего не сломается молча: без переменных бот напечатает понятное сообщение об использовании.
- wincred работает только на windows — изолировать build-тегами, чтобы linux-сборка не ломалась.

## Валидация
- `go vet ./... && go build ./...` (на linux — проверка, что windows-файл не мешает);
- `GOOS=windows go build` — сборка для windows;
- `git rm run.cmd` — убедиться, что ссылок на run.cmd в репо не осталось (grep);
- запуск с `pinguin_ids`/`pinguin_token` в среде, затем только через `.env` из примера — оба варианта работают одинаково;
- запуск без ничего — ожидается сообщение об использовании; при пустом `pinguin_token` CreateBot вернёт понятную ошибку «set pinguin_token=VK_COMMUNITY_TOKEN».
