package gocron

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/smtp"
	"time"
)

type Mail struct {
	Msg  string
	To   []string
	From string
	SMTP string
}

// SendMail connects to the server at addr, authenticates with the
// optional mechanism a if possible, and then sends an email from
// address from, to addresses to, with message msg.
func (m *Mail) Send() error {
	if m == nil {
		return nil
	}
	c, err := smtp.Dial(m.SMTP)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Mail(m.From); err != nil {
		return err
	}
	for _, rcpt := range m.To {
		if err = c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(m.Msg))
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

type StaticFile struct {
	Path string
	Msg  string
}

func (s *StaticFile) WriteAlert(err error) error {
	s.Msg = fmt.Sprintf("%s\n\n%v", s.Msg, err)
	return ioutil.WriteFile(s.Path, []byte(s.Msg), 0600)
}

type Cron struct {
	Interval  time.Duration
	Job       func() error
	MailAlert *Mail
	//	Notification
	File *StaticFile // Path to a file where the alert will be written when the above methods fail.
}

func (c *Cron) Run() {
	for {
		if err := c.Job(); err != nil {
			c.MailAlert.Msg = fmt.Sprintf("%s\n\n%v", c.MailAlert.Msg, err)
			if err := c.MailAlert.Send(); err != nil {
				if err := c.File.WriteAlert(err); err != nil {
					log.Println(err)
				}
			}
			// browser notification too
		}
		time.Sleep(c.Interval)
	}
}
