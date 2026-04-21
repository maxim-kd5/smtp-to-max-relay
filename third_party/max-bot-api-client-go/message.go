package maxbot

import "github.com/max-messenger/max-bot-api-client-go/schemes"

type Message struct {
	userID int64
	chatID int64

	message *schemes.NewMessageBody
}

func NewMessage() *Message {
	return &Message{
		message: &schemes.NewMessageBody{
			Notify:      true,
			Attachments: make([]schemes.AttachmentRequest, 0),
		},
	}
}

func (m *Message) SetUser(userID int64) *Message {
	m.userID = userID
	return m
}

func (m *Message) SetChat(chatID int64) *Message {
	m.chatID = chatID
	return m
}

func (m *Message) SetText(text string) *Message {
	m.message.Text = text
	return m
}

func (m *Message) SetNotify(notify bool) *Message {
	m.message.Notify = notify
	return m
}

func (m *Message) AddFile(file *schemes.UploadedInfo) *Message {
	if file == nil {
		return m
	}
	m.message.Attachments = append(m.message.Attachments, schemes.AttachmentRequest{
		Type:    string(schemes.FILE),
		Payload: file,
	})
	return m
}
