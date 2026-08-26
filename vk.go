package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/object"
)

// create VK community bot client from token
func CreateBot(token string) (*api.VK, error) {
	if token == "" {
		return nil, errors.New("set TOKEN=VK_COMMUNITY_TOKEN")
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
			AddCallbackButton("⏹️🏓", "⏹️🏓", "secondary").
			AddCallbackButton("⏹️🏓▶️", "⏹️🏓▶️", "secondary")
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

// delete message by conversation message id
func deleteMessage(peerID, messageID int) error {
	_, err := bot.MessagesDelete(api.Params{
		"peer_id":        peerID,
		"message_ids":    messageID,
		"delete_for_all": 1,
		"spam":           0,
	})
	return err
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
