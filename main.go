package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/events"
	lp "github.com/SevereCloud/vksdk/v3/longpoll-bot"
	"github.com/SevereCloud/vksdk/v3/object"
	"github.com/cloudfoundry/jibber_jabber"
	"github.com/xlab/closer"
)

func main() {
	// one-shot send: pinguin <peerID> <message...>, longpoll not started
	if len(os.Args) > 2 {
		if err := cliSend(os.Args[1], strings.Join(os.Args[2:], " ")); err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
			os.Exit(1)
		}
		return
	}
	elevateIfNeeded()
	mainCtx, mainCancel = context.WithCancel(context.Background())
	ttCtx, ttCancel = context.WithCancel(mainCtx)
	chats = NewAAA()

	var (
		err error
	)
	defer closer.Close()

	closer.Bind(func() {
		if err != nil {
			let.Println(err)
			SendError(err)
			defer os.Exit(1)
		}
		// remember shutdown time for the catch-up queue on next start
		stopAt = int(time.Now().Unix())
		ltf.Println("closer stopH")
		sendStatus("⏹️🏓", stopH(ttCancel, bh))
		ltf.Println("closer mainCancel()")
		mainCancel()
		// wait for all workers to push their records to save
		wg.Wait()
		// single flush signal: saver drains save and writes the file
		saveDone <- true
		<-saverDone
	})
	ul, err = jibber_jabber.DetectLanguage()
	if err != nil {
		ul = "en"
	}
	// start saver before any error path: closer always waits for saverDone
	go saver()
	if len(chats) == 0 {
		err = Errorf(dic.add(ul,
			"en:%s: set pinguin_ids (and pinguin_token) via environment, .env or Windows Credential Manager\n",
			"ru:%s: задайте pinguin_ids (и pinguin_token) в среде, .env или Windows Credential Manager\n",
		), os.Args[0])
		fatalWait(err)
		return
	} else {
		li.Println(dic.add(ul,
			"en:Allowed PeerID:",
			"ru:Разрешённые PeerID:",
		), chats)
	}
	tmbPingJson = filepath.Join(exeDir(), tmbPingJson)
	li.Println(filepath.FromSlash(tmbPingJson))

	bot, err = CreateBot(envValue("pinguin_token"))

	if err != nil {
		err = srcError(err)
		fatalWait(err)
		return
	}

	err = loader()
	if err != nil {
		fatalWait(err)
		return
	}

	// replay messages accumulated between stopAt and now (Telegram-like queue)
	catchUp(int(time.Now().Unix()))

	tacker = time.NewTicker(tt)
	defer tacker.Stop()
	bh, err = startH(ttCtx)
	sendStatus("▶️🏓", err)
	if err != nil {
		fatalWait(err)
		return
	}

	// watch own messages sent from CLI
	go poller()

	wg.Add(1)
	// main loop
	go func() {
		defer wg.Done()
		ticker = time.NewTicker(dd)
		defer ticker.Stop()
		// tacker = time.NewTicker(tt)
		defer tacker.Stop()
		for {
			select {
			case <-mainCtx.Done():
				ltf.Println("Ticker done")
				return
			case t := <-ticker.C:
				ltf.Println("Tick at", t)
				ips.update(customer{})
			case t := <-tacker.C:
				ltf.Println("Tack at", t)
				sendStatus("⏹️🏓", stopH(ttCancel, bh))
				ttCtx, ttCancel = context.WithCancel(mainCtx)
				bh, err = startH(ttCtx)
				sendStatus("▶️🏓", err)
				if err != nil {
					letf.Println(err)
					restart(tacker, tt)
				}
			}
		}
	}()

	closer.Hold()
}

// stop handler, polling
func stopH(cancel context.CancelFunc, l *lp.LongPoll) (err error) {

	if cancel != nil {
		ltf.Println("Cancel longpoll ctx")
		cancel()
	}
	if l != nil {
		ltf.Println("lp.Shutdown")
		l.Shutdown()
	}
	return
}

// start handler and polling
func startH(_ context.Context) (*lp.LongPoll, error) {
	l, err := lp.NewLongPollCommunity(bot)
	if err != nil {
		return nil, srcError(err)
	}

	// router of message_new events
	l.MessageNew(func(_ context.Context, obj events.MessageNewObject) {
		_ = onMessageNew(&obj.Message)
	})
	// callback buttons
	l.MessageEvent(func(_ context.Context, obj events.MessageEventObject) {
		_ = onMessageEvent(obj)
	})

	go func() {
		if err := l.Run(); err != nil {
			letf.Println("longpoll", err)
		}
	}()

	return l, nil
}

