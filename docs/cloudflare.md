# Running behind Cloudflare

WriteFreely can sit behind Cloudflare's proxy, but the default cache settings are wrong for it
in both directions: they cache too little to help, and — once you start caching HTML — they can
cache something they should not. This describes a configuration that has been measured against
a live instance, and the reasoning behind each part, so you can adapt rather than copy blindly.

Everything here is Cache Rules, available on every plan. Nothing needs a Worker.

## What the application does

Three measured facts drive the whole configuration.

**Post pages set no cookie.** A published post is identical for every logged-out reader and
carries no `Set-Cookie`, so it is safe to cache and serve to everyone.

**The index sets a session cookie on every request.** `GET /` responds with
`Set-Cookie: wfu=…` every single time, even when the request already carried one. The cookie is
a gorilla session; logged out it is empty, logged in it *is* the owner's session.

**Cloudflare will cache that response anyway.** It is widely believed that Cloudflare refuses to
cache a response carrying `Set-Cookie`. Under a Cache Rule that marks the response eligible,
that is not reliable — an instance was observed serving `/` as a `HIT` while the origin set the
cookie on every request. **Do not treat Cloudflare's `Set-Cookie` handling as a safety net.**
The bypass below is the actual protection.

**Uploads and static assets** are served `public, max-age=604800, immutable`.

## The risk to design around

If a page rendered for the signed-in owner is eligible for caching, it can populate the shared
cache and be served to every reader — the owner's view of the site, with its edit and pin
controls, and potentially a session cookie attached.

Two things prevent that: never cache a request that carries a session cookie, and never cache
the admin paths. Both are below.

## Rules

Four rules, in this order. Replace `example.com` with your host.

```
1. Bypass cache for signed-in sessions and admin paths
   (http.host eq "example.com" or http.host eq "www.example.com")
   and (len(http.request.cookies["wfu"]) > 0
        or starts_with(http.request.uri.path, "/me/")
        or starts_with(http.request.uri.path, "/admin")
        or starts_with(http.request.uri.path, "/api/")
        or starts_with(http.request.uri.path, "/auth/")
        or http.request.uri.path eq "/login"
        or http.request.uri.path eq "/logout")
   -> Cache eligibility: Bypass cache

2. Uploads and static assets: honour the origin's TTL
   ... and (GET or HEAD) and NOT-SESSION-OR-ADMIN
   and (starts_with(http.request.uri.path, "/uploads/")
        or starts_with(http.request.uri.path, "/css/")
        or starts_with(http.request.uri.path, "/js/")
        or starts_with(http.request.uri.path, "/img/"))
   -> Eligible for cache; Edge TTL: respect origin; Browser TTL: respect origin

3. Index and feed: short TTL
   ... and (GET or HEAD) and NOT-SESSION-OR-ADMIN
   and (http.request.uri.path eq "/"
        or starts_with(http.request.uri.path, "/feed")
        or starts_with(http.request.uri.path, "/page/"))
   -> Eligible for cache; Edge TTL: 300s; Browser TTL: respect origin

4. Cache anonymous HTML
   ... and (GET or HEAD) and NOT-SESSION-OR-ADMIN
   -> Eligible for cache; Edge TTL: 7200s; Browser TTL: respect origin
```

### ⚠️ Repeat the exclusions in every caching rule

`NOT-SESSION-OR-ADMIN` above is not shorthand for "rule 1 already handled it". It must be
written out in rules 2, 3 and 4:

```
not (len(http.request.cookies["wfu"]) > 0
     or starts_with(http.request.uri.path, "/me/")
     or starts_with(http.request.uri.path, "/admin")
     or starts_with(http.request.uri.path, "/api/")
     or starts_with(http.request.uri.path, "/auth/")
     or http.request.uri.path eq "/login"
     or http.request.uri.path eq "/logout")
```

