package writefreely

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/writeas/web-core/activitypub"
	"github.com/writeas/web-core/activitystreams"
)

var actorTestTable = []struct {
	Name string
	Resp []byte
}{
	{
		"Context as a string",
		[]byte(`{"@context":"https://www.w3.org/ns/activitystreams"}`),
	},
	{
		"Context as a list",
		[]byte(`{"@context":["one string", "two strings"]}`),
	},
}

func TestUnmarshalActor(t *testing.T) {
	for _, tc := range actorTestTable {
		actor := activitystreams.Person{}
		err := unmarshalActor(tc.Resp, &actor)
		if err != nil {
			t.Errorf("%s failed with error %s", tc.Name, err)
		}
	}
}

// TestDecodePrivateKeyPanicsOnEmptyKey documents the upstream defect that
// actorPrivKey exists to guard. web-core's DecodePrivateKey checks whether
// pem.Decode returned a nil block and then dereferences that same nil block to
// format its error message, so the nil check and the panic sit on one line. If
// web-core is ever fixed this test fails, which is the signal to reconsider the
// guard rather than to keep it out of habit.
func TestDecodePrivateKeyPanicsOnEmptyKey(t *testing.T) {
	assert.Panics(t, func() {
		//nolint:errcheck // the panic is the subject of the test
		activitypub.DecodePrivateKey(nil)
	})
}

// TestActorPrivKeyRefusesMissingKey covers the reachable case: a collection
// whose ActivityPub keypair was never generated, which happens when key
// generation fails. That must surface as an error the caller can handle, not
// as a panic inside a dependency.
func TestActorPrivKeyRefusesMissingKey(t *testing.T) {
	p := &activitystreams.Person{}
	p.ID = "https://example.com/api/collections/nokey"

	key, err := actorPrivKey(p)

	assert.Nil(t, key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no private key")
	assert.Contains(t, err.Error(), p.ID, "the error should name the actor, so the operator knows which collection is broken")
}
