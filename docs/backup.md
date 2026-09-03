# Backups

Backups are a compose profile, and they are off until you ask for them. When you do, a side-container takes a consistent snapshot of the database, hands it to [restic](https://restic.net) along with the state directory, encrypts everything, and uploads it to whatever storage you point it at.

## What it backs up, and why the keys matter most

Everything under the state directory: `config.ini`, `keys/`, `uploads/`, any legacy image tree you have put there, and the database. Exclusions are opt-in, and the only ones are the live database file and its journal siblings, which are replaced in the snapshot by a copy taken consistently.

`keys/` is the reason this matters more than it looks. It holds the instance's federation keypair. Posts can be reconstructed from an API export, and images can be re-uploaded, but the keypair cannot be regenerated: every remote server that already knows this actor has its public key, and a new one means every signature it sends is rejected from then on. Losing that directory means losing the instance's identity on the network, permanently, whatever else survives.

## Getting started

Fill in the backup block in `.env`. At minimum that is `RESTIC_REPOSITORY`, `RESTIC_PASSWORD` and the credentials for your storage backend.

Check the configuration before trusting it to a schedule:

```sh
docker compose --profile backup run --rm backup verify
```

That validates the environment, reads `config.ini`, connects to the database, checks there is room in `/tmp` for the staged copy, and reaches the repository, initialising it when it does not exist yet. It reports every problem it finds rather than stopping at the first.

Then start the scheduler:

```sh
docker compose --profile backup up -d
```

It backs up on `BACKUP_SCHEDULE` and applies the retention policy after each run. To take one immediately:

```sh
docker compose --profile backup run --rm backup backup
```

## Database credentials come from config.ini

The sidecar reads `[database]` out of `config.ini` in the state directory, which it already mounts. There is nothing to configure and no database password in `.env`, because a second copy of a password is a second thing to leak and a second thing to drift out of step with the first.

Both engines are supported, and the image variant has to match the one you run. `type = sqlite3` needs the `-sqlite` image, `type = mysql` needs the `-mysql` one. The SQLite variant takes its snapshot through SQLite's online backup API, which is consistent against a running instance without stopping it; the MySQL variant dumps with `--single-transaction`, which is consistent for the same reason.

## Where snapshots go

The sidecar has no opinion. It consumes `RESTIC_REPOSITORY` and `RESTIC_PASSWORD` and backs up to whatever they name: S3, Backblaze B2, an S3-compatible provider, SFTP, or a local path.

Give WriteFreely its own repository with its own password rather than sharing one with another service. Retention policies then stay independent, a mistake in one cannot prune the other, and two stacks do not contend for the repository lock.

## RESTIC_PASSWORD is the secret that matters

Snapshots contain `config.ini`, and `config.ini` can contain mail credentials. They are encrypted, so whoever holds `RESTIC_PASSWORD` plus read access to the storage holds those credentials too. Treat it accordingly.

Store it somewhere that survives the server. A password that only exists in the `.env` on the machine you are backing up is not a backup password, it is a coincidence: the disaster that makes you need the backup is the one that takes the password with it. A backup you cannot decrypt is not a backup.

## Restoring

Stop WriteFreely first. The sidecar refuses to restore while the application answers on the network, because writing a database out from under a running process corrupts it. It cannot stop the container for you: doing that would mean mounting the Docker socket, which is root on the host handed to a backup script, and that is a bad trade for saving one command.

```sh
docker compose stop app
docker compose --profile backup run --rm backup restore latest
```

The snapshot is extracted into a separate staging mount first, so nothing lands on top of live data before you have seen what is in it. Then it asks about each component separately: the database, `keys/`, `uploads/`, `legacy-images/` and `config.ini`. Answer only for what you actually want back.

Whatever a component replaces is kept as `<name>.bak-<timestamp>` rather than deleted, and if a component fails to install, the original is moved back. A wrong answer at a prompt is recoverable. Remove the `.bak-*` copies by hand once you are satisfied, since nothing else will.

`config.ini` defaults to no, and is skipped entirely by `--yes`. An older config points its `filename` and `[uploads] dir` at a state directory that this image no longer uses, and dropping one onto a current install produces a container that crash-loops on a config it cannot find. Restore it only when you specifically mean to, with `--components=config`.

For scripted or tested restores, `--yes` answers the prompts and `--components=database,keys` chooses exactly what to install.

To see what is available first:

```sh
docker compose --profile backup run --rm backup snapshots
```

## Monitoring

The container reports unhealthy unless three things hold: the scheduler is alive, the last run succeeded, and that run was recent. A container that is up but has silently stopped backing anything up is the failure worth catching, and Docker's own "up" status does not catch it.

Size `BACKUP_HEALTH_MAX_AGE` to your schedule plus some grace. It is in minutes, and the default of 11520 is eight days, which suits a weekly schedule. A daily schedule wants something closer to 2880.

Set `BACKUP_HEALTHCHECK_URL` to be told from outside. The URL is pinged after a successful run, and with `/fail` appended after a failed one, which is what [healthchecks.io](https://healthchecks.io) and Uptime Kuma expect. Monitoring is best-effort throughout: an unreachable endpoint is never allowed to fail the backup it is monitoring.

## Verify that it is real

Run a restore once, into a scratch copy of the state directory, before you need one. An untested backup is a hypothesis. `verify` proves the repository is reachable and `snapshots` proves something was written, but neither proves the contents are usable, and the day you find out otherwise is the worst possible day to find out.

`docker compose --profile backup run --rm backup stats` reports what the repository is holding, and `unlock` clears a stale lock left by an interrupted run.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `RESTIC_REPOSITORY` | required | where snapshots go |
| `RESTIC_PASSWORD` | required | the encryption password |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | | S3 and S3-compatible storage |
| `B2_ACCOUNT_ID`, `B2_ACCOUNT_KEY` | | Backblaze B2 |
| `BACKUP_SCHEDULE` | `0 3 * * *` | cron expression |
| `BACKUP_KEEP_DAILY` | `7` | retention |
| `BACKUP_KEEP_WEEKLY` | `4` | retention |
| `BACKUP_KEEP_MONTHLY` | `6` | retention |
| `BACKUP_KEEP_YEARLY` | `2` | retention |
| `BACKUP_HEALTHCHECK_URL` | unset | pinged on success, `/fail` on failure |
| `BACKUP_ALIVE_MAX_AGE` | `2` | minutes without a scheduler heartbeat before unhealthy |
| `BACKUP_HEALTH_MAX_AGE` | `11520` | minutes since the last successful run before unhealthy |
| `BACKUP_SITE` | `writefreely` | a restic tag, so one repository can hold several sites |
| `BACKUP_HOST` | the container hostname | the restic host, used for retention grouping |
| `RESTIC_CACHE_DIR` | restic's default | point it at a volume, or every prune re-downloads the index |
| `APP_URL` | `http://app:8080/` | where the restore checks whether the application is running |
| `PUID`, `PGID` | `1000` | ownership of everything a restore writes |

There are no database variables. Those come from `config.ini`.
