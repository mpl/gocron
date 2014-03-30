package main

import (
	"flag"
	"log"
	"os/exec"
	"time"

	"github.com/mpl/gocron"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("need a command as argument")
	}
	job := func() error {
		cmd := exec.Command(args[0], args[1:]...)
		err := cmd.Run()
		time.Sleep(time.Second)
		return err
	}
	cron := gocron.Cron{
		Interval: time.Minute,
		Job:      job,
		Mail: &gocron.MailAlert{
			Subject: "Gocron error test",
			To:      []string{"pony@foo.com"},
			From:    "unicorn@bar.com",
			SMTP:    "localhost:25",
		},
		Notif: &gocron.Notification{
			Host:    "localhost:8082",
			Msg:     "job error",
			Timeout: 10 * time.Second,
		},
		File: &gocron.StaticFile{
			Path: "gocron.log",
			Msg:  "gocron error",
		},
	}
	cron.Run()
}
