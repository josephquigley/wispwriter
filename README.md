# WriteFreely (Colophon Edition)

**A modified version of [WriteFreely](https://github.com/writefreely/writefreely).**
Not affiliated with or endorsed by Musing Studio LLC or the WriteFreely
project.

This edition versions independently of upstream, so its version numbers do
not correspond to WriteFreely releases.

Upstream keeps the core deliberately minimal and moves features out into
companion services — image hosting, for instance, lives on a separate site
reached through a browser extension. This edition brings those capabilities
in-tree, so a single self-hosted instance is self-sufficient.

## What differs from upstream

### Images are hosted by the instance

Drag an image into the editor and it uploads to your own instance, inserting
a Markdown link at the cursor. A thumbnail strip below the editor deletes an
image from the server and strips its link from the post in one action. No
companion service and no browser extension.

Files are content-addressed on local disk under `static/uploads/`, so
re-uploading identical bytes costs nothing. Images are reference-counted
against post bodies: deleting a post or an image removes the file only when
no other post still references it, and uploads abandoned in unsaved drafts
are swept hourly. Uploads are re-encoded to strip EXIF — phone photos
routinely carry GPS coordinates — and the type is decided by sniffing the
content, never the filename or the submitted header. PNG, JPEG and GIF only;
SVG is rejected because it can carry script, and WebP because decoding it
would add a dependency. Configured under `[uploads]`, disabled by default.

### A post management view

`/me/c/{alias}/posts` lists a blog's posts as a compact table — title, date,
pinned and scheduled badges, and the edit, pin, delete and move actions —
so reaching an older post's controls no longer means scrolling the public
index past every post body. Instance admins get the same view across all
blogs at `/admin/posts`. Post bodies are never loaded for these pages.

### Pinned posts can be reordered

`/me/c/{alias}/pinned` lists the posts that make up a blog's navigation, with
move-up, move-down and unpin controls. Every control is a form submission, so
the page works without JavaScript, on mobile, and with a screen reader.

This also repairs a latent problem: pinning only ever appended a position and
unpinning left a gap behind, so positions drifted into sparse and occasionally
duplicated sequences. They are now normalized to a dense order on every read
and write.

### Multiple `rel="me"` verification links

A blog can declare any number of verification links instead of exactly one,
added and removed as rows in the blog's settings. The first remains the
canonical identity that `fediverse:creator` is derived from. Stored in the
existing collection attribute, so no database migration is involved and a
single pre-existing link upgrades silently.

Note for API consumers: the `Collection` JSON now reads back
`verification_links` as an array. Writes still accept `verification_link`.

### Control over the subscribe form

The email subscribe form now renders at the end of individual post pages,
where upstream shows it only on the blog index — most readers arrive at a
post directly and were never offered the subscription. Two independent
per-blog toggles control each placement. Both default to on, so existing
blogs are unchanged.

### Fixes carried in this edition

- **Owner links broke on single-user instances.** Pin, unpin, delete, the
  subscribe redirect, and the blog links on the account and admin pages all
  built `/<alias>/<slug>` URLs unconditionally. On a single-user instance the
  blog is served at the site root, so every one of those 404'd. They were
  gated on being the blog's owner, which is why anonymous readers never hit
  them.
- **CSRF rejected every protected request over plain HTTP.** `gorilla/csrf`
  assumes TLS unless told otherwise: it set a Secure cookie the browser never
  returned, and required an `https://` Referer that a browser on an http site
  never sends. Saving settings, importing posts and removing an OAuth
  connection all failed with a blanket 403.
- **`UpdateCollection` panicked** on an attribute-only update, dereferencing a
  `sql.Result` that is only assigned when there are column updates. Reachable
  today through a monetization-only update.
- **Builds could report no version at all.** The version was taken from
  `git describe` and injected unconditionally, so any build context without a
  usable git repository produced a binary whose version string was empty and
  whose footer read a bare "v". Building from a git worktree does this every
  time.

### Container changes

The shipped Docker setup had a data-loss bug and could not complete a first
run. See [docs/docker.md](docs/docker.md) for the full guide, which upstream
does not carry in-tree.

- The development stack mounted its database volume at `/var/lib/mysql/data`
  while MariaDB's data directory is `/var/lib/mysql`, so the database lived in
  the container layer and was **destroyed on every recreate**.
- The production stack bind-mounted `./db`, colliding with this repository's
  own `db/` Go package and writing MariaDB's state into the source tree.
- Neither stack could complete a first run: the entrypoint applied migrations
  without creating the schema, and migrating an empty database leaves it
  recorded at a version it never reached.
- Committed database passwords, an app connecting as root, unpinned base
  images, an end-of-life runtime image, OCI labels stranded in the build
  stage, and a healthcheck that reported a working instance as unhealthy
  whenever its landing page was not public.

## Upstream

Everything below this point is WriteFreely's own README, unchanged.
Documentation, the writer's guide and the project itself live at
**<https://writefreely.org>**, and the upstream source at
**<https://github.com/writefreely/writefreely>**.

---


&nbsp;
<p align="center">
	<a href="https://writefreely.org"><img src="https://writefreely.org/img/writefreely.svg" width="350px" alt="WriteFreely" /></a>
</p>
<hr />
<p align="center">
	<a href="https://github.com/writefreely/writefreely/releases/">
		<img src="https://img.shields.io/github/release/writefreely/writefreely.svg" alt="Latest release" />
	</a>
	<a href="https://github.com/writefreely/writefreely/releases/latest">
		<img src="https://img.shields.io/github/downloads/writefreely/writefreely/total.svg" />
	</a>
	<a href="https://ghcr.io/writefreely/writefreely">
		<img src="https://img.shields.io/badge/docker-%230db7ed.svg?logo=docker&logoColor=white" />
	</a>
	<a href="https://github.com/writefreely/writefreely/actions/workflows/docker-publish.yml">
		<img src="https://github.com/writefreely/writefreely/actions/workflows/docker-publish.yml/badge.svg" alt="Build container image, publish as GitHub-package" />
	</a>
</p>
&nbsp;

WriteFreely is a clean, minimalist publishing platform made for writers. Start a blog, share knowledge within your organization, or build a community around the shared act of writing.

![Screenshot of the Reader view of a WriteFreely instance, pen.writefree.ly.](https://files.writefreely.org/img/screens/pen-reader.png)

[Try the writing experience](https://write.as/new)

[Find an instance](https://writefreely.org/instances)

## Features

### Made for writing

Built on a plain, auto-saving editor, WriteFreely gives you a distraction-free writing environment. Once published, your words are front and center, and easy to read.

### A connected community

Start writing together, publicly or privately. Connect with other communities, whether running WriteFreely, [Plume](https://joinplu.me/), or other ActivityPub-powered software. And bring members on board from your existing platforms, thanks to our OAuth 2.0 support.

### Intuitive organization

Categorize articles [with hashtags](https://writefreely.org/docs/latest/writer/hashtags), and create static pages from normal posts by [_pinning_ them](https://writefreely.org/docs/latest/writer/static) to your blog. Create draft posts and publish to multiple blogs from one account.

### International

Blog elements are localized in 20+ languages, and WriteFreely includes first-class support for non-Latin and right-to-left (RTL) script languages.

### Private by default

WriteFreely collects minimal data, and never publicizes more than a writer consents to. Writers can seamlessly create multiple blogs from a single account for different pen names or purposes without publicly revealing their association.

<h2><a href="https://write.as/writefreely"><img src="https://writefreely.org/img/writeas-readme.png" height="32px" alt="Write.as" /></a></h2>

The quickest way to deploy WriteFreely is with [Write.as](https://write.as/writefreely), a hosted service from the team behind WriteFreely. You'll get fully-managed installation, backup, upgrades, and maintenance — and directly fund our free software work ❤️

[**Learn more on Write.as**](https://write.as/writefreely).

## Quick start

WriteFreely deploys as a static binary on any platform and architecture that Go supports. Just use our built-in SQLite support, or add a MySQL or MariaDB database, and you'll be up and running!

For common platforms, start with our [pre-built binaries](https://github.com/writefreely/writefreely/releases/) and head over to our [installation guide](https://writefreely.org/start) to get started.

### Packages

You can also find WriteFreely in these package repositories, thanks to our wonderful community!

* [Arch User Repository](https://aur.archlinux.org/packages/writefreely/)
* [Nanos Repository](https://repo.ops.city/v2/packages/eyberg/writefreely/show)

## Documentation

Read our full [documentation on WriteFreely.org](https://writefreely.org/docs) &mdash;️ and help us improve by contributing to the [writefreely/documentation](https://github.com/writefreely/documentation) repo.

## Development

Start hacking on WriteFreely with our [developer setup guide](https://writefreely.org/docs/latest/developer/setup). For Docker support, see [docs/docker.md](docs/docker.md) in this repository, or our [Docker guide](https://writefreely.org/docs/latest/admin/docker).

## Contributing

We gladly welcome contributions to WriteFreely, whether in the form of [code](https://github.com/writefreely/writefreely/blob/master/CONTRIBUTING.md#contributing-to-writefreely), [bug reports](https://github.com/writefreely/writefreely/issues/new?template=bug_report.md), [feature requests](https://discuss.write.as/c/feedback/feature-requests), [translations](https://poeditor.com/join/project/TIZ6HFRFdE), or [documentation](https://github.com/writefreely/documentation) improvements.

Before contributing anything, please read our [Contributing Guide](https://github.com/writefreely/writefreely/blob/master/CONTRIBUTING.md#contributing-to-writefreely). It describes the correct channels for submitting contributions and any potential requirements.

## License

Copyright © 2018-2026 [Musing Studio LLC](https://musing.studio) and contributing authors. Licensed under the [AGPL](https://github.com/writefreely/writefreely/blob/develop/LICENSE).
