# План: удаление осиротевших ответов бота при перезапуске

## Контекст и диагноз

Мониторинг строится на карте `ips` (IP → канал) в `global.go` (`sCustomer`).
На каждый IP есть воркер `worker()` (worker.go), который держит список подписчиков
`cus`; у каждого подписчика `customer`:

- `MsgID` — conversation id запроса (сообщение с IP);
- `ReplyID` — conversation id ответа бота (статус с кнопками);
- `Cmd` — при сохранении в `pinguin.json` равен IP.

При удалении IP живой командой `❌` (worker.go, ветка `❌`) ответы уже удаляются,
а IP снимается через `ips.del`. Это покрывает штатное удаление.

**Проблема — осиротевшие ответы после перезапуска:**

- **(A)** IP убран из базы мониторинга (например, внешне из `pinguin.json` при
  остановленном боте), а его ответ остался в чате. При старте такой IP просто не
  загружается → его ответы никто не удаляет.
- **(B)** Удалён сам запрос (сообщение с IP), а ответ на него остался.

**Ограничение:** событий `message_delete` в community LongPoll (Bot API) vksdk
нет (в `events/events.go` v3.3.1 отсутствует `EventMessageDelete`, метода
`LongPoll.MessageDelete` тоже нет). Поэтому real-time детекция невозможна —
по решению пользователя обработка **только при перезапуске/чтении истории**.

## Задачи

### 1. Случай A — функция `cleanupOrphans()` (main.go)

Новая функция, вызывается после `loader()` и до `catchUp(...)`.

```go
func cleanupOrphans() {
	for _, peer := range chats {
		res, err := bot.MessagesGetHistory(api.Params{
			"peer_id": peer,
			"count":   200,
		})
		if err != nil {
			let.Println("cleanupOrphans", peer, err)
			continue
		}
		for _, tm := range res.Items { // newest first
			if tm.FromID >= 0 { // только сообщения бота
				continue
			}
			if strings.HasPrefix(tm.Text, cliMark) { // управляющие 🏓-триггеры
				continue
			}
			ip := reIP.FindString(tm.Text)
			if ip == "" || ips.read(ip) { // без IP или IP мониторится — оставляем
				continue
			}
			// messages.delete ожидает глобальный message id, а не conversation
			// message id (см. ветку ❎ в onMessageEvent)
			if tm.ID > 0 {
				if err := deleteMessage(peer, tm.ID); err != nil {
					let.Println(err)
				}
			}
		}
	}
}
```

Примечание: поле `tm.Keyboard` в `object.MessagesMessage` является значением
(не указателем), и его заполнение при `MessagesGetHistory` ненадёжно, поэтому
фильтр по клавиатуре не используется. Вместо этого отбрасываем управляющие
`cliMark`-сообщения (`🏓…`); остальные сообщения бота с IP вне `ips`
рассматриваем как осиротевшие статус-ответы.

**Критично:** для `messages.delete` нельзя использовать `msgID(&tm)` (это
conversation message id) — он приводит к удалению чужих сообщений. Берём только
глобальный `tm.ID`.

### 2. Хелпер `msgExists(peerID, cmid) (bool, error)` (vk.go)

Существующий `convMessage` сливает «нет» и «ошибку сети» в `nil` — это не
годится для безопасной проверки. Новый хелпер разделяет исходы.

```go
func msgExists(peerID, conversationMessageID int) (bool, error) {
	res, err := bot.MessagesGetByConversationMessageID(api.Params{
		"peer_id":                  peerID,
		"conversation_message_ids": []int{conversationMessageID},
	})
	if err != nil {
		return false, err
	}
	return len(res.Items) > 0, nil
}
```

### 3. Случай B — проверка в воркере при загрузке (worker.go)

В ветке load для `MsgID != 0` проверяем, жив ли запрос. Если нет — удаляем его
ответ и НЕ добавляем подписчика.

```go
} else if cust.Cmd == ip && cust.Deadline > 0 { //load
	if cust.MsgID != 0 {
		ok, err := msgExists(cust.PeerID, cust.MsgID)
		if err != nil {
			let.Println("msgExists", cust, err) // ошибка сети — не трогаем
		} else if !ok {
			ltf.Println("drop orphan", cust)
			if cust.ReplyID != 0 {
				if err := deleteMessage(cust.PeerID, cust.ReplyID); err != nil {
					let.Println(err)
				}
			}
			continue // не добавляем в cus
		}
	}
	status = cust.Status
	deadline = time.Unix(cust.Deadline, 0)
	cus = append(cus, cust)
	ltf.Println("loaded ", ip, status, deadline)
}
```

**Защита от ложных удалений:** при `err != nil` (сеть/API) подписчик сохраняется —
случайная ошибка не уничтожает живой ответ. Удаляем только при однозначном
«запроса нет». Отсутствие в `cus` автоматически чистит и `pinguin.json` при
следующем сохранении (на shutdown воркер сохраняет только текущие `cus`).

### 4. Сборка

- `go vet ./...`
- `go mod tidy`
- `make win` (по AGENTS.md)

## Проверка

- Собрать `make win`, запустить.
- Убрать IP из `pinguin.json` при остановленном боте, оставив его статус-ответ в
  чате → при старте ответ должен удалиться (случай A).
- Удалить вручную запрос (сообщение с IP), оставив его ответ, затем перезапустить
  бота → ответ должен удалиться (случай B).
- Убедиться, что живые ответы для мониторящихся IP не удаляются, и что при сетевой
  ошибке проверки подписка не теряется.

## Риски

- `MessagesGetHistory` с `count=200` может не покрыть старые осиротевшие ответы;
  при необходимости добавить пагинацию.
- Проверка `msgExists` делает по одному API-вызову на загружаемого подписчика —
  приемлемо при типовых объёмах.
- CLI-запросы имеют `MsgID == 0` (используется `GlobalID`) — случай B их не
  затрагивает (известное ограничение).