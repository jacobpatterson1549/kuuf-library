# kuuf-library

A site to display books.

## about

Books are loaded from a database and displayed on the list page.
When a user clicks on a book, more information is shown about it, including a picture.
The administrator for the site can edit information about books and add and remove them.

## server

The server defaults to run on port 8000.
This can be configured by setting the `PORT` environment variable or the `-port` application argument.
All application arguments are attempted to be read as environment variables that are the same name, but all uppercase.
All environment variables use the same hyphens as application arguments, excluding the leading hyphen.

## database

### CSV

The library can run on an internal, readonly CSV database.  This database can also be used initialize other databases with the `-csv-backfill` application argument.

### Postgres

A Postgres database will be used by setting the `-database-URL` application argument or the `DATABASE-URL` environment variable.
The script below initializes a Postgres user and database.
Remember to set the password.
(a password can be generated with `openssl rand --hex 10`)

```bash
PGDATABASE="kuuf_library_db" \
PGUSER="elle" \
PGPASSWORD=""REPLACE-THIS \
PGHOSTADDR="127.0.0.1" \
PGPORT="5432" \
sh -c ' \
sudo -u postgres psql \
-c "CREATE DATABASE $PGDATABASE" \
-c "CREATE USER $PGUSER WITH ENCRYPTED PASSWORD '"'"'$PGPASSWORD'"'"'" \
-c "GRANT ALL PRIVILEGES ON DATABASE $PGDATABASE TO $PGUSER" \
&& echo DATABASE-URL=postgres://$PGUSER:$PGPASSWORD@$PGHOSTADDR:$PGPORT/$PGDATABASE'```
