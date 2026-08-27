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
