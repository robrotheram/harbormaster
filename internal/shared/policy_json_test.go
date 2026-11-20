package shared

import (
	"encoding/json"
	"testing"
)

func TestPolicyJSON(t *testing.T) {
	for _, raw := range []string{
		`{"model":"one","model":"two"}`,
		`{"obj":[{"Mode":"one","mode":"two"}]}`,
		`{"model":"one","\u006dodel":"two"}`,
		`{"K":1,"K":2}`,
		`{}{}`,
		`[]`,
	} {
		if _, err := PolicyJSON([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	raw := `{"n":9007199254740993,"params":{"flag":null}}`
	object, err := PolicyJSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(object)
	if err != nil || string(canonical) != raw {
		t.Fatalf("re-encoding lost data: %s (%v)", canonical, err)
	}
}
