package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/object"
)

// create VK community bot client from token
func CreateBot(token string) (*api.VK, error) {
	if token == "" {
		return nil, errors.New("set pinguin_token=VK_COMMUNITY_TOKEN")
	}
	vk := api.NewVK(token)
	// http.DefaultClient has no timeout, a stalled request would block
	// worker shutdown and prevent json save on Ctrl-C
	vk.Client = &http.Client{Timeout: 30 * time.Second}
	return vk, nil
}

// inline keyboards:
// row 1 - single ip commands, rows with "…" prefix - group commands
var (
	// single ip commands only, for status replies
	kbIP = func() *object.MessagesKeyboard {
		kb := object.NewMessagesKeyboardInline()
		kb.AddRow().
			AddCallbackButton("🔁", "🔁", "secondary").
			AddCallbackButton("⏸️", "⏸️", "secondary").
			AddCallbackButton("❌", "❌", "secondary").
			AddCallbackButton("❎", "❎", "secondary")
		return kb
	}()

	// group commands, for pinnable message
	kbGroup = func() *object.MessagesKeyboard {
		kb := object.NewMessagesKeyboardInline()
		kb.AddRow().
			AddCallbackButton("…🔁", "…🔁", "secondary").
			AddCallbackButton("…⏸️", "…⏸️", "secondary").
			AddCallbackButton("…❌", "…❌", "secondary")
		kb.AddRow().
			AddCallbackButton("…✅❌", "…✅❌", "secondary").
			AddCallbackButton("…❗❌", "…❗❌", "secondary").
			AddCallbackButton("…⏸️❌", "…⏸️❌", "secondary")
		return kb
	}()

	// group commands plus owner stop/restart, for pinnable message
	kbOwner = func() *object.MessagesKeyboard {
		kb := object.NewMessagesKeyboardInline()
		kb.AddRow().
			AddCallbackButton("…🔁", "…🔁", "secondary").
			AddCallbackButton("…⏸️", "…⏸️", "secondary").
			AddCallbackButton("…❌", "…❌", "secondary")
		kb.AddRow().
			AddCallbackButton("…✅❌", "…✅❌", "secondary").
			AddCallbackButton("…❗❌", "…❗❌", "secondary").
			AddCallbackButton("…⏸️❌", "…⏸️❌", "secondary")
		kb.AddRow().
			AddCallbackButton("…", "…", "secondary").
			AddCallbackButton(cmdStop, cmdStop, "secondary").
			AddCallbackButton(cmdRestart, cmdRestart, "secondary")
		return kb
	}()
)

// payload is a json encoded string, decode to plain command
func unpay(p json.RawMessage) string {
	var s string
	if err := json.Unmarshal(p, &s); err == nil {
		return s
	}
	return string(p)
}

// send message with optional keyboard, replyTo is conversation message id or 0
func sendKeyboard(peerID, replyTo int, text string, kbs ...*object.MessagesKeyboard) (id int, err error) {
	p := api.Params{
		"peer_id":   peerID,
		"message":   text,
		"random_id": 0,
	}
	if len(kbs) == 0 {
		p["keyboard"] = kbIP.ToJSON()
	} else if kbs[0] != nil {
		p["keyboard"] = kbs[0].ToJSON()
	} // nil - no keyboard
	if replyTo > 0 {
		// reply_to expects a global message id, which is 0 in community chat
		// events, so use forward+is_reply with conversation_message_ids
		p["forward"] = fmt.Sprintf(`{"peer_id":%d,"conversation_message_ids":[%d],"is_reply":true}`, peerID, replyTo)
	}
	return bot.MessagesSend(p)
}

// status reply: forward the request by conversation message id; the
// requester is embedded as a trailing "Name @id<N>" marker, parsed back
// by reAuthor in onMessageEvent
func sendStatusReply(cu customer, text string) (int, error) {
	if cu.UserID > 0 {
		if name := userMention(cu.UserID); name != "" {
			text += " " + name
		}
		text += " @id" + strconv.Itoa(cu.UserID)
	}
	return sendKeyboard(cu.PeerID, cu.MsgID, text)
}

