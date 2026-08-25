# План: перенос бота tmbPi с Telegram на VK

## Источник для переноса

**`/home/koka/src/tmbPi/``** — эталонный репозиторий, оттуда переносится вся функциональность:

| Файл-источник | Роль при переносе |
|---|---|
| `/home/koka/src/tmbPi/main.go` | обработчики, long polling, команды — переписать под VK |
| `/home/koka/src/tmbPi/worker.go` | воркер пинга и статусные сообщения — переписать под VK |
| `/home/koka/src/tmbPi/global.go` | структура `customer`, `sCustomer`, `ikbs`, `AAA` — адаптировать |
| `/home/koka/src/tmbPi/telego.go` | хелперы/предикаты — заменить новым `vk.go` |
| `/home/koka/src/tmbPi/proxy.go` | прокси — НЕ переносить (VK не требует) |
| `/home/koka/src/tmbPi/LunarAnniversaries.go` | Easter Egg — НЕ переносить |
| `/home/koka/src/tmbPi/{log,json,util,ping}.go` | скопировать практически без изменений (проверить импорты) |
| `/home/koka/src/tmbPi/Makefile`, `README.md` | адаптировать |

При переносе сверять поведение и payload-набор кнопок с кодом в `/home/koka/src/tmbPi/` — он является единственным источником истины по функциональности.

## Решения (согласованы с пользователем)

1. **Полная замена** на VK, `telego` удаляется. Параллельная работа TG+VK не нужна.
2. **Состояние пингов сбрасывается** — старый `tmbPing.json` не мигрируется, начинается с чистого файла. Формат хранения становится платформонезависимым (без сериализации объектов сообщений).
3. **Кнопки и эмодзи переносятся**, ссылки — нет:
   - Inline-клавиатура VK с callback-кнопками (`action.type="callback"`, `payload`).
   - Кнопка «…» как *переключатель видимости* групповых кнопок удаляется, но **функциональность групповых команд с payload-префиксом «…» полностью сохраняется** (набор `ikbs` из global.go:44-54): все кнопки видны сразу.
   - Раскладка — 3 ряда:
     - Ряд 1 (одиночный IP): `🔁` `🔂` `⏸️` `❌` `❎` (payload = сам эмодзи, как в worker.go).
     - Ряд 2 (команда всем IP): `…🔁` `…🔂` `…⏸️` `…❌` (payload = `…`+эмодзи).
     - Ряд 3 (завершить пинги со статусом): `…✅❌` `…❗❌` `…⏸️❌` (payload как в global.go).
   - Лейблы групповых кнопок — с префиксом «…» (визуально отличать от одиночных). Payload'ы менять нельзя — на них завязаны `bhAnyCallbackQueryWithMessage` и worker.
   - Логика `ikbsf` (усечённая клавиатура для неразрешённых) удаляется: клавиатура одна для всех, доступ по-прежнему проверяется при обработке callback'а.
4. **Easter Egg (⚡ deep-link `/start base64`, spoiler, share-ссылка) выпиливается** — прямого аналога в VK нет. `LunarAnniversaries.go` и обработчик `bhEasterEgg` удалить.
5. **Прокси больше не нужен** — `proxy.go` удалить, `CreateBotWithProxy` заменить созданием VK-клиента.

## Известные ограничения VK, влияющие на архитектуру

- **Нет редактирования сообщений ботом.** Обновление статуса в worker.go уже идёт через delete+send — этого достаточно. Ветка `EditMessageReplyMarkup` (переключение клавиатуры по «…») удаляется за ненадобностью.
- **Callback-событие** — VK Bots Long Poll, событие `message_event` (`MessageEventAnswer` вместо `AnswerCallbackQuery`).
- **Форматирование**: без entities/spoiler; plain-text + эмодзи. IP можно писать моношифтово через `«…»` или как есть.
- **Вход/выход участников**: события `chat_invite_user` / `chat_kick_user` (в VK-чате это одно событие с action).
- **Reply**: `reply_to` в `messages.send` вместо `ReplyParameters`.
- Бот — **групповой** (community token), long polling через `github.com/SevereCloud/vksdk/v3` (v3, `api.VK`, `longpoll-user`/`lpbots` wrapper — использовать Bots Long Poll).

## Соответствие обработчиков

| Telegram | VK замена |
|---|---|
| `UpdatesViaLongPolling` + `th.BotHandler` | `vksdk` Bots Long Poll, диспетчер событий `message_new`, `message_event`, `message_allow/deny` и др. |
| `bhAnyCallbackQueryWithMessage` | обработчик `message_event`; payload = старый `CallbackQuery.Data` |
| `bhReplyMessageIsMinus` (reply «-» → delete) | `message_new` + проверка reply + текст "-" |
| `bhAnyWithMatch` (reIP) | `message_new` + тот же regexp |
| `bhAnyCommand` (`/start`, `/restart`, `/stop`) | `message_new`, текст команд без изменений; `/start base64` ветку удалить |
| `bhLeftChat` / `bhNewMember` | `message_new` с `action` kick/invite |
| `SendError` | `messages.send` первому разрешённому peer |

## Задачи

1. **go.mod**: добавить `github.com/SevereCloud/vksdk/v3`, убрать `telego`, `jibber_jabber` оставить (язык детектится так же), `closer` оставить.
2. **Новый `vk.go`** (замена `telego.go` + `proxy.go`):
   - `CreateBot(token)` → `*vksdk.VK` из `TOKEN` (токен сообщества).
   - Хелперы: `sendKeyboard(peerID, replyTo, text, rows [][]btn)`, `deleteMessage`, `answerEvent`.
   - Конструктор клавиатуры — 3 ряда (см. «Решения» п.3): ряд 1 без префикса, ряды 2-3 с payload-префиксом «…». Один набор для всех сообщений бота (статусные, «список IP ожидается»), `inline:true`.
   - `SendError(vk, err)`.
3. **`global.go`**: заменить тип `bot *tg.Bot` на VK-клиент; `chats []int64` → peer IDs (отрицательные для чатов/групп, положительные для ЛС); `customer.Tm *tg.Message` → платформонезависимая структура `{PeerID, UserID, MsgID int64}` (+ поля `Status string`, `Deadline int64` для save/load).
4. **`main.go`**:
   - Убрать Easter Egg (`bhEasterEgg`, `reYYYYMMDD`-хендлер), deep-link, `start()`, `me *tg.User`.
   - Переписать `startH`: long poll VK + свой маршрутизатор событий (аналог bh.Handle).
   - `bhAnyCallbackQueryWithMessage`: **сохранить** логику `strings.HasPrefix(Data, "…")` → `ip = ""` и групповые ветки `ips.update(...)`; удалить только ветку `Data == "…"` (EditMessageReplyMarkup); вместо `AnswerCallbackQuery` — `messages.sendMessageEventAnswer`.
   - Проверки доступа: `allowed()`/`notAllowed()` на peer IDs, тексты `dic.add` оставить.
5. **`worker.go`**: удалить `tg.Message`/`tu.*`, использовать новые структуры; локальный `ikbs` worker'а заменить на общую клавиатуру из 3 рядов (без кнопки «…» и без `ikbsf`-логики); **сохранить** default-ветку обработки `…*❌` (групповое завершение по статусу: `tsX`-логику не трогать); статус: delete старого reply + send нового с клавиатурой (как сейчас); save при завершении — новый формат.
6. **`ping.go`, `json.go`, `log.go`, `util.go`, `LunarAnniversaries.go`**: `LunarAnniversaries.go` удалить; в остальных убрать импорты telego (в `ping.go` их нет — проверить).
7. **`proxy.go`** удалить. Прокси-переменные окружения (`TMB_PROXY`, `TMB_URL`, `SOCKS5_PROXY`) больше не читаются.
8. **`README.md`**: обновить — TOKEN теперь токен сообщества VK, аргументы = peer IDs, состояние сбрасывается.
9. **Сборка/проверка**: `go mod tidy && go vet ./... && go build` (Makefile уже собирает; Windows-цель `tmbPi.exe` пересоберётся пользователем).

## Сценарии для проверки после реализации

- ЛС: `/команда` → сообщение об ожидании списка IP + клавиатура.
- Разрешённый пользователь шлёт список IP → статусные сообщения с клавиатурой (3 ряда: 🔁🔂⏸️❌❎ / …🔁…🔂…⏸️…❌ / …✅❌…❗❌…⏸️❌).
- Нажатие ⏸️/🔁/🔂/❌/❎ → одиночная команда для одного IP (❎ = удалить сообщение, ❌ = остановить пинг).
- Нажатие …🔁/…🔂/…⏸️/…❌ → команда применяется ко всем активным IP.
- Нажатие …✅❌/…❗❌/…⏸️❌ → пинги с соответствующим статусом завершаются, их сообщения удаляются.
- Reply «-» на сообщение бота → удаление.
- Неразрешённый пользователь → «Батюшка не благословляет».
- `/stop` от владельца → корректное завершение с сохранением.
- Изменение статуса пинга → старое сообщение удаляется, новое отправляется.

## Риски / открытые вопросы

- Точные имена полей/лимиты клавиатур VK (ширина кнопки, payload ≤ 255 байт — эмодзи-payload проходит) проверить по актуальной документации vksdk v3.
- Если бот используется в VK-чате, peer ID отрицательный — фильтр `chats.allowed` должен принимать оба знака.
