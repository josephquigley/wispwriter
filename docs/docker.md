# Running WriteFreely in Docker

There are two images in this repository, for two different jobs.

| | `Dockerfile` | `Dockerfile.prod` |
|---|---|---|
| Used by | `docker-compose.yml` | `docker-compose.prod.yml` |
| Published to a registry | yes, by CI | no, built locally |
| Asset root | `/go` | `/usr/share/writefreely` |
| Working directory | `/go` | `/data` |
| Runs as | `daemon` | uid/gid 1000 |
| Config lives at | `/go/config.ini` | `/data/config.ini` |
| Keys live at | `/go/keys` (volume) | `/data/keys` |

Use the first for evaluation and development, and the second when you
want a single `./data` directory holding everything the instance owns.

Both images share an entrypoint that generates encryption keys when they
are absent and applies pending database migrations, then runs the server.

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

Then initialize the schema and create your admin account:

```sh
docker compose run --rm -e WRITEFREELY_INIT_DB=true writefreely-web \
    cmd/writefreely/writefreely --create-admin youruser:yourpassword
docker compose up -d
```

WriteFreely is now on <http://localhost:8080>.

## First run, production stack

```sh
cp .env.example .env
$EDITOR .env
mkdir -p data db
sudo chown -R 1000:1000 data     # the image runs as uid 1000
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml run --rm app writefreely --config
docker compose -f docker-compose.prod.yml run --rm \
    -e WRITEFREELY_INIT_DB=true app writefreely --create-admin youruser:yourpassword
docker compose -f docker-compose.prod.yml up -d
```

The database host is `db`, and everything the instance owns lives in
`./data`.

This stack binds to `127.0.0.1:8080`, so put a reverse proxy in front of
it and terminate TLS there.

## What persists, and what does not

Anything not on this list lives in the container's writable layer and is
destroyed when the container is recreated — which happens on every image
upgrade.

**Development stack:**

| Data | Where |
|---|---|
| Database | `db-data` named volume |
| Encryption keys | `web-keys` named volume |
| Configuration | `./config.ini` bind mount |
| Uploaded images | `web-uploads` named volume |

**Production stack:**

| Data | Where |
|---|---|
| Database | `./db` |
| Everything else | `./data` |

Uploaded images deserve a specific note. They are written under the
static asset tree, which is part of the image, so without a volume every
upload is lost on the next `docker compose pull && up -d`. Both stacks
mount one. If you are upgrading an existing deployment that predates
this, copy the files out of the running container before recreating it:

```sh
docker cp writefreely-web:/go/static/uploads ./uploads-backup
```

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `WRITEFREELY_DOCKER` | set in both images | Tells `--config` it is running in a container: bind `0.0.0.0`, use container asset paths |
| `WRITEFREELY_DOCKER_PARENT_DIR` | per image | Asset root the configurator writes into the config |
| `WRITEFREELY_AUTO_MIGRATE` | `true` | Apply pending migrations on start |
| `WRITEFREELY_INIT_DB` | `false` | Create the schema on start. First run only |
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

**Permission denied writing to /data.** The production image runs as uid
1000 and your `./data` is owned by someone else. `sudo chown -R 1000:1000 data`.

**The container reports unhealthy but the site works.** Please report it.
The healthcheck accepts any HTTP response as proof of life, so it should
not false-negative on a password-protected or unconfigured instance.

**Uploaded images disappeared after an upgrade.** The uploads volume was
missing. See the note above.

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
filename = /data/writefreely.db
```

Make sure the containing directory is on a volume, or the database is
destroyed with the container. The binary is built with the `sqlite` build
tag, so no extra image is needed.
