package writefreely

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/writefreely/writefreely/config"

	"github.com/stretchr/testify/assert"
)

func TestParseVerificationLinks(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		want   []string
	}{
		{"empty", "", []string{}},
		{"legacy single value", "https://example.com", []string{"https://example.com"}},
		{"two links", "https://a.example\nhttps://b.example", []string{"https://a.example", "https://b.example"}},
		{"blank lines dropped", "https://a.example\n\n\nhttps://b.example\n", []string{"https://a.example", "https://b.example"}},
		{"whitespace trimmed", "  https://a.example  \n\thttps://b.example\t", []string{"https://a.example", "https://b.example"}},
		{"duplicates removed, first kept", "https://a.example\nhttps://b.example\nhttps://a.example", []string{"https://a.example", "https://b.example"}},
		{"carriage returns tolerated", "https://a.example\r\nhttps://b.example", []string{"https://a.example", "https://b.example"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, parseVerificationLinks(c.stored))
		})
	}
}

func TestSerializeVerificationLinks(t *testing.T) {
	assert.Equal(t, "", serializeVerificationLinks(nil))
	assert.Equal(t, "", serializeVerificationLinks([]string{"", "  "}))
	assert.Equal(t, "https://a.example\nhttps://b.example",
		serializeVerificationLinks([]string{"https://a.example", "", "https://b.example"}))
}

func TestVerificationLinksRoundTrip(t *testing.T) {
	in := []string{"https://a.example", "https://b.example", "https://c.example"}
	assert.Equal(t, in, parseVerificationLinks(serializeVerificationLinks(in)),
		"order must survive a round trip")
}

func TestCollectionVerificationAccessor(t *testing.T) {
	c := &Collection{}
	assert.Equal(t, "", c.Verification(), "no links yields empty string")

	c.Verifications = []string{"https://first.example", "https://second.example"}
	assert.Equal(t, "https://first.example", c.Verification(),
		"the first link is the canonical identity")
}

// multiUser configures the test app for multi-user mode, where a blog is
// served under /{alias}/ rather than at the instance root.
func multiUser(cfg *config.Config) {
	cfg.App.SingleUser = false
}

// saveVerificationLinks updates a collection's verification links through
// the normal UpdateCollection path. UpdateCollection rejects a submission
// with no column updates in it, so a title is always sent alongside.
func saveVerificationLinks(app *App, coll *Collection, links string) error {
	title := coll.Title
	return app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:      uint64(coll.OwnerID),
		Alias:        &coll.Alias,
		Title:        &title,
		Verification: &links,
	}, coll.Alias)
}

func TestVerificationLinksPersist(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "verifier")

	links := "https://a.example\nhttps://b.example"
	err := saveVerificationLinks(app, coll, links)
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{"https://a.example", "https://b.example"}, loaded.Verifications)
	assert.Equal(t, "https://a.example", loaded.Verification())
}

func TestLegacySingleVerificationLinkStillLoads(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "legacyverifier")

	// Write the attribute directly, as a pre-upgrade instance would have.
	err := app.db.SetCollectionAttribute(coll.ID, "verification_link", "https://only.example")
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{"https://only.example"}, loaded.Verifications)
}

func TestVerificationLinksNormalizePerEntry(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "normalizer")

	// A handle on a silo instance resolves without a network lookup, a
	// scheme-less entry gains https://, and a full URL is left alone.
	err := saveVerificationLinks(app, coll, "@me@github.com\nexample.com/about\nhttps://plain.example/me")
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{
		"https://github.com/me",
		"https://example.com/about",
		"https://plain.example/me",
	}, loaded.Verifications)
}

func TestSingleVerificationHandleNormalizesAsBefore(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "singlehandle")

	err := saveVerificationLinks(app, coll, "@me@github.com")
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{"https://github.com/me"}, loaded.Verifications)
	assert.Equal(t, "https://github.com/me", loaded.Verification())
}

func TestClearingVerificationLinksRemovesThemAll(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "clearverify")

	assert.NoError(t, saveVerificationLinks(app, coll, "https://a.example\nhttps://b.example"))
	assert.NoError(t, saveVerificationLinks(app, coll, ""))

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, loaded.Verifications)
	assert.Equal(t, "", loaded.Verification())
}

func TestRendersAllVerificationLinks(t *testing.T) {
	app, router := newTemplateTestApp(t, multiUser)
	_, coll, _ := createTemplateTestUser(t, app, "multiverify")

	err := saveVerificationLinks(app, coll, "https://a.example\nhttps://b.example")
	assert.NoError(t, err)

	// renderedRequest's second return is captured log output, not the body.
	rec, _ := renderedRequest(t, router, "GET", "/"+coll.Alias+"/", nil)
	body := rec.Body.String()
	assert.Contains(t, body, `<link rel="me" href="https://a.example" />`)
	assert.Contains(t, body, `<link rel="me" href="https://b.example" />`)
	assert.Less(t, strings.Index(body, "https://a.example"), strings.Index(body, "https://b.example"),
		"links must render in stored order")
}

