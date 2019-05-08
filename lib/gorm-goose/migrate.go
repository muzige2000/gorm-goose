package gormgoose

import (
	"errors"
	"fmt"
	"github.com/ahl5esoft/golang-underscore"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var (
	ErrTableDoesNotExist = errors.New("table does not exist")
	ErrNoPreviousVersion = errors.New("no previous version found")
)

type MigrationRecord struct {
	ID        uint      `gorm:"primary_key"`
	VersionId int64     `gorm:"unique"`
	TStamp    time.Time `gorm:"default: now()"`
	IsApplied bool      // was this a result of up() or down()
}

type Migration struct {
	Version  int64
	Next     int64  // next version, or -1 if none
	Previous int64  // previous version, -1 if none
	Source   string // path to .go or .sql script
}

type migrationSorter []*Migration

// helpers so we can use pkg sort
func (ms migrationSorter) Len() int           { return len(ms) }
func (ms migrationSorter) Swap(i, j int)      { ms[i], ms[j] = ms[j], ms[i] }
func (ms migrationSorter) Less(i, j int) bool { return ms[i].Version < ms[j].Version }

func newMigration(v int64, src string) *Migration {
	return &Migration{v, -1, -1, src}
}

func RunMigrations(conf *DBConf, migrationsDir string, target int64) (err error) {

	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return err
	}
	defer db.Close()

	return RunMigrationsOnDb(conf, migrationsDir, target, db)
}

// Runs migration on a specific database instance.
func RunMigrationsOnDb(conf *DBConf, migrationsDir string, target int64, db *gorm.DB) (err error) {
	current, err := EnsureDBVersion(conf, db)
	if err != nil {
		return err
	}

	migrations, err := CollectMigrations(migrationsDir, current, target)
	if err != nil {
		return err
	}

	direction := current < target

	if len(migrations) == 0 {
		fmt.Printf("goose: no migrations to run. current version: %d, target: %d, direction: %t\n", current, target, direction)
		return nil
	}

	ms := migrationSorter(migrations)
	ms.Sort(direction)

	fmt.Printf("goose: migrating db environment '%v', current version: %d, target: %d\n",
		conf.Env, current, target)

	for _, m := range ms {

		switch filepath.Ext(m.Source) {
		case ".go":
			err = runGoMigration(conf, m.Source, m.Version, direction)
		case ".sql":
			err = runSQLMigration(conf, db, m.Source, m.Version, direction)
		}

		if err != nil {
			return errors.New(fmt.Sprintf("FAIL %v, quitting migration", err))
		}

		fmt.Println("OK   ", filepath.Base(m.Source))
	}

	return nil
}

func RunMergeMigrations(conf *DBConf, migrationsDir string) (err error) {

	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return err
	}
	defer db.Close()

	return RunMergeMigrationsOnDb(conf, migrationsDir, db)
}

// Runs merge migration on a specific database instance.
func RunMergeMigrationsOnDb(conf *DBConf, migrationsDir string, db *gorm.DB) (err error) {
	migrationRecords, err := MigrationRecords(conf, db)
	if err != nil {
		return err
	}

	migrations, err := NeedMigrations(migrationsDir, migrationRecords)
	if err != nil {
		return err
	}

	lastMigrationRecord := underscore.Last(migrationRecords).(MigrationRecord)
	if len(migrations) == 0 {
		fmt.Printf("goose: no migrations to run. migrationRecords version: %d\n", lastMigrationRecord.VersionId)
		return nil
	}

	ms := migrationSorter(migrations)
	ms.Sort(true)

	fmt.Printf("goose: migrating db environment '%v', migrationRecords version: %d\n",
		conf.Env, lastMigrationRecord.VersionId)

	for _, m := range ms {

		switch filepath.Ext(m.Source) {
		case ".go":
			err = runGoMigration(conf, m.Source, m.Version, true)
		case ".sql":
			err = runSQLMigration(conf, db, m.Source, m.Version, true)
		}

		if err != nil {
			return errors.New(fmt.Sprintf("FAIL %v, quitting migration", err))
		}

		fmt.Println("OK   ", filepath.Base(m.Source))
	}

	return nil
}

// collect all the valid looking migration scripts in the
// migrations folder, and key them by version
func CollectMigrations(dirpath string, current, target int64) (m []*Migration, err error) {

	// extract the numeric component of each migration,
	// filter out any uninteresting files,
	// and ensure we only have one file per migration version.
	err = filepath.Walk(dirpath, func(name string, info os.FileInfo, err error) error {

		if v, e := NumericComponent(name); e == nil {

			for _, g := range m {
				if v == g.Version {
					log.Fatalf("more than one file specifies the migration for version %d (%s and %s)",
						v, g.Source, filepath.Join(dirpath, name))
				}
			}

			if versionFilter(v, current, target) {
				m = append(m, newMigration(v, name))
			}
		}

		return nil
	})

	return m, err
}

// collect all the not migrated migration scirpts in the migrations folder, and by version
func NeedMigrations(dirPath string, currentMigrations []MigrationRecord) (m []*Migration, err error) {
	res := underscore.IndexBy(currentMigrations, "VersionId").(map[int64]MigrationRecord)

	err = filepath.Walk(dirPath, func(name string, info os.FileInfo, err error) error {
		if v, e := NumericComponent(name); e == nil {
			if _, exist := res[v]; !exist {
				m = append(m, newMigration(v, name))
			}
		}
		return nil
	})
	if err != nil {
		return m, err
	}

	return m, nil
}

func versionFilter(v, current, target int64) bool {

	if target > current {
		return v > current && v <= target
	}

	if target < current {
		return v <= current && v > target
	}

	return false
}

