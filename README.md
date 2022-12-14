# kuuf-library


[![Docker Image CI](https://github.com/jacobpatterson1549/kuuf-library/actions/workflows/docker-image.yml/badge.svg)](https://github.com/jacobpatterson1549/kuuf-library/actions/workflows/docker-image.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jacobpatterson1549/kuuf-library)](https://goreportcard.com/report/github.com/jacobpatterson1549/kuuf-library)
[![GoDoc](https://godoc.org/github.com/jacobpatterson1549/kuuf-library?status.svg)](https://godoc.org/github.com/jacobpatterson1549/kuuf-library)

A site to display books.

## about

Books are loaded from a database and displayed on the list page.
When a user clicks on a book title, more information is shown, including a picture.
The library administrator can create and update book listings.

## running

### Docker

It is easiest to run the application using [Docker](https://github.com/docker/docker) and [docker-compose](https://github.com/docker/compose).
This bundles the application into a small image (~30 MB).
The image is built to use SQLite for storing data.

1. The only configuration required is to create a `.env` file.
It contains environment variable mappings that are read by `docker-compose`.
Values are required for `PORT` and `ADMIN_PASSWORD`.
The port used by Docker is forwarded to through to the host.
    ```
    PORT=8000
    ADMIN_PASSWORD=
    ```

1. In a *separate* terminal, build and start the application with `docker-compose up web`.
Building the application might take a few minutes because the sqlite driver requires CGO.
This starts the server on the port specified by `PORT`.
The initial admin password is specified by `ADMIN_PASSWORD`.
This is what the administrator of the library uses to create and update books.

Stop the application by running `docker-compose down` in a separate terminal.
This can also be accomplished by pressing `Ctrl-C` in the terminal the app was started with.

The database can be initialized with the `CSV_BACKFILL=true` environment variable.
Edit [internal/server/resources/library.csv](internal/server/resources/library.csv), with one row for each book.
The application may need to be rebuilt by Docker: `docker-compose up web --build`.

### localhost

Build the application using the `make` command.
The server defaults to run on port 8000.
This can be configured by setting the `PORT` environment variable or the `-port` application argument.
All application arguments are attempted to be read as environment variables.
Environment variables have the same name as application arguments, but are uppercase and have underscores instead of hyphens.
Further information about the application can be accessed by running it with the `-h` argument.

### database

#### CSV

By default, the library runs on an internal, readonly, CSV database.
This database can also be used initialize other databases with the `-csv-backfill` application argument.

#### MongoDB

A MongoDB database can be used.
Do this by setting the `-database-URL` application argument or the `DATABASE_URL` environment variable.
The database url should begin with `mongodb+srv://` for the connection to work.

#### SQLite

A SQLite database can be used.
Do this by setting the `-database-URL` application argument or the `DATABASE_URL` environment variable.
The code has been testing with sqlite3 version 3.37.
The database url should be like `file:library.db` for the connection to use the `library.db` file in the same folder as the application.
To use an absolute to the path to the database file, set the database url to `file://localhost/home/username/library.db` to reference `/home/username/library.db`.

#### Postgres

A Postgres database can be used.
Do this by setting the `-database-URL` application argument or the `DATABASE_URL` environment variable.
The script below initializes a Postgres user and database.
It is a Bash script for Linux.
If on Ubuntu/Debian, install a server for local use with `sudo apt install postgresql`.
Remember to set the password.
A random password can be generated with `openssl rand --hex 10`.

```bash
PGDATABASE="kuuf_library_db" \
PGUSER="kuuf-library" \
PGPASSWORD="" \
PGHOSTADDR="127.0.0.1" \
PGPORT="5432" \
sh -c ' \
sudo -u postgres psql \
-c "CREATE DATABASE $PGDATABASE" \
-c "CREATE USER $PGUSER WITH ENCRYPTED PASSWORD '"'"'$PGPASSWORD'"'"'" \
-c "GRANT ALL PRIVILEGES ON DATABASE $PGDATABASE TO $PGUSER" \
&& echo DATABASE-URL=postgres://$PGUSER:$PGPASSWORD@$PGHOSTADDR:$PGPORT/$PGDATABASE'```