// router instead of telegohandler predicates
func onMessageNew(tm *object.MessagesMessage) error {
	if tm == nil {
		return nil
	}
	pollSoon()
	if tm.Action.Type != "" {
		switch tm.Action.Type {
		case "chat_kick_user":
			return bhLeftChat(tm)
		case "chat_invite_user":
			return bhNewMember(tm)
		}
		return nil
	}
	if tm.ReplyMessage != nil && tm.Text == "-" {
		return bhReplyMessageIsMinus(tm)
	}
	tc := tm.Text
	if tm.ReplyMessage != nil {
		tc += " " + tm.ReplyMessage.Text
	}
	if reIP.MatchString(tc) {
		return bhAnyWithMatch(tc, tm)
	}
	if strings.HasPrefix(tm.Text, "/") {
		return bhAnyCommand(tm)
	}
	return nil
}

// poll chat history for messages sent from CLI (VK longpoll does not emit
// message_new for community's outgoing messages). CLI sends are marked with
// a 🏓 prefix - only those get through.
var lastMsg = map[int]int{}

const cliMark = "🏓"

func pollOwnMessages() {
	for _, peer := range chats {
		res, err := bot.MessagesGetHistory(api.Params{
			"peer_id": peer,
			"count":   10,
		})
		if err != nil {
			let.Println("poll", peer, err)
			continue
		}
		last, ok := lastMsg[peer]
		for _, tm := range res.Items { // newest first
			if tm.ID > lastMsg[peer] {
				lastMsg[peer] = tm.ID
			}
			if !ok || tm.ID <= last { // first poll - baseline, or seen already
				continue
			}
			if tm.FromID >= 0 || !strings.HasPrefix(tm.Text, cliMark) {
				continue
			}
			ltf.Println("poll", peer, tm.ID, tm.FromID, "cli message:", tm.Text)
			if reIP.MatchString(tm.Text) {
				if err := bhAnyWithMatch(tm.Text, &tm); err != nil {
					let.Println("poll subscribe", peer, err)
				}
			}
		}
	}
}

// replay messages accumulated between the last stopH and now for every peer
// in chats (Telegram-like queue), run before longpoll starts
func catchUp(startAt int) {
	for _, peer := range chats {
		res, err := bot.MessagesGetHistory(api.Params{
			"peer_id": peer,
			"count":   200,
		})
		if err != nil {
			let.Println("catchUp", peer, err)
			continue
		}
		n := 0
		for _, tm := range res.Items { // newest first
			if tm.ID > lastMsg[peer] {
				lastMsg[peer] = tm.ID
			}
			if stopAt == 0 || tm.Date < stopAt || tm.Date > startAt {
				continue
			}
			// skip own text without CLI mark (status reports carry IPs),
			// invite/kick actions are replayed
			if tm.Action.Type == "" && tm.FromID < 0 && !strings.HasPrefix(tm.Text, cliMark) {
				continue
			}
			n++
			if err := onMessageNew(&tm); err != nil {
				let.Println("catchUp", peer, tm.ID, err)
			}
		}
		if n > 0 {
			ltf.Println("catchUp", peer, "replayed", n)
		}
	}
}

// watch messages sent from CLI: get history right after any longpoll event,
// then refresh after idle; debounce event storms
var (
	pollReset = make(chan struct{}, 1)
	lastPoll  time.Time
)

// pollSoon restarts the poll timer, call after any handled event
func pollSoon() {
	select {
	case pollReset <- struct{}{}:
	default:
	}
}

func poller() {
	ltf.Println("poller start:", "getHistory after last event, idle", refresh)
	t := time.NewTimer(refresh)
	defer t.Stop()
	for {
		select {
		case <-mainCtx.Done():
			ltf.Println("poller done")
			return
		case <-t.C:
			pollOwnMessages()
			lastPoll = time.Now()
			t.Reset(refresh)
		case <-pollReset:
			if time.Since(lastPoll) < time.Second*5 {
				t.Reset(time.Until(lastPoll.Add(time.Second * 5)))
				continue
			}
			pollOwnMessages()
			lastPoll = time.Now()
			t.Reset(refresh)
		}
	}
}

// conversation message id of message, fallback to id
func msgID(tm *object.MessagesMessage) int {
	if tm.ConversationMessageID > 0 {
		return tm.ConversationMessageID
	}
	return tm.ID
}

// handler IP
func bhAnyWithMatch(tc string, tm *object.MessagesMessage) error {
	keys, _ := set(reIP.FindAllString(tc, -1))
	ltf.Println("MessageNew anyWithIP", keys, tm.PeerID, tm.FromID, msgID(tm))
	for _, ip := range keys {
		ips.write(ip, customer{PeerID: tm.PeerID, UserID: tm.FromID, MsgID: msgID(tm)})
	}
	return nil
}

