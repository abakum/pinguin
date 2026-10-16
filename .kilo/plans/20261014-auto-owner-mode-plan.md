# Авто-режим: пустой pinguin_ids → владелец = создатель сообщества, чаты не ограничены

Контекст: сейчас пустой `pinguin_ids` — fatal (main.go:73-79), хотя подписки сами по себе
работают в любом чате (allowlist в `bhAnyWithMatch` нет). Образец — lunarreturns.js:
`initOwner` (стр. 864) берёт `role="creator"` из `groups.getMembers filter=managers`.

## Решения (подтверждены пользователем)

1. `pinguin_ids` задан → поведение не меняется (config-режим, владелец = первый id).
2. `pinguin_ids` пуст → авто-режим:
   - владелец = создатель сообщества (автодетект через API);
   - белый список чатов отключён: приветствие `bhNewMember` и подписки в любом чате;
   - DM-панель — только владельцу (kbOwner), посторонним — plain help;
   - catchUp/cleanupOrphans обходят все диалоги.
3. Неудача детекта → продолжить без владельца (ownerID=0): подписки работают,
   кнопки владельца отвечают notAllowed, 💥/⏹️/▶️ только в лог.
4. Перечисление peers: `messages.getConversations` (пагинация) ∪ PeerID подписок из
   `hosts.list()` (в json отдельного списка чатов нет, но каждая подписка несёт PeerID).
5. Out of scope: `isChatAdmin` из lunarreturns (у pinguin нет функций админов бесед).

## Изменения

### global.go

- Добавить `var ownerID int` (создатель сообщества в авто-режиме; 0 = неизвестен).
- `func isOwner(userID int) bool`: config → `len(chats) > 0 && chats[:1].allowed(userID)`;
  авто → `userID > 0 && userID == ownerID`.
- `func peers() []int`: config → `chats`; авто → `conversations()` ∪ уникальные
  `cu.PeerID` из `hosts.list()`.

### vk.go

- `func detectOwner() (int, error)`: `bot.GroupsGetByID(nil)` → group id (так же
  поступает vksdk `NewLongPollCommunity`, longpoll.go:76); затем
  `bot.GroupsGetMembersFilterManagers(api.Params{"group_id": gid})` (response
  `Items[].Role`, `ID` внутри `UsersUser`); вернуть id с `Role == "creator"`.
- `func initOwner() int`: до 3 попыток `detectOwner` с паузами (~1с, ~5с), лог результата;
  0 при неудаче (не fatal).
- `func conversations() []int`: `bot.MessagesGetConversations(api.Params{"count": 200,
  "offset": N})` пагинацией, собирать `Items[i].Conversation.Peer.ID`; ошибки — лог,
  вернуть собранное.
- `func ownerPeer() int`: `chats[0]` в config-режиме, иначе `ownerID`; 0 = некуда слать.
- `sendStatus`/`SendError` (vk.go:312-334): условие `len(chats) > 0` и `chats[0]` →
  `p := ownerPeer(); if bot != nil && p > 0 { … "peer_id": p … }`.

### main.go

- Стартап (73-84): fatal при пустых ids убрать. Блок перенести ПОСЛЕ `CreateBot`
  (детекту нужен `bot`): `if len(chats) == 0 { ownerID = initOwner(); лог
  "нет pinguin_ids, создатель сообщества: N/not found" } else { прежний лог
  «Разрешённые PeerID» }`. Порядок: CreateBot → детект → loader → cleanupOrphans →
  catchUp → startH.
- main.go:270 (CLI-хендлер MessageReply): `target := chats[0]` → `target := ownerPeer()`;
  при 0 — лог и return.
- main.go:396 и main.go:429 (catchUp, cleanupOrphans): `for _, peer := range chats` →
  `for _, peer := range peers()`.
- main.go:554 и main.go:567 (⏹️/▶️/…-кнопки): `tm.PeerID > 0 && len(chats) > 0 &&
  chats[:1].allowed(obj.UserID)` → `tm.PeerID > 0 && isOwner(obj.UserID)`.
- main.go:700 (/restart, /stop): то же → `isOwner(tm.FromID)`.
- main.go:710-716 (bhAnyCommand DM): условие панели → `chats.allowed(tm.FromID) ||
  isOwner(tm.FromID)`; выбор kbOwner → `isOwner(tm.FromID)`. В авто-режиме посторонним
  в ЛС — plain help (ветка 723).
- main.go:771 (editPanel): `len(chats) > 0 && chats[:1].allowed(userID)` → `isOwner(userID)`.
- main.go:831 (bhNewMember): `if !chats.allowed(tm.PeerID)` → `if len(chats) > 0 &&
  !chats.allowed(tm.PeerID)` (в авто-режиме приветствовать в любом чате).

## Граничные случаи

- config-режим: все пути эквивалентны текущим (`isOwner` ≡ `chats[:1].allowed`,
  `peers()` ≡ `chats`, `ownerPeer()` ≡ `chats[0]`).
- авто + ownerID=0: кнопки владельца → notAllowed (кроме 🔁), SendError/sendStatus
  пропускают отправку, CLI-хендлер логирует и выходит; catchUp/cleanup работают.
- Кик из чата: диалог может остаться в getConversations → getHistory вернёт ошибку →
  лог и continue (существующая обработка в catchUp/cleanupOrphans).
- ownerID пишется один раз при старте до запуска горутин — гонок нет.

## Риски

- Несколько страниц `messages.getConversations` при большом числе диалогов — только
  при старте/рестарте, приемлемо.
- `groups.getById` без параметров работает с токеном сообщества (на этом построен
  vksdk `NewLongPollCommunity`) — риск низкий; при сбое действует решение 3.

## Валидация

1. `go vet ./...`, `go mod tidy`, `go build ./...`, `make win`.
2. Ручные сценарии:
   - без `pinguin_ids` (только токен): старт; в логе creator id; `/stop` из ЛС
     создателя работает; из чужой ЛС — plain help без панели;
   - приглашение в новую беседу → приветствие без `pinguin_ids`;
   - подписка в беседе → рестарт процесса → catchUp реплеит триггер, статусный ответ
     на месте; cleanupOrphans удаляет ответ после ❌;
   - с `pinguin_ids`: поведение прежнее (владелец = первый id, чаты ограничены).
