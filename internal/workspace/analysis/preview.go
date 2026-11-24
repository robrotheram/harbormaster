package analysis

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strings"

	"agent-workbench/internal/shared"
)

const maxPage = 500
const maxPoints = 5000

type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

type Preview struct {
	Limit   int      `json:"limit,omitempty"`
	Cursor  string   `json:"cursor,omitempty"`
	Columns []string `json:"columns,omitempty"`
	Sort    []Sort   `json:"sort,omitempty"`
	Filters []Filter `json:"filters,omitempty"`
}

type dataRequest struct {
	Preview
	Offset uint64 `json:"offset,omitempty"`
	Sample bool   `json:"sample,omitempty"`
}

func (p *Preview) valid(cap int) bool {
	if p.Limit == 0 {
		p.Limit = min(100, cap)
	}
	if p.Limit < 1 || p.Limit > cap || len(p.Columns) > 512 || len(p.Sort) > 8 || len(p.Filters) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, column := range p.Columns {
		if !validField(column) || seen[column] {
			return false
		}
		seen[column] = true
	}
	for i, order := range p.Sort {
		order.Direction = strings.ToLower(order.Direction)
		if !validField(order.Field) || order.Direction != "asc" && order.Direction != "desc" {
			return false
		}
		p.Sort[i] = order
	}
	for _, filter := range p.Filters {
		if !validField(filter.Field) {
			return false
		}
		switch filter.Op {
		case "is_null", "is_not_null":
			if filter.Value != nil {
				return false
			}
		case "contains":
			if value, ok := filter.Value.(string); !ok || len(value) > 512 {
				return false
			}
		case "eq", "ne", "lt", "lte", "gt", "gte":
			switch value := filter.Value.(type) {
			case float64, bool:
			case string:
				if len(value) > 512 {
					return false
				}
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validField(field string) bool {
	return len(field) > 0 && len(field) <= 256 && !strings.ContainsRune(field, 0)
}

func (s *Store) cursor(data []byte, position uint64) []byte {
	b := binary.BigEndian.AppendUint64(nil, position)
	sign := hmac.New(sha256.New, []byte(s.token))
	sign.Write([]byte("analysis-cursor-v1\x00"))
	sign.Write(data)
	sign.Write(b)
	return sign.Sum(b)
}

func (s *Store) previewDataset(w http.ResponseWriter, r *http.Request) {
	var p Preview
	if shared.Decode(w, r, &p, shared.AnalysisRequestLimit) {
		s.serveData(w, r, r.PathValue("dataset"), "dataset", p, maxPage, false)
	}
}

func (s *Store) serveData(w http.ResponseWriter, r *http.Request, id, kind string, p Preview, limit int, sample bool) {
	if !p.valid(limit) {
		shared.Error(w, 400, "INVALID_ANALYSIS_PREVIEW")
		return
	}
	root, err := s.open()
	if err != nil {
		shared.Error(w, 503, "ANALYSIS_STORAGE_UNAVAILABLE")
		return
	}
	defer root.Close()
	dataset, err := readDataset(root, id, kind)
	if err != nil {
		shared.Error(w, 404, "ANALYSIS_DATA_UNAVAILABLE")
		return
	}
	cursor := p.Cursor
	p.Cursor = ""
	scope, _ := json.Marshal([]any{id, dataset.SHA256, p})
	var offset uint64
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) != 8+sha256.Size || sample {
			shared.Error(w, 400, "INVALID_ANALYSIS_CURSOR")
			return
		}
		offset = binary.BigEndian.Uint64(decoded)
		if offset > 1<<32 || !hmac.Equal(decoded, s.cursor(scope, offset)) {
			shared.Error(w, 400, "INVALID_ANALYSIS_CURSOR")
			return
		}
	}
	request := engineRequest{Format: dataset.Format, Columns: dataset.Columns,
		Preview: &dataRequest{Preview: p, Offset: offset, Sample: sample}}
	var result engineResult
	if !s.query(w, r, root, id+"/data."+dataset.Format, request, &result) {
		return
	}
	response := struct {
		engineResult
		NextCursor *string `json:"next_cursor"`
	}{engineResult: result}
	if result.HasMore && limit == maxPage {
		next := base64.RawURLEncoding.EncodeToString(s.cursor(scope, offset+uint64(len(result.Rows))))
		response.NextCursor = &next
	} else if result.HasMore && !result.Sampled {
		response.Truncated = true
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(response); err != nil || encoded.Len() > maxResponse {
		shared.Error(w, 422, "ANALYSIS_RESPONSE_TOO_LARGE")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded.Bytes())
}