func TestFediverseCreatorUsesFirstLink(t *testing.T) {
	app, router := newTemplateTestApp(t, multiUser)
	_, coll, post := createTemplateTestUser(t, app, "fedicreator")

	// Both links resolve to a known remote user, so only the ordering can
	// decide which handle fediverse:creator ends up carrying.
	for _, ru := range []struct{ url, handle string }{
		{"https://first.example/@one", "@one@first.example"},
		{"https://second.example/@two", "@two@second.example"},
	} {
		_, err := app.db.Exec("INSERT INTO remoteusers (actor_id, inbox, shared_inbox, url, handle) VALUES (?, ?, ?, ?, ?)",
			ru.url, ru.url+"/inbox", ru.url+"/inbox", ru.url, ru.handle)
		assert.NoError(t, err)
	}

	err := saveVerificationLinks(app, coll, "https://first.example/@one\nhttps://second.example/@two")
	assert.NoError(t, err)

	slug := post.Slug.String
	if slug == "" {
		slug = post.ID
	}
	rec, _ := renderedRequest(t, router, "GET", "/"+coll.Alias+"/"+slug, nil)
	body := rec.Body.String()

	assert.Equal(t, 1, strings.Count(body, `name="fediverse:creator"`),
		"exactly one fediverse:creator tag")
	assert.Contains(t, body, `<meta name="fediverse:creator" content="@one@first.example">`)
	assert.NotContains(t, body, "@two@second.example")
	assert.Contains(t, body, `<link rel="me" href="https://second.example/@two" />`)
}

func TestSettingsRendersOneRowPerLink(t *testing.T) {
	app, router := newTemplateTestApp(t, multiUser)
	u, coll, _ := createTemplateTestUser(t, app, "rowsettings")

	err := saveVerificationLinks(app, coll, "https://a.example\nhttps://b.example")
	assert.NoError(t, err)

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+coll.Alias, cookies)
	assert.Equal(t, 2, strings.Count(rec.Body.String(), `name="verification_link_row"`),
		"one input row per stored link")
}

func TestSubmittingRowsSavesAllLinks(t *testing.T) {
	app, router := newTemplateTestApp(t, multiUser)
	u, coll, _ := createTemplateTestUser(t, app, "rowsubmit")

	form := url.Values{}
	form.Set("title", coll.Title)
	form.Add("verification_link_row", "https://a.example")
	form.Add("verification_link_row", "  ")
	form.Add("verification_link_row", "https://b.example")

	req := httptest.NewRequest("POST", "/api/collections/"+coll.Alias, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(loginCookie(t, app, u))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Less(t, rec.Code, 400, "settings POST should not error")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{"https://a.example", "https://b.example"}, loaded.Verifications)
}

func TestSubmittingEmptyRowClearsLinks(t *testing.T) {
	app, router := newTemplateTestApp(t, multiUser)
	u, coll, _ := createTemplateTestUser(t, app, "rowclear")

	assert.NoError(t, saveVerificationLinks(app, coll, "https://a.example"))

	form := url.Values{}
	form.Set("title", coll.Title)
	form.Add("verification_link_row", "")

	req := httptest.NewRequest("POST", "/api/collections/"+coll.Alias, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(loginCookie(t, app, u))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Less(t, rec.Code, 400, "settings POST should not error")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.Equal(t, []string{}, loaded.Verifications)
}

// TestCollectionJSONKeepsSingleLinkKey covers the compatibility shim. A
// client written against the single-link API reads verification_link, and
// dropping that key would break it silently -- the field would simply be
// absent rather than raise anything.
func TestCollectionJSONKeepsSingleLinkKey(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "compatlinks")

	assert.NoError(t, saveVerificationLinks(app, coll, "https://a.example\nhttps://b.example"))

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)

	b, err := json.Marshal(loaded)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &out))

	assert.Equal(t, []interface{}{"https://a.example", "https://b.example"}, out["verification_links"],
		"the current key carries every link")
	assert.Equal(t, "https://a.example", out["verification_link"],
		"the deprecated key carries the first, as it did when only one was possible")
}

func TestCollectionJSONSingleLinkKeyEmptyWhenUnset(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "compatnone")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)

	b, err := json.Marshal(loaded)
	assert.NoError(t, err)

	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &out))
	assert.Equal(t, "", out["verification_link"], "the key is still present, as it was before")
}
