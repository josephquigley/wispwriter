package writefreely

import (
	"testing"

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
