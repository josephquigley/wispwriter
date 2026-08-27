# Running WriteFreely in Docker

One image, one layout. The binary goes where binaries go, the assets
where read-only shared data goes, and everything the instance writes into
a single state directory.

| Path | Holds |
|---|---|
| `/usr/bin/writefreely` | the binary |
| `/usr/share/writefreely/` | `templates/`, `static/`, `pages/`, read-only |
| `/var/lib/writefreely/` | `config.ini`, `keys/`, the SQLite database if used, `uploads/` |

That state directory is the only thing to back up, and the only volume
either compose file mounts for the application. The container runs as uid
1000 and its working directory is the state directory, so the binary
finds `config.ini` without a `-c` flag.

Put uploads there too, rather than leaving them under the asset tree:

```ini
[uploads]
enabled = true
dir = /var/lib/writefreely/uploads
```

The entrypoint generates encryption keys when they are absent, creates
the schema on a first run and applies pending migrations, then runs the
server.

## First run, development stack

```sh
cp .env.example .env
$EDITOR .env                 # set MYSQL_PASSWORD and MYSQL_ROOT_PASSWORD
touch config.ini             # see the warning below
docker compose up -d
```

`config.ini` must exist as a *file* before the first `up`. Docker creates
a **directory** in its place if it does not, and the container then fails
in a way that looks unrelated. If that happens, `rm -rf config.ini`,
`touch config.ini`, and recreate.

The web container will exit on its first start, reporting that it has no
configuration. That is expected. Generate one:

```sh
docker compose run --rm writefreely-web cmd/writefreely/writefreely --config
```

Answer `Production, behind reverse proxy` unless you know otherwise, and
give the database section these values, matching your `.env`:

```
Host      writefreely-db
Port      3306
Database  writefreely
Username  writefreely
Password  <MYSQL_PASSWORD>
```

Then create your admin account and start the stack. The schema is created
automatically on the first start, so there is no separate init step:

```sh
docker compose up -d
docker compose exec writefreely-web \
    cmd/writefreely/writefreely --create-admin youruser:yourpassword
```

WriteFreely is now on <http://localhost:8080>.

## First run, production stack

```sh
cp .env.example .env
$EDITOR .env
mkdir -p data dbdata
sudo chown -R 1000:1000 data     # the app image runs as uid 1000
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml run --rm app writefreely --config
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml exec app \
    writefreely --create-admin youruser:yourpassword
```

The database host is `db`, and everything the instance owns lives in
`./data`.

Both stacks use LinuxServer's MariaDB image. Its `PUID`/`PGID`
settings decide who owns the files it writes, which is what lets the
production stack work from bind mounts without you pre-chowning the
database directory. Its data directory is `/config/databases`, not
`/var/lib/mysql` -- so if you ever swap it for the official `mariadb`
image, the existing files are not where that image looks and it will not
migrate them for you.

That image boots through s6 init and expects to start the server itself,
so do not add a `command:` override to either database service. It
already defaults to `utf8mb4` / `utf8mb4_general_ci`; anything else goes
in `/config/custom.cnf`, which it writes on first start.

Note that `schema.sql` declares `DEFAULT CHARSET=latin1` on every table
it creates, so the server-level character set matters less than it looks
-- the application connects with `charset=utf8mb4` regardless.

This stack binds to `127.0.0.1:8080`, so put a reverse proxy in front of
it and terminate TLS there.

## What persists, and what does not

Anything not on this list lives in the container's writable layer and is
destroyed when the container is recreated — which happens on every image
upgrade.

**Development stack:**

| Data | Where |
|---|---|
| Database | `db-data` named volume (mounted at `/config`, LinuxServer layout) |
| Keys, uploads, SQLite database | `web-state` named volume at `/var/lib/writefreely` |
| Configuration | `./config.ini` bind mount |

**Production stack:**

| Data | Where |
|---|---|
| Database | `./dbdata` (LinuxServer layout: files sit under `./dbdata/databases`) |
| Everything else | `./data`, mounted at `/var/lib/writefreely` |

Uploaded images default to a directory inside the static asset tree,
which is part of the image, so without either a volume there or an
`[uploads] dir` pointing into the state directory, every upload is lost
on the next `docker compose pull && up -d`. Setting `dir` as shown above
puts them on the state volume with everything else.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `WRITEFREELY_DOCKER` | set in both images | Tells `--config` it is running in a container: bind `0.0.0.0`, use container asset paths |
| `WRITEFREELY_DOCKER_PARENT_DIR` | per image | Asset root the configurator writes into the config |
| `WRITEFREELY_AUTO_MIGRATE` | `true` | Apply pending migrations on start |
| `WRITEFREELY_INIT_DB` | `auto` | Create the schema on start. `auto` initializes when the keys also had to be generated, i.e. on a first run. `true` forces it, `false` disables it |
| `WRITEFREELY_CONFIG` | `config.ini` | Config file the entrypoint looks for |
| `WRITEFREELY_KEYS_DIR` | `keys` | Directory the entrypoint checks for keys |

`MYSQL_PASSWORD` and `MYSQL_ROOT_PASSWORD` come from `.env` and are read
by the database container. Neither compose stack starts without them —
deliberately, so that no deployment inherits a password published in this
repository.

## Upgrading

```sh
docker compose pull
docker compose up -d
```

The entrypoint applies pending migrations on start, so an upgrade that
includes a schema change needs no separate step. **Back the database up
first.** Migrations are not reversible, and downgrading the image after
one has run will not work.

To upgrade without automatic migrations, set `WRITEFREELY_AUTO_MIGRATE`
to `false` and run `writefreely --migrate` yourself when ready.

## Troubleshooting

**The web container exits immediately on a fresh install.** It has no
`config.ini`. The log says so and prints the command to create one.

**"no such file or directory" mentioning config.ini, but the file
exists.** Docker created a directory rather than using your file. Remove
it, `touch config.ini`, recreate the container.

**The app cannot reach the database.** The database host is the *service*
name — `writefreely-db` in the development stack, `db` in production —
not `localhost`. A container's `localhost` is itself.

**Permission denied writing to the state directory.** The image runs as
uid 1000 and your `./data` is owned by someone else.
`sudo chown -R 1000:1000 data`.

**The container reports unhealthy but the site works.** Please report it.
The healthcheck accepts any HTTP response as proof of life, so it should
not false-negative on a password-protected or unconfigured instance.

**Uploaded images disappeared after an upgrade.** The uploads volume was
missing. See the note above.

**`migrate: no such table: appcontent`, or `Table 'writefreely.appcontent'
doesn't exist`.** Migrations ran against a database with no schema, which
leaves it stamped at a version it never reached. Recreate the database
from empty and start again; the entrypoint creates the schema on a first
run, so this should only happen if `WRITEFREELY_INIT_DB=false` was set on
a fresh instance.

**`Invalid database type 'sqlite'. Only 'mysql' and 'sqlite3' are
supported right now.`** The driver name in `config.ini` is `sqlite3`, not
`sqlite`. The interactive configurator writes the right value; this bites
when the file is written by hand.

## Running on SQLite instead of MariaDB

Neither compose stack uses SQLite, but the images support it and it is a
reasonable choice for a small single-user instance. Drop the database
service and point the config at a file on a volume:

```ini
[database]
type     = sqlite3
filename = /var/lib/writefreely/writefreely.db
```

Make sure the containing directory is on a volume, or the database is
destroyed with the container. The binary is built with the `sqlite` build
tag, so no extra image is needed.
