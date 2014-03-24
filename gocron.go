package gocron

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os/exec"
	"time"
)

// TODO(mpl): write to tmp file if File == nil

type Cron struct {
	Interval time.Duration
	Job      func() error
	Mail     *MailAlert
	Notif    *Notification
	File     *StaticFile // Path to a file where the alert will be written when the above methods fail.
}

func (c *Cron) Run() {
	c.Notif.init()
	for {
		if err := c.Job(); err != nil {
			if err := c.Mail.Send(err); err != nil {
				mailFail := fmt.Errorf("Could not send mail alert %q: %v",
					c.Mail.Msg(), err)
				if err := c.File.WriteAlert(mailFail); err != nil {
					log.Fatal(err)
				}
			}
			if err := c.Notif.Send(); err != nil {
				notiFail := fmt.Errorf("Could not open notification: %v", err)
				if err := c.File.WriteAlert(notiFail); err != nil {
					log.Fatal(err)
				}
			}
		}
		time.Sleep(c.Interval)
	}
}

type MailAlert struct {
	Subject string
	msg     string
	To      []string
	From    string
	SMTP    string
}

func (m *MailAlert) Msg() string {
	return m.msg
}

// Send connects to the server at addr, authenticates with the
// optional mechanism a if possible, and then sends an email from
// address from, to addresses to, with message msg.
func (m *MailAlert) Send(alert error) error {
	if m == nil {
		return nil
	}
	m.msg = fmt.Sprintf("%s\n\n%v", m.Subject, alert)

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
	_, err = w.Write([]byte(m.msg))
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

const idstring = "http://golang.org/pkg/http/#ListenAndServe"

var tpl *template.Template

type Notification struct {
	Host    string
	Msg     string
	Timeout time.Duration
}

func (n *Notification) init() {
	if n == nil {
		return
	}
	tpl = template.Must(template.New("main").Parse(mainHTML(n)))
	http.HandleFunc("/", mainHandler)
	go func() {
		if err := http.ListenAndServe(n.Host, nil); err != nil {
			log.Fatalf("Could not start http server for notifications: %v", err)
		}
	}()
}

func (n *Notification) Send() error {
	if n == nil {
		return nil
	}
	url := "http://" + n.Host
	return exec.Command("xdg-open", url).Run()
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Server", idstring)
	if err := tpl.Execute(w, nil); err != nil {
		log.Printf("Could not execute template: %v", err)
	}
}

func mainHTML(n *Notification) string {
	s := `<!DOCTYPE HTML>
<html>
	<head>
		<title>Reminder</title>
	</head>

	<body>
	<script>
`

	if n.Timeout > 0 {
		windowTimeout := n.Timeout + time.Duration(3)*time.Second
		s = fmt.Sprintf("%s\n\tsetTimeout(window.close, %d);",
			s, int64(windowTimeout/time.Millisecond))
	}

	s = s + `
window.onload=function(){notify()};

function enableNotify() {
	if (!(window.webkitNotifications)) {
		alert("Notifications not supported on this browser.");
		return;
	}
	var havePermission = window.webkitNotifications.checkPermission();
	if (havePermission == 0) {
		alert("Notifications already allowed.");
		return;
	}
	window.webkitNotifications.requestPermission();
}

function notify() {
	if (!(window.webkitNotifications)) {
		console.log("Notifications not supported");
		return;
	}
	var havePermission = window.webkitNotifications.checkPermission();
	if (havePermission != 0) {
		console.log("Notifications not allowed.");
		return;
	}
	var notification = window.webkitNotifications.createNotification(
		'',
		'gocron notification',
		'` + n.Msg + `'
	);

	// NOTE: the tab/window needs to be still open for the cancellation
	// of the notification to work.
	notification.onclick = function () {
		this.cancel();
	};
`
	if n.Timeout > 0 {
		sTimeout := fmt.Sprintf("%d", int64(n.Timeout/time.Millisecond))
		s = s + `
	notification.ondisplay = function(event) {
		setTimeout(function() {
			event.currentTarget.cancel();
		}, ` + sTimeout + `);
	};
`
	}

	s = s + `
	notification.show();
} 

	</script>

	<a id="notifyLink" href="#" onclick="enableNotify();return false;">Enable notifications?</a>

	<h2> Bazinga </h2>
	</body>
</html>
`

	return s
}
