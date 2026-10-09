package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/events"
	lp "github.com/SevereCloud/vksdk/v3/longpoll-bot"
	"github.com/SevereCloud/vksdk/v3/object"
	"github.com/cloudfoundry/jibber_jabber"
	"github.com/xlab/closer"
)

func main() {
	// one-shot send: pinguin <sourcePeerID> 0|<targetPeerID>|<targetChatID>
	// messageWord1 ..., longpoll not started
	if len(os.Args) > 3 {
		if err := cliSend(os.Args[1:]); err != nil {
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
		// mark the group as stopped on a full bot stop (not on in-process restart)
		setGroupDescription(descStopped)
		ltf.Println("closer stopH")
		sendStatus(cmdStop, stopH(ttCancel, bh))
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

	// delete orphaned bot answers at startup: replies whose ip is no longer
	// monitored, or whose request message has been deleted
	cleanupOrphans()

	// replay messages accumulated between stopAt and now (Telegram-like queue)
	catchUp(int(time.Now().Unix()))

	tacker = time.NewTicker(tt)
	defer tacker.Stop()
	bh, err = startH(ttCtx)
	sendStatus(cmdRestart, err)
	if err != nil {
		fatalWait(err)
		return
	}
	// mark the group as working only on a full process start (not on
	// in-process restart, which never re-runs main)
	setGroupDescription(descWorking)

	wg.Add(1)
	// main loop
	go func() {
		defer wg.Done()
		ticker = time.NewTicker(tth)
		defer ticker.Stop()
		// tacker = time.NewTicker(tt)
		defer tacker.Stop()
		// loader() ran before ticker existed, so the refresh Reset in
		// sCustomer.add was skipped: arm the period from the loaded hosts
		if ips.count() > 0 {
			ticker.Reset(refresh)
		}
		for {
			select {
			case <-mainCtx.Done():
				ltf.Println("Ticker done")
				return
			case t := <-ticker.C:
				ltf.Println("Tick at", t)
				ips.update(customer{})
				// keep the period in step with the worker set instead of
				// relying on the 0<->non-empty transitions in add/del
				if ips.count() > 0 {
					ticker.Reset(refresh)
				} else {
					ticker.Reset(tth)
				}
			case t := <-tacker.C:
				ltf.Println("Tack at", t)
				if err := recycleLP(); err != nil {
					letf.Println(err)
					restart(tacker, tt)
				}
			}
		}
	}()

	// press …⏸️ at 18:00 and …🔁 at 08:00 Moscow time, same as the panel
	go workHours()

	closer.Hold()
}

// Moscow time, fixed UTC+3 without DST
var msk = time.FixedZone("MSK", 3*60*60)

// press …⏸️ at 18:00 and …🔁 at 08:00 MSK: pause all workers for the
// evening or resume them for the working day
func workHours() {
	for {
		n := time.Now().In(msk)
		y, m, d := n.Date()
		morning := time.Date(y, m, d, 8, 0, 0, 0, msk)
		evening := time.Date(y, m, d, 18, 0, 0, 0, msk)
		var next time.Time
		cmd := "🔁"
		switch {
		case n.Before(morning):
			next = morning
		case n.Before(evening):
			next, cmd = evening, "⏸️"
		default:
			next = morning.AddDate(0, 0, 1)
		}
		select {
		case <-mainCtx.Done():
			ltf.Println("workHours done")
			return
		case <-time.After(time.Until(next)):
			ltf.Println("workHours", cmd)
			ips.update(customer{Cmd: cmd})
		}
	}
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

// serialize longpoll recycling between the Tack case and superviseLP
var lpMu sync.Mutex

// revive backoff shared by superviseLP generations, guarded by lpMu
var lpBackoff = time.Second * 3

// stop the current longpoll and start a fresh one (2h Tack recycle and
// crash recovery path)
func recycleLP() (err error) {
	lpMu.Lock()
	defer lpMu.Unlock()
	sendStatus(cmdStop, stopH(ttCancel, bh))
	ttCtx, ttCancel = context.WithCancel(mainCtx)
	bh, err = startH(ttCtx)
	sendStatus(cmdRestart, err)
	return err
}

// start handler and polling
func startH(ctx context.Context) (*lp.LongPoll, error) {
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
	// CLI one-shot sends trigger a message_reply in the owner dialog; the
	// trigger text carries the target peer id, the answer goes to that peer
	l.MessageReply(func(_ context.Context, obj events.MessageReplyObject) {
		tm := object.MessagesMessage(obj)
		// FromID < 0: message_reply fires only for community's own sends,
		// nobody else can produce them
		if tm.FromID >= 0 || !strings.HasPrefix(tm.Text, cliMark) {
			return
		}
		target := chats[0]
		body := strings.TrimPrefix(tm.Text, cliMark)
		if f, rest, ok := strings.Cut(body, " "); ok {
			if n, err := strconv.Atoi(f); err == nil {
				target, body = n, rest
			}
		}
		// CLI one-shot: for uniformity the request text is echoed to the
		// target peer and the bot answers as a reply to it
		ltf.Println("reply", target, "cli:", body)
		if !reIP.MatchString(body) {
			return
		}
		reqID, err := bot.MessagesSend(api.Params{
			"peer_id":   target,
			"message":   body,
			"random_id": 0,
		})
		if err != nil {
			let.Println("reply request", target, err)
			return
		}
		cmid := convMsgByID(target, reqID)
		uniq, _ := set(reIP.FindAllString(body, -1))
		for _, ip := range uniq {
			ips.write(ip, customer{PeerID: target, UserID: tm.PeerID, MsgID: cmid})
		}
	})

	go superviseLP(ctx, l)

	return l, nil
}

// run the longpoll and revive it after failures: vksdk's RunWithContext
// returns on any single a_check error (e.g. context deadline exceeded)
// without an internal retry, which used to leave the bot unresponsive
// until the next Tack
func superviseLP(ctx context.Context, l *lp.LongPoll) {
	const minBackoff, maxBackoff = time.Second * 3, time.Minute
	started := time.Now()
	err := l.RunWithContext(ctx)
	if err == nil || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		if err != nil {
			letf.Println("longpoll", err) // expected context canceled on Shutdown
		}
		return
	}
	letf.Println("longpoll", err)
	// a generation that lived long retries quickly, a flapping one keeps
	// the grown backoff from its predecessors
	lpMu.Lock()
	if time.Since(started) >= time.Minute {
		lpBackoff = minBackoff
	}
	b := lpBackoff
	lpBackoff = min(lpBackoff*2, maxBackoff)
	lpMu.Unlock()
	for mainCtx.Err() == nil {
		time.Sleep(b)
		if err := recycleLP(); err != nil {
			letf.Println("recycleLP", err)
			lpMu.Lock()
			b = lpBackoff
			lpBackoff = min(lpBackoff*2, maxBackoff)
			lpMu.Unlock()
			continue
		}
		return
	}
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

// replay messages accumulated between the last stopH and now for every peer
// in chats (Telegram-like queue), run before longpoll starts
const cliMark = "🏓"

// owner report/stop/restart button payloads, must match the kbOwner
// keyboard (vk.go)
const (
	cmdStop    = "⏹️🏓"
	cmdRestart = "▶️🏓"
	cmdReport  = "…" // all hosts with statuses and authors

	cmdVerify = "❓" // worker sentinel: re-verify that request messages still exist

	// delay before the panel refresh after a group command, enough for
	// workers to apply it and re-ping their hosts (ping timeout is ~0.6s)
	panelDelay = time.Second * 2
)

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

// delete orphaned bot answers after load: status replies (bot messages with a
// keyboard and an ip) whose ip is no longer monitored. Used at startup.
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
			if tm.FromID >= 0 { // bot messages only
				continue
			}
			if strings.HasPrefix(tm.Text, cliMark) { // keep control-plane triggers
				continue
			}
			if !reStatusReply.MatchString(tm.Text) { // status replies only: ✅⏸️ 1.2.3.4 / ❗ 5.6.7.8 etc
				continue
			}
			ip := reIP.FindString(tm.Text)
			if ip == "" || ips.read(ip) { // keep if the ip is monitored
				continue
			}
			// messages.delete expects the global message id, not the
			// conversation message id (see the ❎ branch in onMessageEvent)
			if tm.ID > 0 {
				ltf.Println("cleanupOrphans delete", peer, ip, tm.ID)
				if err := deleteMessage(peer, tm.ID); err != nil {
					let.Println(err)
				}
			}
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
	tm := convMessage(obj.PeerID, obj.ConversationMessageID)
	if tm == nil {
		return nil
	}
	Data := unpay(obj.Payload)
	// reply author by the "Name @id<N>" marker only: replies without a
	// marker have no author, their buttons (except 🔁) are ignored
	authorID := 0
	if m := reAuthor.FindStringSubmatch(tm.Text); m != nil {
		// "@id<N>" as sent, or the "[id<N>|...]" mention VK re-renders it to
		if m[1] != "" {
			authorID, _ = strconv.Atoi(m[1])
		} else {
			authorID, _ = strconv.Atoi(m[2])
		}
	} else if tm.ReplyMessage != nil && tm.ReplyMessage.FromID > 0 {
		// pre-marker replies never get re-tagged until the status
		// changes: the requester is the reply parent; the bot itself
		// (negative id) is not an author
		authorID = tm.ReplyMessage.FromID
	}
	my := obj.PeerID == obj.UserID || obj.UserID == authorID
	ip := reIP.FindString(tm.Text)
	if strings.HasPrefix(Data, "…") {
		ip = ""
	}
	// foreign 🔁 works on any reply, ⏸️/❌/❎ need a marked one
	allowed := my || Data == "🔁" || authorID > 0 && (Data == "⏸️" || Data == "❌" || Data == "❎")
	ups := fmt.Sprintf("#%d%s", obj.UserID, notAllowed(allowed, 0, ul))
	letf.Println("MessageEvent", Data, ups, tf(ips.count() == 0, "∅", ip+Data), "author", authorID, tm.Text)
	err := answerEvent(obj.EventID, obj.UserID, obj.PeerID, ups+tf(ips.count() == 0, "∅", ip+Data))
	if err != nil {
		let.Println(err)
	}
	if !my && Data != "🔁" { // foreign 🔁 goes the common way, no protocol
		foreignButton(obj, tm, Data, ip, authorID)
		return nil
	}
	if Data == "❎" {
		if tm.ID > 0 { // messages.delete expects global message id, not cmid
			err = deleteMessage(obj.PeerID, tm.ID)
			if err != nil {
				let.Println(err)
			}
		}
		if ip != "" && ips.read(ip) {
			// drop the presser's subscription too, so the hosts list keeps
			// only interested peers; MsgID 0 routes it as a button command
			ips.write(ip, customer{Cmd: "❎", PeerID: obj.PeerID, UserID: obj.UserID})
		}
		return nil
	}

	// owner-only stop/restart buttons
	if Data == cmdStop || Data == cmdRestart {
		if tm.PeerID > 0 && len(chats) > 0 && chats[:1].allowed(obj.UserID) {
			if Data == cmdStop {
				closer.Close()
			} else {
				restart(tacker, tt)
			}
		}
		return nil
	}

	// owner-only report button, intercepted before the "…" group commands:
	// edit the panel reply in place with the hosts list, no separate answer
	if Data == cmdReport {
		if tm.PeerID > 0 && len(chats) > 0 && chats[:1].allowed(obj.UserID) {
			go editPanel(obj.PeerID, obj.ConversationMessageID, obj.UserID)
		}
		return nil
	}

	// unsubscribe button on orphaned status messages: worker is gone,
	// delete the message with buttons directly
	if Data == "❌" && ips.count() == 0 {
		if tm.ID > 0 {
			if err := deleteMessage(obj.PeerID, tm.ID); err != nil {
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
		if tm.PeerID > 0 { // panel lives in a direct message only
			// wait for the workers to apply the command and re-ping
			// before refreshing the panel, unlike the instant … report
			go func() {
				time.Sleep(panelDelay)
				editPanel(obj.PeerID, obj.ConversationMessageID, obj.UserID)
			}()
		}
	} else {
		if ip == "" { // guard: an unknown non-group button must not spawn a broken worker
			let.Println("unknown button", Data)
			return nil
		}
		ips.write(ip, customer{Cmd: Data})
	}
	return nil
}

// button press on someone else's status reply: ⏸️ edits the pressed reply
// to a protocol line in place, ❌/❎ log a keyless protocol reply and hand
// the subscription over to the presser
func foreignButton(obj events.MessageEventObject, tm *object.MessagesMessage, Data, ip string, authorID int) {
	if authorID <= 0 || ip == "" || !ips.read(ip) {
		let.Println("foreign button ignored", Data, authorID, ip)
		return
	}
	reqCmid := 0 // the original request, target for the presser's new reply
	if tm.ReplyMessage != nil {
		reqCmid = tm.ReplyMessage.ConversationMessageID
	}
	if reqCmid == 0 {
		reqCmid = msgID(tm)
	}
	st := "" // status at tap time
	if f := strings.Fields(tm.Text); len(f) > 0 {
		st = f[0]
	}
	tap := fmt.Sprintf(" @id%d @id%d", obj.UserID, authorID)
	switch Data {
	case "⏸️":
		// replace the pressed reply with a protocol one: a fresh send
		// keeps the reply header, unlike messages.edit
		text := strings.TrimSuffix(st, "⏸️") + "⏸️ " + ip + " " + userMention(obj.UserID) + tap
		newID, err := sendKeyboard(obj.PeerID, reqCmid, text, kbIP)
		if err != nil {
			let.Println(err)
		}
		if tm.ID > 0 {
			if err := deleteMessage(obj.PeerID, tm.ID); err != nil {
				let.Println(err)
			}
		}
		if err != nil || newID <= 0 {
			ips.write(ip, customer{Cmd: Data, PeerID: obj.PeerID, UserID: obj.UserID})
			return
		}
		// UserID is the author: the worker adopts the new protocol reply
		// as the author's status reply and does not recreate it
		ips.write(ip, customer{Cmd: Data, PeerID: obj.PeerID, UserID: authorID, ReplyID: newID})
	case "❌", "❎":
		if Data == "❎" && tm.ID > 0 { // same as the own-press branch
			if err := deleteMessage(obj.PeerID, tm.ID); err != nil {
				let.Println(err)
			}
		}
		// protocol line without keyboard: explicit nil, or sendKeyboard
		// attaches kbIP by default
		if _, err := sendKeyboard(obj.PeerID, reqCmid, st+" "+ip+" "+Data+userMention(obj.UserID)+tap, nil); err != nil {
			let.Println(err)
		}
		if reqCmid == 0 {
			let.Println("subscription takeover skipped, no request cmid")
			return
		}
		// subscribe the presser first so the worker never runs out of
		// subscribers and the channel stays open, then unsubscribe the
		// old ones
		ips.write(ip, customer{PeerID: obj.PeerID, UserID: obj.UserID, MsgID: reqCmid})
		if Data == "❌" {
			old := hosts.list()[ip].Cus
			if len(old) == 0 {
				ips.write(ip, customer{Cmd: "❎", UserID: authorID})
			}
			for _, cu := range old {
				if cu.UserID != 0 && cu.UserID != obj.UserID {
					ips.write(ip, customer{Cmd: "❎", UserID: cu.UserID})
				}
			}
		} else {
			ips.write(ip, customer{Cmd: "❎", UserID: authorID})
		}
	}
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
	go func() {
		// workers re-verify the requests of their subscribers and drop orphans
		ips.update(customer{Cmd: cmdVerify})
		// replies whose ip is no longer monitored
		cleanupOrphans()
	}()
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

// refresh the control panel reply ("⠀", a reply to "/") in place with the
// current hosts list; the keyboard is rebuilt the same way as in
// bhAnyCommand, messages.edit wipes it otherwise
func editPanel(peerID, conversationMessageID, userID int) {
	kb := kbGroup
	if len(chats) > 0 && chats[:1].allowed(userID) {
		kb = kbOwner
	}
	if err := editMessage(peerID, conversationMessageID, hostsText(), kb); err != nil {
		let.Println(err)
	}
}

// hosts report for the kbOwner panel: one line per host with status and
// unique author mentions, sorted by ip, "∅" when nothing is monitored
func hostsText() string {
	hs := hosts.list()
	if len(hs) == 0 {
		return "∅"
	}
	keys := make([]string, 0, len(hs))
	for ip := range hs {
		keys = append(keys, ip)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, ip := range keys {
		st := hs[ip]
		seen := map[int]bool{}
		var auth []string
		for _, cu := range st.Cus {
			if cu.PeerID == 0 || cu.UserID == 0 || seen[cu.UserID] {
				continue
			}
			seen[cu.UserID] = true
			name := userMention(cu.UserID)
			if name == "" {
				name = "@id" + strconv.Itoa(cu.UserID)
			}
			auth = append(auth, name)
		}
		line := strings.TrimSpace(st.Status+" "+ip) + tf(len(auth) > 0, " "+strings.Join(auth, ", "), "")
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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
