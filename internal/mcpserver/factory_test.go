package mcpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestFactoryEvidenceFitsProjectBroker(t *testing.T) {
	for _, size := range []int{512 * 1024, 512*1024 + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			session, _ := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"artifact_id": "artifact1", "content": base64.StdEncoding.EncodeToString(make([]byte, size))})
			}))
			result := call(t, session, "factory_evidence_read", map[string]any{"workspace_id": "workspace1", "id": "artifact1"})
			if result.IsError != (size > 512*1024) {
				t.Fatalf("size %d error=%v", size, result.IsError)
			}
			b, err := json.Marshal(result)
			if err != nil || len(b) > 2<<20 {
				t.Fatalf("broker response: %d bytes, %v", len(b), err)
			}
		})
	}
}
