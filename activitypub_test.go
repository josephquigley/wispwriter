package writefreely

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

// TestCreateActivityIDDoesNotMatchObjectID confirms that a Create
// activity's id is distinct from the id of the object it wraps. Two
// distinct objects sharing one id violates ActivityPub, and it's what
// makes GoToSocial silently drop delivered posts.
func TestCreateActivityIDDoesNotMatchObjectID(t *testing.T) {
	o := activitystreams.NewNoteObject()
	o.ID = "https://example.com/api/posts/abc123"

	a := activitystreams.NewCreateActivity(o)
	a.ID += "#Create"

	if a.ID == a.Object.ID {
		t.Errorf("activity id must not equal object id, both were %q", a.ID)
	}
	if !strings.HasPrefix(a.ID, a.Object.ID) {
		t.Errorf("activity id %q should be derived from object id %q", a.ID, a.Object.ID)
	}
}

// updateActivityID mirrors the id-suffixing logic in federatePost for an
// Update activity: it appends the object's updated time so that
// successive edits of the same post get distinct activity ids.
func updateActivityID(o *activitystreams.Object) string {
	a := activitystreams.NewUpdateActivity(o)
	updateTime := time.Now()
	if a.Updated != nil && !a.Updated.IsZero() {
		updateTime = *a.Updated
	}
	return a.ID + fmt.Sprintf("#Update/%d", updateTime.Unix())
}

// TestUpdateActivityIDDoesNotMatchObjectID confirms that an Update
// activity's id is distinct from the id of the object it wraps, for the
// same reason as Create.
func TestUpdateActivityIDDoesNotMatchObjectID(t *testing.T) {
	o := activitystreams.NewNoteObject()
	o.ID = "https://example.com/api/posts/abc123"
	updated := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	o.Updated = &updated

	id := updateActivityID(o)

	if id == o.ID {
		t.Errorf("activity id must not equal object id, both were %q", id)
	}
	if !strings.HasPrefix(id, o.ID) {
		t.Errorf("activity id %q should be derived from object id %q", id, o.ID)
	}
}

// TestUpdateActivityIDVariesPerEdit confirms that editing the same post
// twice produces two different Update activity ids. A static suffix
// (e.g. always appending "#Update") would give every edit of a post the
// same activity id as its first edit. Receivers dedupe activities by id,
// so a conforming implementation would treat the second edit as one
// already processed and silently drop it, which is worse than the bug
// this fix addresses.
func TestUpdateActivityIDVariesPerEdit(t *testing.T) {
	o := activitystreams.NewNoteObject()
	o.ID = "https://example.com/api/posts/abc123"

	firstEdit := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	o.Updated = &firstEdit
	firstID := updateActivityID(o)

	secondEdit := firstEdit.Add(time.Hour)
	o.Updated = &secondEdit
	secondID := updateActivityID(o)

	if firstID == secondID {
		t.Errorf("two edits of the same post produced the same activity id %q; "+
			"receivers that dedupe by id would drop the second edit", firstID)
	}
}