**Cloudflare evaluates every matching cache rule in order, and a later match overrides an
earlier one.** A bypass rule at the top does not protect anything below it: a broad "cache
anonymous HTML" rule underneath silently turns caching back on for the very requests you
excluded. This is easy to get wrong and gives no error — the rules look right and a request
carrying `wfu` still comes back `HIT`.

### Why the index gets a short TTL

WriteFreely has no outgoing webhooks, so nothing can tell Cloudflare to purge when you publish.
A short edge TTL on the index and feed is the substitute: a new post appears within five
minutes. Post pages themselves are unaffected — a new post's URL is a cache miss on its first
request — so only the listing pages need the shorter window.

If you drive purges another way (a cron that polls the feed, or a hook in your own publishing
script), raise rule 3's TTL to match rule 4.

### Why not just strip the cookie on `/`

You can make the index fully cacheable by having your reverse proxy drop `Set-Cookie` for
requests that arrive **without** a session cookie, which is safe when the instance is
single-user with registration closed — an anonymous session then holds nothing. Stock
WriteFreely uses it to track posts made by anonymous authors, so this does not generalise.

If you do it, strip **only** when the request had no session cookie, so a logged-in response can
never lose its `Set-Cookie` and be marked public. Never strip unconditionally, and never
override the origin's cache-control on `/` to force caching.

## Verify it

After any rule change, purge and check. Anonymous should hit; everything session- or
admin-shaped should be `DYNAMIC` (not eligible) or `BYPASS`:

```sh
curl -sI https://example.com/            | grep -i cf-cache-status   # HIT (after a warm-up)
curl -sI https://example.com/some-post   | grep -i cf-cache-status   # HIT
curl -sI -H 'Cookie: wfu=x' https://example.com/ | grep -i cf-cache-status   # DYNAMIC
curl -sI https://example.com/me/settings | grep -i cf-cache-status   # DYNAMIC
curl -sI https://example.com/api/collections/<alias>/outbox | grep -i cf-cache-status  # DYNAMIC
```

The one that matters most is the third. If it says `HIT`, the exclusions are missing from a
later rule and the owner's session can reach the shared cache.

## Purge after anything that changes what the origin serves

Upgrades, config changes, a theme edit, or switching the site over from other software. A
migration onto WriteFreely was invisible to the public for over an hour because this was
skipped: the origin served WriteFreely while the edge kept serving the old site's cached HTML.

```sh
curl -X POST \
  -H "Authorization: Bearer $CF_TOKEN" -H "Content-Type: application/json" \
  --data '{"purge_everything":true}' \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache"
```

## Zone settings worth checking

- **SSL mode: Full (strict)**, not Full. Both encrypt Cloudflare→origin, but Full accepts *any*
  certificate the origin presents — expired, self-signed, wrong host. Anything able to attract
  or intercept that traffic can present its own certificate and read or modify requests while
  visitors still see a valid padlock. Strict requires a valid certificate. If your origin
  terminates TLS with a real certificate, there is no reason to stay on Full.
- **Browser Cache TTL: Respect Existing Headers.** Otherwise Cloudflare overrides the
  `max-age` sent to browsers, and a reader's own browser can pin a stale index for hours.
- **Minimum TLS version: 1.2.**

## Applying by API

Cache rules live in one ruleset per zone. Fetch the ruleset id once:

```sh
curl -s -H "Authorization: Bearer $CF_TOKEN" \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/rulesets" \
  | jq -r '.result[] | select(.phase=="http_request_cache_settings") | .id'
```

Then `PUT` the whole rule list — the request body is `{"rules": [ ... ]}`, and it replaces
every rule in the ruleset, so send them all:

```sh
curl -X PUT \
  -H "Authorization: Bearer $CF_TOKEN" -H "Content-Type: application/json" \
  --data @rules.json \
  "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/rulesets/$RULESET_ID"
```

Save the response somewhere version-controlled before you change anything, so you can put the
previous rules back.