func (ms migrationSorter) Sort(direction bool) {

	// sort ascending or descending by version
	if direction {
		sort.Sort(ms)
	} else {
		sort.Sort(sort.Reverse(ms))
	}

	// now that we're sorted in the appropriate direction,
	// populate next and previous for each migration
	for i, m := range ms {
		prev := int64(-1)
		if i > 0 {
			prev = ms[i-1].Version
			ms[i-1].Next = m.Version
		}
		ms[i].Previous = prev
	}
}

// look for migration scripts with names in the form:
//  XXX_descriptivename.ext
// where XXX specifies the version number
// and ext specifies the type of migration
func NumericComponent(name string) (int64, error) {

	base := filepath.Base(name)

	if ext := filepath.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("not a recognized migration file type")
	}

	idx := strings.Index(base, "_")
	if idx < 0 {
		return 0, errors.New("no separator found")
	}

	n, e := strconv.ParseInt(base[:idx], 10, 64)
	if e == nil && n <= 0 {
		return 0, errors.New("migration IDs must be greater than zero")
	}

	return n, e
}

// EnsureDBVersion retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func EnsureDBVersion(conf *DBConf, db *gorm.DB) (int64, error) {
	rows := make([]MigrationRecord, 0)
	err := db.Order("version_id desc").Find(&rows).Error

	if err != nil {
		return 0, createVersionTable(conf, db)
	}

	// The most recent record for each migration specifies
	// whether it has been applied or rolled back.
	// The first version we find that has been applied is the current version.

	toSkip := make([]int64, 0)

	for _, row := range rows {
		// have we already marked this version to be skipped?
		skip := false
		for _, v := range toSkip {
			if v == row.VersionId {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		// if version has been applied we're done
		if row.IsApplied {
			return row.VersionId, nil
		}

		// latest version of migration has not been applied.
		toSkip = append(toSkip, row.VersionId)
	}

	panic("failure in EnsureDBVersion()")
}

// EnsureDBVersion retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func MigrationRecords(conf *DBConf, db *gorm.DB) (ms []MigrationRecord, err error) {
	err = db.Order("version_id desc").Where("is_applied is false").Find(&ms).Error

	if err != nil {
		return ms, createVersionTable(conf, db)
	}
	return ms, err
}

// Create the goose_db_version table
// and insert the initial 0 value into it
func createVersionTable(conf *DBConf, db *gorm.DB) error {
	txn := db.Begin()
	if txn.Error != nil {
		return txn.Error
	}

	if err := txn.CreateTable(&MigrationRecord{}).Error; err != nil {
		txn.Rollback()
		return err
	}

	record := MigrationRecord{VersionId: 0, IsApplied: true}
	if err := txn.Create(&record).Error; err != nil {
		txn.Rollback()
		return err
	}

	return txn.Commit().Error
}

// wrapper for EnsureDBVersion for callers that don't already have
// their own DB instance
func GetDBVersion(conf *DBConf) (version int64, err error) {

	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return -1, err
	}
	defer db.Close()

	version, err = EnsureDBVersion(conf, db)
	if err != nil {
		return -1, err
	}

	return version, nil
}

func GetPreviousDBVersion(dirpath string, version int64) (previous int64, err error) {

	previous = -1
	sawGivenVersion := false

	err = filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > previous && v < version {
					previous = v
				}
				if v == version {
					sawGivenVersion = true
				}
			}
		}

		return nil
	})

	if previous == -1 {
		if sawGivenVersion {
			// the given version is (likely) valid but we didn't find
			// anything before it.
			// 'previous' must reflect that no migrations have been applied.
			previous = 0
		} else {
			err = ErrNoPreviousVersion
		}
	}

	return
}

// helper to identify the most recent possible version
// within a folder of migration scripts
func GetMostRecentDBVersion(dirpath string) (version int64, err error) {

	version = -1

	err = filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {
		if walkerr != nil {
			return walkerr
		}

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > version {
					version = v
				}
			}
		}

		return nil
	})

	if version == -1 {
		err = errors.New("no valid version found")
	}

	return
}

func CreateMigration(name, migrationType, dir string, t time.Time) (path string, err error) {

	if migrationType != "go" && migrationType != "sql" {
		return "", errors.New("migration type must be 'go' or 'sql'")
	}

	timestamp := t.Format("20060102150405")
	filename := fmt.Sprintf("%v_%v.%v", timestamp, name, migrationType)

	fpath := filepath.Join(dir, filename)

	var tmpl *template.Template
	if migrationType == "sql" {
		tmpl = sqlMigrationTemplate
	} else {
		tmpl = goMigrationTemplate
	}

	path, err = writeTemplateToFile(fpath, tmpl, timestamp)

	return
}

// FinalizeMigration update the version table for the given migration,
// and finalize the transaction.
func FinalizeMigration(conf *DBConf, txn *gorm.DB, direction bool, v int64) error {

	// XXX: drop goose_db_version table on some minimum version number?
	record := MigrationRecord{}
	txn.FirstOrCreate(&record, MigrationRecord{VersionId: v})
	record.IsApplied = direction

	if err := txn.Save(&record).Error; err != nil {
		txn.Rollback()
		return err
	}

	return txn.Commit().Error
}

var goMigrationTemplate = template.Must(template.New("goose.go-migration").Parse(`
package main

import (
	"github.com/jinzhu/gorm"
)

// Up is executed when this migration is applied
func Up_{{ . }}(txn *gorm.DB) {

}

// Down is executed when this migration is rolled back
func Down_{{ . }}(txn *gorm.DB) {

}
`))

var sqlMigrationTemplate = template.Must(template.New("goose.sql-migration").Parse(`
-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied


-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
`))
