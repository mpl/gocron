package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mpl/gocron"
)

func skipToday() (bool, error) {
	doneFile := filepath.Join(os.Getenv("$HOME"), ".done")
	fi, err := os.Stat(doneFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	// if .done file was not created today, we ignore it and remove it
	// this breaks if it was created the same day of another month, but I don't
	// really care
	today := time.Now().Day()
	doneDay := fi.ModTime().Day()
	if doneDay != today {
		if err := os.Remove(doneFile); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func main() {
	if len(os.Args) > 1 {
		println("syncblobs -interval=0 -askauth=true -debug=true")
		os.Exit(1)
	}
	notToday, err := skipToday()
	if err != nil {
		log.Fatal(err)
	}
	if notToday {
		return
	}
	job := func() error {
		return errors.New("syncblobs -interval=0 -askauth=true -debug=true")
	}
	cron := gocron.Cron{
		Interval: 1 * time.Minute,
		LifeTime: 30 * time.Second, // so it will actually die before the next run
		Job:      job,
		Notif: &gocron.Notification{
			Host:    "localhost:8082",
			Msg:     "Syncblobs reminder",
			Timeout: 5 * time.Minute, // how long the browser tab will remain open
		},
	}
	cron.Run()
}
