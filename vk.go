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

// status reply: answer to a CLI request by its global message id, or forward
// the user request by conversation message id
func sendStatusReply(cu customer, text string) (int, error) {
	if cu.GlobalID > 0 {
		return bot.MessagesSend(api.Params{
			"peer_id":   cu.PeerID,
			"message":   text,
			"random_id": 0,
			"reply_to":  cu.GlobalID,
			"keyboard":  kbIP.ToJSON(),
		})
	}
	return sendKeyboard(cu.PeerID, cu.MsgID, text)
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
// cu must have MsgID != 0 or GlobalID != 0
func requestExists(cu customer) (bool, error) {
	if cu.MsgID != 0 {
		return msgExists(cu.PeerID, cu.MsgID)
	}
	return msgExistsGlobal(cu.GlobalID)
}

// report whether a message with the given global id still exists, separating
// a genuine miss (false) from a network/API error (err != nil)
func msgExistsGlobal(globalID int) (bool, error) {
	res, err := bot.MessagesGetByID(api.Params{
		"message_ids": globalID, // peer_id must stay unset: with peer_id the API expects cmids instead
	})
	if err != nil {
		return false, err
	}
	return len(res.Items) > 0, nil
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

// send service marker to first peerID, or the error itself with 💥 if any
func sendStatus(marker string, err error) {
	if err != nil {
		SendError(err)
		return
	}
	if bot != nil && len(chats) > 0 {
		_, _ = bot.MessagesSend(api.Params{
			"peer_id":   chats[0],
			"message":   marker,
			"random_id": 0,
		})
	}
}

// send error message to first peerID in args
func SendError(err error) {
	if bot != nil && len(chats) > 0 && err != nil {
		_, _ = bot.MessagesSend(api.Params{
			"peer_id":   chats[0],
			"message":   "💥 " + err.Error(),
			"random_id": 0,
		})
	}
}
