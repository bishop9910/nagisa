package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "nagisa/api/netdisk/v1"
)

// The query decoder is the piece that makes the advertised camelCase parameter
// names work, so it gets its own coverage: a silent failure here would make
// every filtered listing ignore its filters.
func TestProtoJSONQueryDecoder(t *testing.T) {
	cases := []struct {
		name  string
		query string
		check func(*testing.T, *v1.ListNodesRequest)
	}{
		{
			name:  "camelCase names from the OpenAPI document",
			query: "parentId=abc&pageSize=25&pageToken=tok&recursive=true&maxDepth=3&includeTrashed=true",
			check: func(t *testing.T, req *v1.ListNodesRequest) {
				if req.ParentId != "abc" || req.PageSize != 25 || req.PageToken != "tok" {
					t.Errorf("string and int parameters were not bound: %+v", req)
				}
				if !req.Recursive || req.MaxDepth != 3 || !req.IncludeTrashed {
					t.Errorf("boolean and int parameters were not bound: %+v", req)
				}
			},
		},
		{
			name:  "original proto names still work",
			query: "parent_id=abc&page_size=25&include_trashed=true",
			check: func(t *testing.T, req *v1.ListNodesRequest) {
				if req.ParentId != "abc" || req.PageSize != 25 || !req.IncludeTrashed {
					t.Errorf("snake_case parameters were not bound: %+v", req)
				}
			},
		},
		{
			name:  "unknown parameters are ignored",
			query: "parentId=abc&whatever=1",
			check: func(t *testing.T, req *v1.ListNodesRequest) {
				if req.ParentId != "abc" {
					t.Errorf("the known parameter was dropped: %+v", req)
				}
			},
		},
		{
			name:  "an empty value leaves the default in place",
			query: "parentId=&pageSize=",
			check: func(t *testing.T, req *v1.ListNodesRequest) {
				if req.ParentId != "" || req.PageSize != 0 {
					t.Errorf("an empty value was bound: %+v", req)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &v1.ListNodesRequest{}
			httpReq := httptest.NewRequest(http.MethodGet, "/v1/nodes/list?"+tc.query, nil)
			if err := ProtoJSONQueryDecoder(httpReq, req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			tc.check(t, req)
		})
	}
}

func TestProtoJSONQueryDecoderRejectsBadValues(t *testing.T) {
	for _, query := range []string{"pageSize=not-a-number", "recursive=maybe"} {
		req := &v1.ListNodesRequest{}
		httpReq := httptest.NewRequest(http.MethodGet, "/v1/nodes/list?"+query, nil)
		if err := ProtoJSONQueryDecoder(httpReq, req); err == nil {
			t.Errorf("query %q was accepted", query)
		}
	}
}

// The search request carries an enum, an int64 and two timestamps, which is
// exactly the combination the canonical JSON mapping renders differently from
// the reflection based default.
func TestProtoJSONQueryDecoderHandlesEnumInt64AndTimestamp(t *testing.T) {
	req := &v1.SearchNodesRequest{}
	httpReq := httptest.NewRequest(http.MethodGet,
		"/v1/nodes/search?kind=NODE_KIND_FOLDER&minSize=1048576&updatedAfter=2026-01-02T03:04:05Z", nil)
	if err := ProtoJSONQueryDecoder(httpReq, req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Kind != v1.NodeKind_NODE_KIND_FOLDER {
		t.Errorf("enum by name = %v", req.Kind)
	}
	if req.MinSize != 1048576 {
		t.Errorf("int64 = %d", req.MinSize)
	}
	if req.UpdatedAfter == nil || !req.UpdatedAfter.AsTime().Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("timestamp = %v", req.UpdatedAfter)
	}

	req = &v1.SearchNodesRequest{}
	httpReq = httptest.NewRequest(http.MethodGet, "/v1/nodes/search?kind=1", nil)
	if err := ProtoJSONQueryDecoder(httpReq, req); err != nil {
		t.Fatalf("decode enum by number: %v", err)
	}
	if req.Kind != v1.NodeKind_NODE_KIND_FOLDER {
		t.Errorf("enum by number = %v", req.Kind)
	}
}

func TestProtoJSONRequestAndResponse(t *testing.T) {
	body := `{
		"folder": {"name": "docs", "visibility": "VISIBILITY_INTERNAL", "size": "4096"},
		"conflictPolicy": "CONFLICT_POLICY_OVERWRITE",
		"acl": [{"subjectType": "SUBJECT_TYPE_ROLE", "subjectId": "guest", "effect": "EFFECT_DENY",
		         "permissions": ["PERMISSION_UPLOAD"]}]
	}`
	req := &v1.CreateFolderRequest{}
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/nodes/folders/create", strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if err := ProtoJSONRequestDecoder(httpReq, req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.GetFolder().GetVisibility() != v1.Visibility_VISIBILITY_INTERNAL {
		t.Errorf("enum name was not decoded: %v", req.GetFolder().GetVisibility())
	}
	if req.GetFolder().GetSize() != 4096 {
		t.Errorf("an int64 written as a string was not decoded: %d", req.GetFolder().GetSize())
	}
	if req.GetConflictPolicy() != v1.ConflictPolicy_CONFLICT_POLICY_OVERWRITE {
		t.Errorf("conflict policy = %v", req.GetConflictPolicy())
	}
	acl := req.GetAcl()
	if len(acl) != 1 || acl[0].GetEffect() != v1.Effect_EFFECT_DENY ||
		acl[0].GetPermissions()[0] != v1.Permission_PERMISSION_UPLOAD {
		t.Errorf("acl was not decoded: %+v", acl)
	}

	// A read-only field echoed back by a client must not break the request.
	echo := `{"folder": {"name": "docs", "unknownField": 1}, "somethingNew": true}`
	req = &v1.CreateFolderRequest{}
	httpReq = httptest.NewRequest(http.MethodPost, "/v1/nodes/folders/create", strings.NewReader(echo))
	httpReq.Header.Set("Content-Type", "application/json")
	if err := ProtoJSONRequestDecoder(httpReq, req); err != nil {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}

	// An empty body is not an error; the handler decides what is required.
	req = &v1.CreateFolderRequest{}
	httpReq = httptest.NewRequest(http.MethodPost, "/v1/nodes/folders/create", strings.NewReader(""))
	if err := ProtoJSONRequestDecoder(httpReq, req); err != nil {
		t.Fatalf("empty body: %v", err)
	}

	// A malformed body must be reported, not silently ignored.
	req = &v1.CreateFolderRequest{}
	httpReq = httptest.NewRequest(http.MethodPost, "/v1/nodes/folders/create", strings.NewReader("{not json"))
	httpReq.Header.Set("Content-Type", "application/json")
	if err := ProtoJSONRequestDecoder(httpReq, req); err == nil {
		t.Errorf("a malformed body was accepted")
	}

	// The reply uses the same mapping, which is what keeps the document
	// accurate: enum names, int64 as a string, camelCase field names.
	recorder := httptest.NewRecorder()
	reply := &v1.Node{Id: "n1", Name: "报告.pdf", Kind: v1.NodeKind_NODE_KIND_FILE, Size: 1048576,
		Visibility: v1.Visibility_VISIBILITY_PRIVATE}
	if err := ProtoJSONResponseEncoder(recorder, httptest.NewRequest(http.MethodGet, "/", nil), reply); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content type = %q", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("the reply is not JSON: %v", err)
	}
	if decoded["kind"] != "NODE_KIND_FILE" {
		t.Errorf("enum was not rendered by name: %v", decoded["kind"])
	}
	if decoded["size"] != "1048576" {
		t.Errorf("int64 was not rendered as a string: %v (%T)", decoded["size"], decoded["size"])
	}
	if decoded["visibility"] != "VISIBILITY_PRIVATE" {
		t.Errorf("visibility = %v", decoded["visibility"])
	}
	if decoded["name"] != "报告.pdf" {
		t.Errorf("name = %v", decoded["name"])
	}
}
