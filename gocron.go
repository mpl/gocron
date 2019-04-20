// Copyright 2018 Mathieu Lonjaret

// Package gocron allows to regularly run a job, and to be notified,
// when that run failed. Notifications can happen through e-mail, browser
// notifications, or a local file.
package gocron

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// TODO(mpl): fix notifications.
// TODO(mpl): docs
// TODO(mpl): option to skip running if previous run is still running.
// Activity detection as well? probably not.

type Cron struct {
	Interval time.Duration
	LifeTime time.Duration // if set, we (and our webserver) exit after this time
	Job      func() error
	Mail     *MailAlert
	Notif    *Notification
	File     *StaticFile
}

func (c *Cron) Run() {
	start := time.Now()
	// TODO(mpl): maybe give the option to not have a file? meh.
	c.File = c.File.init()
	c.Notif.init()
	mailchan := make(chan struct{})
	for {
		if jobErr := c.Job(); jobErr != nil {
			if err := c.Notif.Send(jobErr); err != nil {
				notiFail := fmt.Errorf("Could not open notification: %v", err)
				if err := c.File.WriteAlert(notiFail); err != nil {
					log.Fatal(err)
				}
			}
			if err := c.File.WriteAlert(jobErr); err != nil {
				log.Fatal(err)
			}
			// TODO(mpl): c.Mail.Send indeed does check that c.Mail is not nil, but I don't
			// want the time out message in the log if we did not even try to send e-mail.
			// Better fix later.
			if c.Mail != nil {
				go func() {
					if err := c.Mail.Send(jobErr); err != nil {
						mailFail := fmt.Errorf("Could not send mail alert %q: %v",
							c.Mail.Msg(), err)
						if err := c.File.WriteAlert(mailFail); err != nil {
							log.Fatal(err)
						}
						mailchan <- struct{}{}
					}
				}()
				select {
				case <-mailchan:
				case <-time.After(10 * time.Second):
					mailFail := fmt.Errorf("timed out sending mail alert %q", c.Mail.Msg())
					c.File.WriteAlert(mailFail)
				}
			}

		}
		// TODO(mpl): maybe remove this, now that we have LifeTime. But it is breaking,
		// so think about it.
		if c.Interval == 0 {
			break
		}
		time.Sleep(c.Interval)
		if time.Now().After(start.Add(c.LifeTime)) {
			return
		}
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
	if m == nil {
		return ""
	}
	return m.msg
}

func (m *MailAlert) Send(alert error) error {
	if m == nil {
		return nil
	}
	m.msg = fmt.Sprintf("Subject: %s\nFrom: %s\n\n%v", m.Subject, m.From, alert)

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

func (s *StaticFile) init() *StaticFile {
	if s == nil || s.Path == "" {
		tempFile, err := ioutil.TempFile("", "gocron")
		if err != nil {
			log.Fatal("Could not create temp file for static file alerts: %v", err)
		}
		return &StaticFile{Path: tempFile.Name()}
	}
	return s
}

func (s *StaticFile) WriteAlert(jobErr error) error {
	// TODO(mpl): use s.Msg as logger prefix maybe
	//	s.Msg = fmt.Sprintf("%s %v\n", s.Msg, err)
	f, err := os.OpenFile(s.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0700)
	if err != nil {
		return fmt.Errorf("could not open or create file %v: %v", s.Path, err)
	}
	defer f.Close()
	log.SetOutput(f)
	log.Printf("%v", jobErr)
	return nil
}

const idstring = "http://golang.org/pkg/http/#ListenAndServe"

type Notification struct {
	Host          string
	Msg           string
	Timeout       time.Duration // if set, we close the tab after this duration
	tpl           *template.Template
	pageBody      string
	windowTimeout int64
	notiTimeout   int64
}

func (n *Notification) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := struct {
		Noti          string
		Body          string
		WindowTimeout int64
	}{
		Noti:          n.Msg,
		Body:          n.pageBody,
		WindowTimeout: n.windowTimeout,
	}
	w.Header().Set("Server", idstring)
	if err := n.tpl.Execute(w, &data); err != nil {
		println(fmt.Sprintf("Could not execute template: %v", err))
		//		log.Printf("Could not execute template: %v", err)
	}
}

func (n *Notification) init() {
	if n == nil {
		return
	}
	if n.Timeout > 0 {
		n.windowTimeout = int64((n.Timeout + time.Duration(3)*time.Second) / time.Millisecond)
		n.notiTimeout = int64(n.Timeout / time.Millisecond)
	}

	n.tpl = template.Must(template.New("main").Parse(mainHTML()))
	mux := http.NewServeMux()
	mux.Handle("/", n)
	hostc := make(chan struct{})
	go func() {
		addr, err := net.ResolveTCPAddr("tcp", n.Host)
		if err != nil {
			log.Fatal(err)
		}
		listener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			log.Fatal(err)
		}
		n.Host = listener.Addr().String()
		// TODO(mpl): I dont think this fake synchro is actually useful.
		hostc <- struct{}{}
		if err := http.Serve(listener, mux); err != nil {
			log.Fatalf("Could not start http server for notifications: %v", err)
		}
	}()
	<-hostc
}

func (n *Notification) Send(err error) error {
	if n == nil {
		return nil
	}
	n.pageBody = fmt.Sprintf("%v", err)
	url := "http://" + n.Host
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, url).Run()
}

func mainHTML() string {
	s := `<!DOCTYPE HTML >
<html>
	<head>
		<title>Reminder</title>
	</head>

	<body>
	<script>

	{{if .WindowTimeout}}
setTimeout(window.close, {{.WindowTimeout}});
	{{end}}
window.onload=function(){notify('{{.Noti}}')};

function notify(notiBody) {
	if (!("Notification" in window)) {
		console.log("Notifications not supported on this browser.");
		return;
	}

	// Let's check whether notification permissions have already been granted
	if (Notification.permission === "granted") {
		// If it's okay let's create a notification
		var notification = new Notification('gocron notification', { body: notiBody});
//		var notification = new Notification('gocron notification', { body: 'blabla'});
		return;
	}

	// Otherwise, we need to ask the user for permission
	if (Notification.permission !== "denied") {
		Notification.requestPermission().then(function (permission) {
			// If the user accepts, let's create a notification
			if (permission === "granted") {
				var notification = new Notification('gocron notification', { body: notiBody});
//				var notification = new Notification('gocron notification', { body: 'blabla'});
			} else {
				console.log("Notifications are denied.");
			}
		});
	}
} 

	</script>

	<a id="notifyLink" href="#" onclick="notify('notifications are enabled');return false;">Enable notifications?</a>

	<h2> {{.Noti}} </h2>
	{{.Body}}
	</body>
</html>
`

	return s
}