// edit own message text by conversation message id, keyboard must be
// passed again or messages.edit wipes it
func editMessage(peerID, conversationMessageID int, text string, kb *object.MessagesKeyboard) error {
	p := api.Params{
		"peer_id":                  peerID,
		"conversation_message_id":  conversationMessageID,
		"message":                  text,
		"keep_forward_attachments": "1", // keep the reply/forward header on edit
	}
	if kb != nil {
		p["keyboard"] = kb.ToJSON()
	}
	_, err := bot.MessagesEdit(p)
	return err
}

// delete message by global message id; VK reports per-id errors in the
// response body with a 200 status, so inspect it too
func deleteMessage(peerID, messageID int) error {
	res, err := bot.MessagesDelete(api.Params{
		"peer_id":        peerID,
		"message_ids":    messageID,
		"delete_for_all": 1,
		"spam":           0,
	})
	ltf.Println("messages.delete", peerID, messageID, res, err)
	if err != nil {
		return err
	}
	for _, r := range res {
		if r.Error != nil {
			return fmt.Errorf("messages.delete %d: code %d", messageID, r.Error.Code)
		}
	}
	return nil
}

// delete a status reply by its message id: ReplyID holds the global message
// id returned by messages.send, which messages.delete accepts directly
// (the conversation message id is a different number, see the delete log)
func deleteReply(peerID, messageID int) error {
	return deleteMessage(peerID, messageID)
}

// answer callback event with snackbar text
func answerEvent(eventID string, userID, peerID int, text string) error {
	_, err := bot.MessagesSendMessageEventAnswer(api.Params{
		"event_id":   eventID,
		"user_id":    userID,
		"peer_id":    peerID,
		"event_data": object.NewMessagesEventDataShowSnackbar(text),
	})
	return err
}

// resolved user names cache, id -> "First Last" (see userMention)
var userNames sync.Map

// initials of the user name for status replies and host reports: "First
// Last" -> "FL", empty when the name cannot be resolved; no [idN|Name]
// mention to avoid mention notification noise, ids travel in the @id<N>
// markers instead
func userMention(id int) string {
	name := ""
	if v, ok := userNames.Load(id); ok {
		name = v.(string)
	} else {
		res, err := bot.UsersGet(api.Params{"user_ids": id})
		switch {
		case err != nil:
			let.Println("users.get", id, err)
		case len(res) == 0:
			let.Println("users.get", id, "empty")
		default:
			name = strings.TrimSpace(res[0].FirstName + " " + res[0].LastName)
			if name == "" {
				return ""
			}
			userNames.Store(id, name)
		}
	}
	return initials(name)
}

// first letter of every word, uppercased: "Константин Абакумов" -> "КА"
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		b.WriteString(strings.ToUpper(string(r[0])))
	}
	return b.String()
}

// get bot message by conversation message id
func convMessage(peerID, conversationMessageID int) *object.MessagesMessage {
	res, err := bot.MessagesGetByConversationMessageID(api.Params{
		"peer_id":                  peerID,
		"conversation_message_ids": []int{conversationMessageID},
	})
	if err != nil {
		let.Println(err)
		return nil
	}
	if len(res.Items) == 0 {
		return nil
	}
	return &res.Items[0]
}

// conversation message id of a just sent message, by its global id from
// messages.send: newest history scan; 0 when not found
func convMsgByID(peerID, globalID int) int {
	res, err := bot.MessagesGetHistory(api.Params{
		"peer_id": peerID,
		"count":   10,
	})
	if err != nil {
		let.Println("convMsgByID", peerID, err)
		return 0
	}
	for _, tm := range res.Items { // newest first
		if tm.ID == globalID {
			return tm.ConversationMessageID
		}
	}
	let.Println("convMsgByID", peerID, globalID, "not found")
	return 0
}

// report whether a conversation message still exists, separating a genuine
// miss (false) from a network/API error (err != nil)
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

// report whether the request message a reply is based on still exists;
// cu must have MsgID != 0
func requestExists(cu customer) (bool, error) {
	return msgExists(cu.PeerID, cu.MsgID)
}

