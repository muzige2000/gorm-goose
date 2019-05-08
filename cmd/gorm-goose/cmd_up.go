package main

import (
	"github.com/muzige2000/gorm-goose/lib/gorm-goose"
	"log"
)

var upCmd = &Command{
	Name:    "up",
	Usage:   "",
	Summary: "Migrate the DB to the most recent version available",
	Help:    `up extended help here...`,
	Run:     upRun,
}

func upRun(cmd *Command, args ...string) {

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	target, err := gormgoose.GetMostRecentDBVersion(conf.MigrationsDir)
	if err != nil {
		log.Fatal(err)
	}

	if err := gormgoose.RunMigrations(conf, conf.MigrationsDir, target); err != nil {
		log.Fatal(err)
	}
}
