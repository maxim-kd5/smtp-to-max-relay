package schemes

type UploadType string

const (
	IMAGE UploadType = "image"
	VIDEO UploadType = "video"
	AUDIO UploadType = "audio"
	FILE  UploadType = "file"
)

type UpdateType string

const (
	UpdateTypeMessageCreated UpdateType = "message_created"
)

type UpdateInterface interface {
	GetType() UpdateType
}

type Update struct {
	UpdateType UpdateType `json:"update_type"`
	Timestamp  int64      `json:"timestamp"`
}

func (u *Update) GetType() UpdateType {
	if u == nil {
		return ""
	}
	return u.UpdateType
}

type MessageCreatedUpdate struct {
	Update
	Message    Message `json:"message"`
	UserLocale *string `json:"user_locale,omitempty"`
}

func (u *MessageCreatedUpdate) GetType() UpdateType {
	if u == nil {
		return ""
	}
	return u.Update.UpdateType
}

type User struct {
	UserId    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
	IsBot     bool   `json:"is_bot"`
}

type BotInfo struct {
	UserId      int64  `json:"user_id"`
	FirstName   string `json:"first_name"`
	Username    string `json:"username,omitempty"`
	IsBot       bool   `json:"is_bot"`
	Description string `json:"description,omitempty"`
}

type Recipient struct {
	UserId int64 `json:"user_id,omitempty"`
	ChatId int64 `json:"chat_id,omitempty"`
}

type MessageBody struct {
	Mid  string `json:"mid,omitempty"`
	Text string `json:"text,omitempty"`
}

type Message struct {
	Sender    *User       `json:"sender,omitempty"`
	Recipient Recipient   `json:"recipient"`
	Timestamp int64       `json:"timestamp"`
	Body      MessageBody `json:"body"`
}

type AttachmentRequest struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type NewMessageBody struct {
	Text        string              `json:"text,omitempty"`
	Attachments []AttachmentRequest `json:"attachments,omitempty"`
	Notify      bool                `json:"notify"`
}

type UploadedInfo struct {
	Token string `json:"token"`
}

type UploadEndpoint struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}
