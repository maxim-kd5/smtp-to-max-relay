package email

type ParsedEmail struct {
	Subject     string
	From        string
	To          []string
	TextBody    string
	HTMLBody    string
	Attachments []Attachment
	MessageID   string
}

type Attachment struct {
	Filename    string
	ContentType string
	SizeBytes   int64
	Data        []byte
}
