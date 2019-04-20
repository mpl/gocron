package main

import (
	//	"os/exec"
	"errors"
	"time"

	"github.com/mpl/gocron"
)

func main() {
	job := func() error {
		return errors.New("syncblobs -interval=0 -askauth=true -debug=true")
		// return exec.Command("open", "http://localhost:8082").Run()
	}
	cron := gocron.Cron{
		Interval: 1 * time.Minute,
		LifeTime: 30 * time.Second, // so it will actually die before the next run
		Job:      job,
		Notif: &gocron.Notification{
			Host:    "localhost:8082",
			Msg:     "Syncblobs reminder",
			Timeout: 5 * time.Minute,
		},
	}
	cron.Run()
}