// community creator: groups.getById with a community token returns the
// token's own group (the same way vksdk NewLongPollCommunity does), then
// groups.getMembers filter=managers lists management roles
func detectOwner() (int, error) {
	res, err := bot.GroupsGetByID(nil)
	if err != nil {
		return 0, srcError(err)
	}
	if len(res.Groups) == 0 {
		return 0, errors.New("groups.getById: empty")
	}
	mans, err := bot.GroupsGetMembersFilterManagers(api.Params{
		"group_id": res.Groups[0].ID,
	})
	if err != nil {
		return 0, srcError(err)
	}
	for _, m := range mans.Items {
		if m.Role == "creator" {
			return m.ID, nil
		}
	}
	return 0, fmt.Errorf("groups.getMembers: no creator among %d managers", len(mans.Items))
}

// detect the owner with retries; 0 (not fatal) keeps the bot running
// without owner features, see the auto mode in main
func initOwner() int {
	for _, pause := range []time.Duration{0, time.Second, 5 * time.Second} {
		time.Sleep(pause)
		id, err := detectOwner()
		if err == nil {
			li.Println("community creator", id)
			return id
		}
		letf.Println("detectOwner", err)
	}
	letf.Println("creator not found, owner features disabled")
	return 0
}

// all conversation peer ids (chats and direct messages) known to the
// community, paginated; errors are logged, whatever was collected is
// returned
func conversations() []int {
	var (
		ps     []int
		offset int
	)
	for {
		res, err := bot.MessagesGetConversations(api.Params{
			"count":  200,
			"offset": offset,
		})
		if err != nil {
			let.Println("getConversations", err)
			break
		}
		for _, it := range res.Items {
			if id := it.Conversation.Peer.ID; id > 0 {
				ps = append(ps, id)
			}
		}
		offset += len(res.Items)
		if len(res.Items) == 0 || offset >= res.Count {
			break
		}
	}
	return ps
}

// owner peer for service messages and the CLI default: the first
// pinguin_ids entry, or the detected creator; 0 - nowhere to send
func ownerPeer() int {
	if len(chats) > 0 {
		return chats[0]
	}
	return ownerID
}

// send plain CLI message; the running instance sees owner dialog sends as
// message_reply longpoll events (outgoing community message), chat sends
// are not delivered, so cliSend mirrors a trigger to the owner dialog
func sendPlain(peerID int, text string) error {
	_, err := bot.MessagesSend(api.Params{
		"peer_id":   peerID,
		"message":   cliMark + text,
		"random_id": 0,
	})
	return srcError(err)
}

// one-shot send from command line: first arg is the explicit trigger peer
// id, second arg 0 for the trigger peer itself or a chat id for the target.
// Only a marked trigger with the target peer id goes to the trigger peer -
// the only peer where message_reply longpoll events fire; the running
// instance picks it up instantly and all visible output goes to the target
// peer
func cliSend(args []string) error {
	usage := fmt.Errorf("usage: %s <sourcePeerID> 0|<targetPeerID>|<targetChatID> messageWord1 ...", filepath.Base(os.Args[0]))
	if len(args) < 3 {
		return usage
	}
	trig, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("trigger peer id %q: %w\n%w", args[0], err, usage)
	}
	target := trig
	if args[1] != "0" {
		target, err = strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("peer id %q: %w\n%w", args[1], err, usage)
		}
	}
	b, err := CreateBot(envValue("pinguin_token"))
	if err != nil {
		return srcError(err)
	}
	bot = b
	return sendPlain(trig, strconv.Itoa(target)+" "+strings.Join(args[2:], " "))
}

// send service marker to the owner peer, or the error itself with 💥 if any
func sendStatus(marker string, err error) {
	if err != nil {
		SendError(err)
		return
	}
	if p := ownerPeer(); bot != nil && p > 0 {
		_, _ = bot.MessagesSend(api.Params{
			"peer_id":   p,
			"message":   marker,
			"random_id": 0,
		})
	}
}

// send error message to the owner peer: the first pinguin_ids entry or
// the detected creator
func SendError(err error) {
	if p := ownerPeer(); bot != nil && p > 0 && err != nil {
		_, _ = bot.MessagesSend(api.Params{
			"peer_id":   p,
			"message":   "💥 " + err.Error(),
			"random_id": 0,
		})
	}
}