// handler callback button
func onMessageEvent(obj events.MessageEventObject) error {
	pollSoon()
	tm := convMessage(obj.PeerID, obj.ConversationMessageID)
	if tm == nil {
		return nil
	}
	Data := unpay(obj.Payload)
	my := true
	if obj.PeerID != obj.UserID && tm.ReplyMessage != nil {
		my = obj.UserID == tm.ReplyMessage.FromID
	}
	ip := reIP.FindString(tm.Text)
	if strings.HasPrefix(Data, "…") {
		ip = ""
	}
	ups := fmt.Sprintf("#%d%s", obj.UserID, notAllowed(my, 0, ul))
	letf.Println("MessageEvent", Data, ups, tf(ips.count() == 0, "∅", ip+Data))
	err := answerEvent(obj.EventID, obj.UserID, obj.PeerID, ups+tf(ips.count() == 0, "∅", ip+Data))
	if err != nil {
		let.Println(err)
	}
	if !my {
		return nil
	}
	if Data == "❎" {
		if tm.ID > 0 { // messages.delete expects global message id, not cmid
			err = deleteMessage(obj.PeerID, tm.ID)
			if err != nil {
				let.Println(err)
			}
		}
		return nil
	}

	// owner-only stop/restart buttons
	if Data == "⏹️🏓" || Data == "⏹️🏓▶️" {
		if tm.PeerID > 0 && len(chats) > 0 && chats[:1].allowed(obj.UserID) {
			if Data == "⏹️🏓" {
				closer.Close()
			} else {
				restart(tacker, tt)
			}
		}
		return nil
	}

	if ips.count() == 0 {
		return nil
	}
	if strings.HasPrefix(Data, "…") {
		ips.update(customer{Cmd: strings.TrimPrefix(Data, "…")})
	} else {
		ips.write(ip, customer{Cmd: Data})
	}
	return nil
}

// handler DeleteMessage
func bhReplyMessageIsMinus(tm *object.MessagesMessage) error {
	re := tm.ReplyMessage
	id := re.ConversationMessageID
	if id == 0 {
		id = re.ID
	}
	err := deleteMessage(tm.PeerID, id)
	if err != nil {
		let.Println(err)
		_, err = bot.MessagesEdit(api.Params{
			"peer_id":    tm.PeerID,
			"message_id": id,
			"message":    "-",
		})
		if err != nil {
			let.Println(err)
		}
	}
	return nil
}

// send t.C then reset t
func restart(t *time.Ticker, d time.Duration) {
	if t != nil {
		t.Reset(time.Millisecond * 100)
		time.Sleep(time.Millisecond * 150)
		t.Reset(d)
	}
}

// handler Command
func bhAnyCommand(tm *object.MessagesMessage) error {
	// For owner as first peerID in args
	if tm.PeerID > 0 && len(chats) > 0 && chats[:1].allowed(tm.FromID) {
		if strings.HasPrefix(tm.Text, "/restart") {
			restart(tacker, tt)
			return nil
		}
		if strings.HasPrefix(tm.Text, "/stop") {
			closer.Close()
			return nil
		}
	}
	if tm.PeerID == tm.FromID && chats.allowed(tm.FromID) {
		// direct message from allowed peer - pinnable control panel
		kb := kbGroup
		if len(chats) > 0 && chats[:1].allowed(tm.FromID) {
			kb = kbOwner // first peer in args can also stop/restart
		}
		_, err := sendKeyboard(tm.PeerID, msgID(tm), "⠀", kb)
		if err != nil {
			let.Println(err)
		}
		return nil
	}
	// group chats and strangers - plain text without keyboard
	text := dic.add(ul,
		"en:List of IP addresses expected\n",
		"ru:Ожидался список IP адресов\n",
	) + "/127.0.0.1 127.0.0.2 127.0.0.254"
	_, err := sendKeyboard(tm.PeerID, msgID(tm), text, nil)
	if err != nil {
		let.Println(err)
	}
	return nil
}

// handler LeftChat
func bhLeftChat(tm *object.MessagesMessage) error {
	text := dic.add(ul,
		"en:He flew away, but promised to return❗\n",
		"ru:Он улетел, но обещал вернуться❗\n",
	) + dic.add(ul,
		"en:Cute... 😢",
		"ru:Милый... 😢",
	)
	_, err := sendKeyboard(tm.PeerID, msgID(tm), text)
	if err != nil {
		let.Println(err)
	}
	return nil
}

// handler NewMember
func bhNewMember(tm *object.MessagesMessage) error {
	if !chats.allowed(tm.PeerID) {
		return nil
	}
	ltf.Println(tm.Action.MemberID)
	text := dic.add(ul,
		"en:Hello villagers!\nThe cart is ready!🏓",
		"ru:Здорово, селяне!\nТелега готова!🏓",
	)
	_, err := sendKeyboard(tm.PeerID, msgID(tm), text)
	if err != nil {
		let.Println(err)
	}
	return nil
}

// message for peerID
func notAllowed(ok bool, peerID int, key string) (s string) {
	s = "\n🏓"
	if ok {
		return
	}
	s = dic.add(key,
		"en:\nNot allowed for you",
		"ru:\nБатюшка не благословляет Вас",
	)
	if peerID != 0 {
		s += fmt.Sprintf(":%d", peerID)
	}
	s += "\n🏓"
	return
}
