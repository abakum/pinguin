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
	mainCtx, mainCancel = context.WithCancel(context.Background())
	ttCtx, ttCancel = context.WithCancel(mainCtx)

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
		ltf.Println("closer mainCancel()")
		mainCancel()
		ltf.Println("closer ips.close")
		ips.close()
		wg.Wait()
		// pressEnter()
	})
	ul, err = jibber_jabber.DetectLanguage()
	if err != nil {
		ul = "en"
	}
	if len(chats) == 0 {
		err = Errorf(dic.add(ul,
			"en:Usage: %s AllowedPeerID1 AllowedPeerID2 AllowedPeerIDx\n",
			"ru:Использование: %s РазрешённыйPeerID1 РазрешённыйPeerID2 РазрешённыйPeerIDх\n",
		), os.Args[0])
		return
	} else {
		li.Println(dic.add(ul,
			"en:Allowed PeerID:",
			"ru:Разрешённые PeerID:",
		), chats)
	}
	ex, err := os.Getwd()
	if err == nil {
		tmbPingJson = filepath.Join(ex, tmbPingJson)
	}
	li.Println(filepath.FromSlash(tmbPingJson))

	bot, err = CreateBot(os.Getenv("TOKEN"))

	if err != nil {
		err = srcError(err)
		return
	}

	tacker = time.NewTicker(tt)
	defer tacker.Stop()
	bh, err = startH(ttCtx)
	SendError(fmt.Errorf("startH %v", err))
	if err != nil {
		return
	}

	wg.Add(1)
	go saver()

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
				SendError(fmt.Errorf("stopH %v", stopH(ttCancel, bh)))
				ttCtx, ttCancel = context.WithCancel(mainCtx)
				bh, err = startH(ttCtx)
				SendError(fmt.Errorf("startH %v", err))
				if err != nil {
					letf.Println(err)
					restart(tacker, tt)
				}
			}
		}
	}()

	err = loader()
	if err != nil {
		return
	}
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

// conversation message id of message, fallback to id
func msgID(tm *object.MessagesMessage) int {
	if tm.ConversationMessageID > 0 {
		return tm.ConversationMessageID
	}
	return tm.ID
}

// handler IP
func bhAnyWithMatch(tc string, tm *object.MessagesMessage) error {
	ok, ups := allowed(tm.FromID, tm.PeerID)
	keys, _ := set(reIP.FindAllString(tc, -1))
	ltf.Println("MessageNew anyWithIP", keys, tm.PeerID, tm.FromID, msgID(tm))
	if ok {
		for _, ip := range keys {
			ips.write(ip, customer{PeerID: tm.PeerID, UserID: tm.FromID, MsgID: msgID(tm)})
		}
	} else {
		news := ""
		for _, ip := range keys {
			if ips.read(ip) {
				ips.write(ip, customer{PeerID: tm.PeerID, UserID: tm.FromID, MsgID: msgID(tm)})
			} else {
				news += ip + " "
			}
		}
		if len(news) > 1 {
			_, err := sendKeyboard(tm.PeerID, msgID(tm), "/"+strings.TrimRight(news, " ")+"\n"+ups)
			if err != nil {
				let.Println(err)
			}
		}
		return nil
	}
	return nil
}

// handler callback button
func onMessageEvent(obj events.MessageEventObject) error {
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
		// direct message from allowed peer - two pinnable control panels
		_, err := sendKeyboard(tm.PeerID, msgID(tm), "…🔁 …⏸️ …❌", kbGroup1)
		if err != nil {
			let.Println(err)
		}
		_, err = sendKeyboard(tm.PeerID, msgID(tm), "…✅❌  …❗❌  …⏸️❌", kbGroup2)
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

// is key in args
func allowed(peerIDs ...int) (ok bool, s string) {
	s = "\n🏓"
	for _, v := range peerIDs {
		ok = chats.allowed(v)
		if ok {
			return
		}
	}
	s = notAllowed(false, peerIDs[0], ul)
	return
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
