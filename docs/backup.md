# Backups

Nothing in this repository backs your instance up. Backups are a side-container you add to your own compose file, built and published separately at [josephquigley/writefreely-backup](https://github.com/josephquigley/writefreely-backup). That repository is the reference; this page is the short version and the reason it is worth doing.

It takes a consistent snapshot of the database, hands it to [restic](https://restic.net) along with the state directory, encrypts everything, and uploads it wherever you point it. It works against stock WriteFreely as well as this fork, and it reads its database credentials from `config.ini` rather than needing them again in `.env`.

## Back up the state directory, and understand what is in it

Everything the instance owns lives in one directory, mounted at `/data`: `config.ini`, `keys/`, `uploads/`, the SQLite database if you use one. Backing up that directory and the database is the whole job.

`keys/` is the part people underestimate. It holds the instance's federation keypair. Posts can be re-imported from an export and images can be re-uploaded, but the keypair cannot be regenerated: every remote server that already knows this actor holds its public key, and a new one means every signature it sends is rejected from then on. Losing that directory costs the instance its identity on the network, whatever else survives.

The database needs care rather than a file copy. A running instance is writing to it, and `cp` can capture a page torn mid-transaction: the copy looks fine until you restore it. The sidecar takes SQLite snapshots through the online backup API and dumps MySQL with `--single-transaction`, then verifies the result before storing it.

## Adding it

There is one image per database, so pick the one matching your `[database] type`: `writefreely-backup-sqlite` for `sqlite3`, `writefreely-backup-mysql` for `mysql`. Both ship both drivers and differ only in which client is installed, so the wrong one fails at startup rather than at pull time (it says which image you want, but it is still a surprise you do not need). Then add a service to your compose file, under a profile so it stays off until you ask for it:

```yaml
  backup:
    image: ghcr.io/josephquigley/writefreely-backup-sqlite:latest
    restart: unless-stopped
    # Match the user the app container runs as, so a restore writes files
    # it can read.
    user: "${PUID:-1000}:${PGID:-1000}"
    environment:
      RESTIC_REPOSITORY: ${RESTIC_REPOSITORY:-}
      RESTIC_PASSWORD: ${RESTIC_PASSWORD:-}
      RESTIC_CACHE_DIR: /cache
      AWS_ACCESS_KEY_ID: ${AWS_ACCESS_KEY_ID:-}
      AWS_SECRET_ACCESS_KEY: ${AWS_SECRET_ACCESS_KEY:-}
      BACKUP_SCHEDULE: 0 3 * * *
      BACKUP_KEEP_DAILY: 7
      BACKUP_KEEP_WEEKLY: 4
      BACKUP_KEEP_MONTHLY: 6
      BACKUP_HEALTHCHECK_URL: ${BACKUP_HEALTHCHECK_URL:-}
      # Size to BACKUP_SCHEDULE plus grace. 2880 is two days, for a daily
      # schedule; the 11520 default suits a weekly one.
      BACKUP_HEALTH_MAX_AGE: 2880
      BACKUP_SITE: my-blog
      APP_URL: http://app:8080/
      PUID: ${PUID:-1000}
      PGID: ${PGID:-1000}
    volumes:
      - ./data:/data
      - ./data-restore:/restore
      - backup_cache:/cache
    profiles: [backup]

volumes:
  backup_cache:
```

On the MariaDB stack, use `writefreely-backup-mysql` and add `depends_on: db: condition: service_healthy`. Add the restic and storage variables to your `.env`, then:

```sh
mkdir -p data-restore
docker compose --profile backup run --rm backup verify
docker compose --profile backup up -d
```

`verify` checks everything before you trust a schedule to it: the environment, `config.ini`, the database connection, room in `/tmp` for the staged copy, and the repository, initialising it if it is new.

Pin by digest once it works, so an upgrade is a deliberate edit rather than a tag moving underneath a running container. Each package carries `latest` and `main` following its default branch, `sha-xxxxxxx` for an exact commit, and `1` / `1.4` / `1.4.2` once there is a release to pin to.

## RESTIC_PASSWORD is the secret that matters

Snapshots contain `config.ini`, and on most instances `config.ini` contains mail credentials. They are encrypted, so whoever holds `RESTIC_PASSWORD` plus read access to the storage holds those credentials too.

Store it somewhere that survives the server. A password that exists only in the `.env` on the machine being backed up is not a backup password, it is a coincidence: the disaster that makes you need the backup is the one that takes the password with it. A backup you cannot decrypt is not a backup.

## Restoring

Stop the app first. The sidecar refuses to restore while WriteFreely answers on the network, because writing a database out from under a running process corrupts it.

```sh
docker compose stop app
docker compose --profile backup run --rm backup restore latest
```

It asks about each component separately, keeps whatever it replaces as `<name>.bak-<timestamp>`, and rolls a component back if it fails to install. `config.ini` defaults to no: an older one can point `filename` and `[uploads] dir` at `/var/lib/writefreely`, which this image stopped using in 0.18.0, and restoring it onto a `/data` install produces the `no config.ini found` crash loop described in [docker.md](docker.md).

Rehearse a restore against a scratch copy before you need one. An untested backup is a hypothesis.

The full reference, including every environment variable and how to add another database engine, is in the [writefreely-backup README](https://github.com/josephquigley/writefreely-backup#readme).
