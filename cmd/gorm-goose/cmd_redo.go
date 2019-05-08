package main

import (
	"log"
)

var redoCmd = &Command{
	Name:    "redo",
	Usage:   "",
	Summary: "Re-run the latest migration",
	Help:    `redo extended help here...`,
	Run:     redoRun,
}

func redoRun(cmd *Command, args ...string) {
	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	current, err := pkg.GetDBVersion(conf)
	if err != nil {
		log.Fatal(err)
	}

	previous, err := pkg.GetPreviousDBVersion(conf.MigrationsDir, current)
	if err != nil {
		log.Fatal(err)
	}

	if err := pkg.RunMigrations(conf, conf.MigrationsDir, previous); err != nil {
		log.Fatal(err)
	}

	if err := pkg.RunMigrations(conf, conf.MigrationsDir, current); err != nil {
		log.Fatal(err)
	}
}
