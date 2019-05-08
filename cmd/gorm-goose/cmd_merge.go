package main

import (
	"github.com/muzige2000/gorm-goose/lib/gorm-goose"
	"log"
)

var mergeCmd = &Command{
	Name:    "merge",
	Usage:   "merge",
	Summary: "Migrate the DB to version available",
	Help:    `merge extended help here...`,
	Run:     mergeRun,
}

func mergeRun(cmd *Command, args ...string) {
	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	if err := gormgoose.RunMergeMigrations(conf, conf.MigrationsDir); err != nil {
		log.Fatal(err)
	}

}
