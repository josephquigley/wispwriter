# Switching from upstream WriteFreely

An existing upstream WriteFreely instance can move to this edition in
place. There is no export, no import and no data conversion. The same
database, the same `config.ini` and the same keys carry over.

`scripts/switch-from-writefreely.sh` performs the switch. Read this page
first anyway, because the one step that cannot be undone is the database
migration, and the one step that must not be skipped is the backup.

## What carries over, and what changes

| | Upstream | This edition |
|---|---|---|
| Binary name | `writefreely` | `writefreely` |
| Config file | `config.ini` | `config.ini`, plus an optional `[uploads]` section |
| Database schema | through migration V17 | V17 plus V18, which adds `post_images` |
| Software name in nodeinfo, the `Server` header and the outbound user agent | `WriteFreely` | `WriteFreely`, deliberately unchanged |
| Name shown to people | WriteFreely | WriteFreely (Wisp Edition) |
| Update checks | on | off, see the README |

This edition is based on upstream 0.17.2 and carries upstream's migrations
V1 through V17 unmodified, so an instance running an older release
migrates straight through in one step, exactly as an upstream upgrade
would.

Migration V18 only creates a table. Nothing existing is altered or
dropped. The reordering of pinned posts uses `posts.pinned_position`,
which upstream already has, and multiple verification links are stored in
the `verification_link` attribute upstream already has, one per line.

Because the machine-readable software name is untouched, federation sees
no change: remote instances that already follow your blogs keep working,
and no re-verification is needed.

## Bare metal

The switch replaces the program and its assets, then migrates. Assets are
not optional: this edition's templates reference JavaScript and styles
that upstream's do not ship.

1. Stop the service and back up the database and the `keys` directory.
   Losing the keys invalidates every session and every encrypted value,
   and no backup of them can be reconstructed.

2. Install this edition over the old install, from the installation root
   (`/var/www/writefreely` on a typical setup): the `writefreely` binary,
   and the `templates`, `static` and `pages` directories. Leave
   `config.ini`, `keys` and any SQLite database where they are.

3. Run the script from that same directory:

   ```sh
   ./scripts/switch-from-writefreely.sh
   ```

   It checks that the binary in place really is this edition, confirms the
   backup with you, applies the migration, and offers to enable image
   uploads. It is equivalent to running `writefreely --migrate` yourself
   and editing `config.ini` by hand.

4. Start the service again.

## Docker

The application is a drop-in, but the image layout is not. Upstream's
image works out of `/go`, with `config.ini` bind mounted as a single file
and the keys in a named volume. This image works out of
`/var/lib/writefreely`, where one directory holds the config, the keys,
uploads, and the SQLite database if there is one.

| | `writeas/writefreely` | `ghcr.io/josephquigley/wispwriter` |
|---|---|---|
| Working directory | `/go` | `/var/lib/writefreely` |
| Config | `/go/config.ini`, bind mounted file | `<state>/config.ini` |
| Keys | named volume at `/go/keys` | `<state>/keys` |
| Assets | `/go/{templates,static,pages}`, writable | `/usr/share/writefreely`, read only |
| Runs as | `daemon` | `PUID`:`PGID`, 1000 by default |
| Entrypoint | the binary | a script that generates keys when absent, creates the schema on a first run, and migrates |

So the work is gathering the state into one directory. The script does
that, and it is carried inside the image, so there is nothing to clone:

```sh
docker compose down
docker run --rm -it \
    --user "$(id -u):$(id -g)" \
    -v "$PWD:/workspace" -w /workspace \
    -v writefreely_web-keys:/keys-src:ro \
    --entrypoint switch-from-writefreely \
    ghcr.io/josephquigley/wispwriter:latest --docker --keys-dir /keys-src
```

Replace `writefreely_web-keys` with your own volume, which
`docker volume ls` will show. Compose names volumes after the project, so
the prefix is usually the directory the stack lives in. Running the script
from a clone instead works the same way, and finds a single keys volume on
its own through the docker CLI:

```sh
./scripts/switch-from-writefreely.sh --docker
```

Either way it copies `config.ini` and the keys into `./data`, moves a
SQLite database in and repoints `filename` at it, offers to enable image
uploads, and prints the compose changes to make. It does not edit your
compose file, does not touch the database service, and deletes nothing.

Then point the app service at the new image and the new mount:

```yaml
  writefreely-web:
    image: ghcr.io/josephquigley/wispwriter:latest
    user: "${PUID:-1000}:${PGID:-1000}"
    volumes:
      - ./data:/var/lib/writefreely
```

Remove the old `config.ini` file mount and the keys volume from that
service. Leave the database service alone: its data is untouched by any of
this, and the app reaches it by service name as before. Add `PUID` and
`PGID` to `.env` if your host user is not uid 1000, as described in
[docker.md](docker.md).

`docker compose up -d` then starts the new image, and the entrypoint
applies migration V18.

The keys must be in `./data/keys` **before** that first start. The
entrypoint treats absent keys as the signal that this is a new instance:
it would generate a fresh set, invalidating every session, and then run
`--init-db` against a database that already has a schema.

The compose files in this repository are a working example of the target
layout, but they are not a drop-in replacement for yours. They name the
database service `db`, use LinuxServer's MariaDB image, and expect
`./dbdata`, none of which matches an upstream stack.

## Enabling image uploads

Uploads are off until the config says otherwise, whether or not you use
the script:

```ini
[uploads]
enabled     = true
max_size_mb = 10
dir         = /var/lib/writefreely/uploads
```

`max_size_mb` bounds one file, not a user's total storage. `dir` is where
the files are written. Under Docker it has to point into the state
directory, because everything else in the container is discarded when the
container is recreated. On bare metal it can be left out, in which case
uploads go under the static asset tree, which is fine until an upgrade
replaces that tree. Which image types are accepted is fixed by the
decoders compiled in, and is not configurable.

## Going back to upstream

Installing upstream over this edition mostly works. The `post_images`
table is simply unused, and every other table is upstream's own.

The exception is the migration counter. This edition leaves
`appmigrations` recording version 18, and upstream's next migration will
also be numbered 18, so upstream would consider it already applied and
skip it. Going back means dropping `post_images` and setting that version
back to 17 by hand, before running upstream's `--migrate`.

Uploaded images are files on disk, and upstream has nowhere to serve them
from. Posts referencing them will show broken images.
