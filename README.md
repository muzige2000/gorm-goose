# gorm-goose

This is a fork of [https://bitbucket.org/liamstask/goose](https://bitbucket.org/liamstask/goose) for [gorm](https://github.com/jinzhu/gorm).

gorm-goose is a database migration tool for [gorm](https://github.com/jinzhu/gorm).
Currently, available drivers are: "postgres", "mysql", or "sqlite3".

You can manage your database's evolution by creating incremental SQL or Go scripts.

# Install

    $ go get github.com/muzige2000/gorm-goose/cmd/gorm-goose

This will install the `gorm-goose` binary to your `$GOPATH/bin` directory.

You can also build gorm-goose into your own applications by importing `github.com/muzige2000/gorm-goose/lib/gorm-goose`.

# Usage

gorm-goose provides several commands to help manage your database schema.

## create

Create a new Go migration.

    $ gorm-goose create AddSomeColumns
    $ goose: created db/migrations/20130106093224_AddSomeColumns.go

Edit the newly created script to define the behavior of your migration.

You can also create an SQL migration:

    $ gorm-goose create AddSomeColumns sql
    $ goose: created db/migrations/20130106093224_AddSomeColumns.sql

## up

Apply all available migrations.

    $ gorm-goose up
    $ goose: migrating db environment 'development', current version: 0, target: 3
    $ OK    001_basics.sql
    $ OK    002_next.sql
    $ OK    003_and_again.go

### option: pgschema

Use the `pgschema` flag with the `up` command specify a postgres schema.

    $ gorm-goose -pgschema=my_schema_name up
    $ goose: migrating db environment 'development', current version: 0, target: 3
    $ OK    001_basics.sql
    $ OK    002_next.sql
    $ OK    003_and_again.go

## down

Roll back a single migration from the current version.

    $ gorm-goose down
    $ goose: migrating db environment 'development', current version: 3, target: 2
    $ OK    003_and_again.go

## redo

Roll back the most recently applied migration, then run it again.

    $ gorm-goose redo
    $ goose: migrating db environment 'development', current version: 3, target: 2
    $ OK    003_and_again.go
    $ goose: migrating db environment 'development', current version: 2, target: 3
    $ OK    003_and_again.go

## status

Print the status of all migrations:

    $ gorm-goose status
    $ goose: status for environment 'development'
    $   Applied At                  Migration
    $   =======================================
    $   Sun Jan  6 11:25:03 2013 -- 001_basics.sql
    $   Sun Jan  6 11:25:03 2013 -- 002_next.sql
    $   Pending                  -- 003_and_again.go

## dbversion

Print the current version of the database:

    $ gorm-goose dbversion
    $ goose: dbversion 002


`gorm-goose -h` provides more detailed info on each command.


# Migrations

gorm-goose supports migrations written in SQL or in Go - see the `gorm-goose create` command above for details on how to generate them.

## SQL Migrations

A sample SQL migration looks like:

```sql
-- +goose Up
CREATE TABLE post (
    id int NOT NULL,
    title text,
    body text,
    PRIMARY KEY(id)
);

-- +goose Down
DROP TABLE post;
```

Notice the annotations in the comments. Any statements following `-- +goose Up` will be executed as part of a forward migration, and any statements following `-- +goose Down` will be executed as part of a rollback.

By default, SQL statements are delimited by semicolons - in fact, query statements must end with a semicolon to be properly recognized by goose.

More complex statements (PL/pgSQL) that have semicolons within them must be annotated with `-- +goose StatementBegin` and `-- +goose StatementEnd` to be properly recognized. For example:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION histories_partition_creation( DATE, DATE )
returns void AS $$
DECLARE
  create_query text;
BEGIN
  FOR create_query IN SELECT
      'CREATE TABLE IF NOT EXISTS histories_'
      || TO_CHAR( d, 'YYYY_MM' )
      || ' ( CHECK( created_at >= timestamp '''
      || TO_CHAR( d, 'YYYY-MM-DD 00:00:00' )
      || ''' AND created_at < timestamp '''
      || TO_CHAR( d + INTERVAL '1 month', 'YYYY-MM-DD 00:00:00' )
      || ''' ) ) inherits ( histories );'
    FROM generate_series( $1, $2, '1 month' ) AS d
  LOOP
    EXECUTE create_query;
  END LOOP;  -- LOOP END
END;         -- FUNCTION END
$$
language plpgsql;
-- +goose StatementEnd
```

## Go Migrations

A sample Go migration looks like:

```go
package main

import (
	"fmt"
	"github.com/jinzhu/gorm"
)

func Up_20130106222315(txn *gorm.DB) {
	fmt.Println("Hello from migration 20130106222315 Up!")
}

func Down_20130106222315(txn *gorm.DB) {
	fmt.Println("Hello from migration 20130106222315 Down!")
}

```

`Up_20130106222315()` will be executed as part of a forward migration, and `Down_20130106222315()` will be executed as part of a rollback.

The numeric portion of the function name (`20130106222315`) must be the leading portion of migration's filename, such as `20130106222315_descriptive_name.go`. `gorm-goose create` does this by default.

A transaction is provided, rather than the DB instance directly, since gorm-goose also needs to record the schema version within the same transaction. Each migration should run as a single transaction to ensure DB integrity, so it's good practice anyway.


# Configuration

gorm-goose expects you to maintain a folder (typically called "db"), which contains the following:

* a `dbconf.yml` file that describes the database configurations you'd like to use
* a folder called "migrations" which contains `.sql` and/or `.go` scripts that implement your migrations

You may use the `-path` option to specify an alternate location for the folder containing your config and migrations.

A sample `dbconf.yml` looks like

```yml
development:
    driver: postgres
    open: user=liam dbname=tester sslmode=disable
```

Here, `development` specifies the name of the environment, and the `driver` and `open` elements are passed directly to database/sql to access the specified database.

You may include as many environments as you like, and you can use the `-env` command line option to specify which one to use. gorm-goose defaults to using an environment called `development`.

gorm-goose will expand environment variables in the `open` element. For an example, see the Heroku section below.

## Using goose with Heroku

These instructions assume that you're using [Keith Rarick's Heroku Go buildpack](https://github.com/kr/heroku-buildpack-go). First, add a file to your project called (e.g.) `install_goose.go` to trigger building of the goose executable during deployment, with these contents:

```go
// use build constraints to work around http://code.google.com/p/go/issues/detail?id=4210
// +build heroku

// note: need at least one blank line after build constraint
package main

import _ "github.com/muzige2000/gorm-goose/cmd/gorm-goose"
```

[Set up your Heroku database(s) as usual.](https://devcenter.heroku.com/articles/heroku-postgresql)

Then make use of environment variable expansion in your `dbconf.yml`:

```yml
production:
    driver: postgres
    open: $DATABASE_URL
```

To run gorm-goose in production, use `heroku run`:

    heroku run gorm-goose -env production up
