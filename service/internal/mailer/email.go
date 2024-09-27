package mailer

import "github.com/wneessen/go-mail"

type Email struct {
	to      string
	from    string
	subject string
	body    string
}

func (e *Email) To() string {
	return e.to
}

func (e *Email) SetTo(to string) {
	e.to = to
}

func (e *Email) From() string {
	return e.from
}

func (e *Email) SetFrom(from string) {
	e.from = from
}

func (e *Email) Subject() string {
	return e.subject
}

func (e *Email) SetSubject(subject string) {
	e.subject = subject
}

func (e *Email) Body() string {
	return e.body
}

func (e *Email) SetBody(body string) {
	e.body = body
}

func (e *Email) ToMessage() (*mail.Msg, error) {
	msg :=
		mail.NewMsg()

	err := msg.From(e.from)
	if err != nil {
		return nil, err
	}

	err = msg.To(e.to)

	if err != nil {
		return nil, err
	}

	msg.Subject(e.subject)
	msg.SetBodyString("text/plain", e.body)

	return msg, nil
}

func NewEmail(subject, body string) *Email {
	return &Email{
		subject: subject,
		body:    body,
	}
}
