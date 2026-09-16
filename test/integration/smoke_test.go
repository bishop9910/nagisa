//go:build integration

// Package integration drives a running netdisk server through its public HTTP
// API. It is the end to end check that the pieces fit together: the password
// key handshake, the authority hierarchy, the folder ACL and password model,
// the tree operations and the audit trail.
//
// Start a server first, then run:
//
//	go test -tags integration ./test/integration/ -v
//
// The base URL defaults to http://127.0.0.1:18000 and can be overridden with
// NETDISK_BASE_URL. The expected administrator password comes from
// NETDISK_ADMIN, defaulting to the value used by configs/config.yaml in a local
// run; the guest needs no credential because it is entered through the public
// POST /v1/auth/guest.
package integration

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type client struct {
	base  string
	http  *http.Client
	token string
	node  string
}

func newClient(t *testing.T) *client {
	t.Helper()
	base := os.Getenv("NETDISK_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:18000"
	}
	c := &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
	if err := c.ping(); err != nil {
		t.Skipf("no netdisk server at %s: %v", c.base, err)
	}
	return c
}

func (c *client) ping() error {
	resp, err := c.http.Get(c.base + "/v1/system/health")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return nil
}

// try performs a request and reports a transport or status failure as an
// error, so a goroutine can hand it back to the test goroutine.
func (c *client) try(method, path string, body any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// do performs a request and decodes the reply. When wantStatus is non-zero the
// call is expected to fail with that status and no error is reported for it.
func (c *client) do(t *testing.T, method, path string, body any, wantStatus int) (map[string]any, int) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.node != "" {
		req.Header.Set("X-Node-Token", c.node)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if wantStatus != 0 {
		if resp.StatusCode != wantStatus {
			t.Fatalf("%s %s: want status %d, got %d (%s)", method, path, wantStatus, resp.StatusCode, string(raw))
		}
		return out, resp.StatusCode
	}
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: status %d (%s)", method, path, resp.StatusCode, string(raw))
	}
	return out, resp.StatusCode
}

// passwordKey is the RSA key clients encrypt password fields with.
type passwordKey struct {
	key   *rsa.PublicKey
	keyID string
}

func (c *client) passwordKey(t *testing.T) *passwordKey {
	t.Helper()
	out, _ := c.do(t, http.MethodGet, "/v1/auth/config", nil, 0)
	pemText, _ := out["passwordPublicKey"].(string)
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		t.Fatalf("auth config did not return a PEM public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key is not RSA")
	}
	if encoding, _ := out["passwordEncoding"].(string); encoding != "rsa-oaep-sha256" {
		t.Fatalf("unexpected password encoding %q", encoding)
	}
	keyID, _ := out["passwordKeyId"].(string)
	return &passwordKey{key: key, keyID: keyID}
}

// encode encrypts a password the way the API requires it.
func (k *passwordKey) encode(t *testing.T, plain string) string {
	t.Helper()
	sealed, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, k.key, []byte(plain), nil)
	if err != nil {
		t.Fatalf("encrypt password: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sealed)
}

func (c *client) login(t *testing.T, key *passwordKey, username, password string) map[string]any {
	t.Helper()
	out, _ := c.do(t, http.MethodPost, "/v1/auth/login", map[string]any{
		"username": username,
		"password": key.encode(t, password),
	}, 0)
	return out
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// num reads a numeric field. 64 bit integers travel as JSON strings in the
// protobuf JSON mapping, so both forms are accepted here.
func num(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case string:
		var out float64
		_, _ = fmt.Sscanf(v, "%g", &out)
		return out
	default:
		return 0
	}
}

func list(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

// TestSmoke walks the whole surface in the order a front end would.
func TestSmoke(t *testing.T) {
	admin := newClient(t)
	key := admin.passwordKey(t)
	t.Logf("password key %s, encoding rsa-oaep-sha256", key.keyID)

	// Every run uses its own names so the suite can be replayed against a
	// database that already holds the previous run's objects.
	run := strconv.FormatInt(time.Now().UnixNano()%1000000, 10)
	aliceUser := "alice" + run
	bobUser := "bob" + run
	carolUser := "carol" + run
	malloryUser := "mallory" + run
	docsName := "documents-" + run
	secretName := "secret-" + run
	subName := "y2026-" + run

	adminPassword := envOr("NETDISK_ADMIN", "Admin@12345")

	t.Run("public endpoints", func(t *testing.T) {
		info, _ := admin.do(t, http.MethodGet, "/v1/system/info", nil, 0)
		if got := str(info, "databaseBackend"); got != "sqlite" {
			t.Errorf("database_backend = %q, want sqlite", got)
		}
		if list(info, "features") == nil {
			t.Errorf("system info advertised no features")
		}
		if _, status := admin.do(t, http.MethodGet, "/v1/system/health", nil, 0); status != http.StatusOK {
			t.Errorf("health status %d", status)
		}
	})

	// A protected route must reject an anonymous caller before anything else
	// happens.
	if _, status := admin.do(t, http.MethodGet, "/v1/nodes/list", nil, http.StatusUnauthorized); status != http.StatusUnauthorized {
		t.Fatalf("anonymous listing returned %d", status)
	}

	t.Run("login", func(t *testing.T) {
		// A wrong password must fail with the same shape as an unknown account.
		wrong, status := admin.do(t, http.MethodPost, "/v1/auth/login", map[string]any{
			"username": "admin", "password": key.encode(t, "definitely-wrong"),
		}, http.StatusUnauthorized)
		if str(wrong, "reason") != "NETDISK_UNAUTHENTICATED" {
			t.Errorf("wrong password reason = %q", str(wrong, "reason"))
		}
		_ = status

		reply := admin.login(t, key, "admin", adminPassword)
		admin.token = str(reply, "accessToken")
		if admin.token == "" {
			t.Fatalf("login returned no access token")
		}
		me, _ := admin.do(t, http.MethodGet, "/v1/auth/me", nil, 0)
		if str(me, "username") != "admin" {
			t.Fatalf("me returned %q", str(me, "username"))
		}
		if num(me, "rank") != 1000 {
			t.Errorf("admin rank = %v, want 1000", num(me, "rank"))
		}
		if num(me, "manageable") != 0 {
			t.Errorf("admin reported itself as manageable")
		}
	})

	guest := newClient(t)
	guestKey := guest.passwordKey(t)
	// The built-in guest account carries no usable password: the public
	// POST /v1/auth/guest is its only entrance, and a password login for it
	// must fail like any other bad credential.
	refused, _ := guest.do(t, http.MethodPost, "/v1/auth/login", map[string]any{
		"username": "guest", "password": guestKey.encode(t, "Guest@12345"),
	}, http.StatusUnauthorized)
	if str(refused, "reason") != "NETDISK_UNAUTHENTICATED" {
		t.Errorf("guest password login reason = %q", str(refused, "reason"))
	}
	guestReply, _ := guest.do(t, http.MethodPost, "/v1/auth/guest", nil, 0)
	guest.token = str(guestReply, "accessToken")
	if guest.token == "" {
		t.Fatalf("guest login returned no token")
	}
	if got := num(guestReply["user"].(map[string]any), "permissionsMask"); got != 3 {
		t.Errorf("guest permissions mask = %v, want 3 (view|download)", got)
	}

	t.Run("guest is read only", func(t *testing.T) {
		guest.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder": map[string]any{"name": "guest-should-not"},
		}, http.StatusForbidden)
		guest.do(t, http.MethodPost, "/v1/users/create", map[string]any{
			"user":     map[string]any{"username": malloryUser, "rank": 5},
			"password": guestKey.encode(t, "Mallory@123"),
		}, http.StatusForbidden)
	})

	t.Run("only a manager can create accounts", func(t *testing.T) {
		presets, _ := admin.do(t, http.MethodGet, "/v1/users/roles/list", nil, 0)
		if len(list(presets, "presets")) != 4 {
			t.Errorf("expected four role presets, got %d", len(list(presets, "presets")))
		}
		if num(presets, "maxGrantableRank") != 999 {
			t.Errorf("admin max grantable rank = %v, want 999", num(presets, "maxGrantableRank"))
		}
	})

	managerPermissions := []string{
		"PERMISSION_VIEW", "PERMISSION_DOWNLOAD", "PERMISSION_UPLOAD", "PERMISSION_EDIT",
		"PERMISSION_DELETE", "PERMISSION_TRASH_MANAGE", "PERMISSION_SHARE",
		"PERMISSION_ACL_MANAGE", "PERMISSION_USER_MANAGE",
	}
	aliceOut, _ := admin.do(t, http.MethodPost, "/v1/users/create", map[string]any{
		"user": map[string]any{
			"username":    aliceUser,
			"nickname":    "Alice",
			"role":        "ROLE_MANAGER",
			"rank":        500,
			"permissions": managerPermissions,
			"quota_bytes": 10737418240,
		},
		"password": key.encode(t, "Alice@12345"),
	}, 0)
	aliceID := str(aliceOut, "id")
	if aliceID == "" {
		t.Fatalf("creating the manager returned no id: %v", aliceOut)
	}
	if num(aliceOut, "rank") != 500 {
		t.Errorf("manager rank = %v, want 500", num(aliceOut, "rank"))
	}

	t.Run("authority hierarchy", func(t *testing.T) {
		// The admin outranks the manager, which outranks a plain account.
		admin.do(t, http.MethodPost, "/v1/users/create", map[string]any{
			"user":     map[string]any{"username": carolUser, "role": "ROLE_USER", "rank": 1000},
			"password": key.encode(t, "Carol@12345"),
		}, http.StatusForbidden)

		alice := newClient(t)
		aliceKey := alice.passwordKey(t)
		aliceReply := alice.login(t, aliceKey, aliceUser, "Alice@12345")
		alice.token = str(aliceReply, "accessToken")
		if alice.token == "" {
			t.Fatalf("manager login failed")
		}
		// A manager may not touch the built-in administrator.
		alice.do(t, http.MethodPut, "/v1/users/update", map[string]any{
			"user":       map[string]any{"id": str(mustGet(t, admin, "/v1/auth/me"), "id"), "nickname": "pwned"},
			"updateMask": "nickname",
		}, http.StatusForbidden)

		// A manager may create an account below its own rank.
		bobOut, _ := alice.do(t, http.MethodPost, "/v1/users/create", map[string]any{
			"user":     map[string]any{"username": bobUser, "role": "ROLE_USER", "rank": 100},
			"password": aliceKey.encode(t, "Bob@12345"),
		}, 0)
		bobID := str(bobOut, "id")
		if bobID == "" {
			t.Fatalf("manager could not create a lower ranked account")
		}
		// ... but may not promote it above itself.
		alice.do(t, http.MethodPut, "/v1/users/update", map[string]any{
			"user":       map[string]any{"id": bobID, "rank": 900},
			"updateMask": "rank",
		}, http.StatusForbidden)
		// ... and may not grant a permission it does not hold.
		alice.do(t, http.MethodPut, "/v1/users/update", map[string]any{
			"user":       map[string]any{"id": bobID, "permissionsMask": 2048},
			"updateMask": "permissionsMask",
		}, http.StatusForbidden)
	})

	docsOut, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
		"folder": map[string]any{
			"name":        docsName,
			"description": "shared project material",
			"visibility":  "VISIBILITY_PRIVATE",
		},
		"conflict_policy": "CONFLICT_POLICY_FAIL",
	}, 0)
	docsID := str(docsOut, "id")
	if docsID == "" {
		t.Fatalf("folder creation failed: %v", docsOut)
	}
	if got := str(docsOut, "displayPath"); got != "/我的网盘/"+docsName {
		t.Errorf("display path = %q", got)
	}

	t.Run("name conflicts", func(t *testing.T) {
		admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder":          map[string]any{"name": docsName},
			"conflict_policy": "CONFLICT_POLICY_FAIL",
		}, http.StatusConflict)
		renamed, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder":          map[string]any{"name": docsName},
			"conflict_policy": "CONFLICT_POLICY_RENAME",
		}, 0)
		if got := str(renamed, "name"); got != docsName+" (1)" {
			t.Errorf("auto rename produced %q, want %q", got, docsName+" (1)")
		}
	})

	// A folder with a description, an allow list, a deny list and a password.
	secretOut, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
		"folder": map[string]any{
			"name":        secretName,
			"description": "password protected",
		},
		"password": key.encode(t, "Folder@123"),
		"acl": []map[string]any{
			{
				"subject_type": "SUBJECT_TYPE_USER", "subject_id": aliceID,
				"effect": "EFFECT_ALLOW", "inherit": true,
				"permissions": []string{"PERMISSION_VIEW", "PERMISSION_DOWNLOAD"},
			},
			{
				"subject_type": "SUBJECT_TYPE_EVERYONE", "effect": "EFFECT_DENY",
				"inherit": true, "permissions": []string{"PERMISSION_UPLOAD"},
			},
		},
	}, 0)
	secretID := str(secretOut, "id")
	if !bools(secretOut, "passwordProtected") {
		t.Errorf("folder is not marked as password protected")
	}

	t.Run("folder acl", func(t *testing.T) {
		acl, _ := admin.do(t, http.MethodGet, "/v1/nodes/"+secretID+"/acl", nil, 0)
		if len(list(acl, "entries")) != 2 {
			t.Errorf("expected two acl entries, got %d", len(list(acl, "entries")))
		}
		if !bools(acl, "editable") {
			t.Errorf("the owner cannot edit the acl")
		}
	})

	t.Run("folder password", func(t *testing.T) {
		alice := newClient(t)
		aliceKey := alice.passwordKey(t)
		reply := alice.login(t, aliceKey, aliceUser, "Alice@12345")
		alice.token = str(reply, "accessToken")

		alice.do(t, http.MethodGet, "/v1/nodes/list?parent_id="+secretID, nil, http.StatusForbidden)
		unlock, _ := alice.do(t, http.MethodPost, "/v1/nodes/"+secretID+"/unlock", map[string]any{
			"password": aliceKey.encode(t, "Folder@123"),
		}, 0)
		alice.node = str(unlock, "unlockToken")
		if alice.node == "" {
			t.Fatalf("unlock returned no token")
		}
		alice.do(t, http.MethodGet, "/v1/nodes/list?parent_id="+secretID, nil, 0)

		// Without the node token the folder is locked again.
		alice.node = ""
		alice.do(t, http.MethodGet, "/v1/nodes/list?parent_id="+secretID, nil, http.StatusForbidden)
	})

	t.Run("tree operations", func(t *testing.T) {
		sub, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder": map[string]any{"name": subName, "parent_id": docsID},
		}, 0)
		subID := str(sub, "id")
		leaf, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder": map[string]any{"name": "q1", "parent_id": subID},
		}, 0)
		leafID := str(leaf, "id")

		path, _ := admin.do(t, http.MethodGet, "/v1/nodes/"+leafID+"/path", nil, 0)
		if got := str(path, "displayPath"); got != "/我的网盘/"+docsName+"/"+subName+"/q1" {
			t.Errorf("breadcrumb = %q", got)
		}

		// A folder may not be moved into its own subtree.
		admin.do(t, http.MethodPost, "/v1/nodes/move", map[string]any{
			"ids": []string{docsID}, "target_parent_id": leafID,
		}, http.StatusBadRequest)

		moved, _ := admin.do(t, http.MethodPost, "/v1/nodes/move", map[string]any{
			"ids": []string{subID}, "target_parent_id": docsID,
		}, 0)
		if len(list(moved, "nodes")) != 1 {
			t.Fatalf("move returned %d nodes", len(list(moved, "nodes")))
		}

		copied, _ := admin.do(t, http.MethodPost, "/v1/nodes/copy", map[string]any{
			"ids": []string{subID}, "target_parent_id": docsID,
			"conflict_policy": "CONFLICT_POLICY_RENAME",
		}, 0)
		if got := str(list(copied, "nodes")[0].(map[string]any), "name"); got != subName+" (1)" {
			t.Errorf("copy name = %q", got)
		}

		tree, _ := admin.do(t, http.MethodGet, "/v1/nodes/tree?root_id="+docsID+"&depth=3", nil, 0)
		if len(list(tree, "children")) == 0 {
			t.Errorf("tree returned no children")
		}
		stats, _ := admin.do(t, http.MethodGet, "/v1/nodes/"+docsID+"/stats", nil, 0)
		if num(stats, "folderCount") == 0 {
			t.Errorf("subtree stats reported no folders")
		}
	})

	t.Run("trash", func(t *testing.T) {
		sub, _ := admin.do(t, http.MethodGet, "/v1/nodes/list?parent_id="+docsID, nil, 0)
		if len(list(sub, "nodes")) == 0 {
			t.Fatalf("documents is empty")
		}
		victim := str(list(sub, "nodes")[0].(map[string]any), "id")

		del, _ := admin.do(t, http.MethodPost, "/v1/nodes/delete", map[string]any{"ids": []string{victim}}, 0)
		if num(del, "affectedCount") == 0 {
			t.Errorf("delete affected nothing")
		}
		trash, _ := admin.do(t, http.MethodGet, "/v1/nodes/trash/list", nil, 0)
		if num(trash, "totalSize") == 0 {
			t.Errorf("trash is empty after a delete")
		}
		restored, _ := admin.do(t, http.MethodPost, "/v1/nodes/trash/restore", map[string]any{
			"ids": []string{victim}, "conflict_policy": "CONFLICT_POLICY_RENAME",
		}, 0)
		if len(list(restored, "nodes")) != 1 {
			t.Errorf("restore returned %d nodes", len(list(restored, "nodes")))
		}
	})

	t.Run("shares", func(t *testing.T) {
		share, _ := admin.do(t, http.MethodPost, "/v1/shares/create", map[string]any{
			"node_id":      docsID,
			"name":         "documents share",
			"description":  "for external colleagues",
			"permissions":  []string{"PERMISSION_VIEW", "PERMISSION_DOWNLOAD"},
			"password":     key.encode(t, "Share@123"),
			"passwordHint": "ask the team",

			"maxDownloads": 10,
		}, 0)
		token := str(share, "token")
		if token == "" {
			t.Fatalf("share creation returned no token")
		}
		if !bools(share, "passwordProtected") {
			t.Errorf("share is not marked as protected")
		}

		anonymous := newClient(t)
		locked, _ := anonymous.do(t, http.MethodPost, "/v1/shares/access", map[string]any{
			"token": token,
		}, http.StatusForbidden)
		hint, _ := locked["metadata"].(map[string]any)
		if str(hint, "password_hint") != "ask the team" {
			t.Errorf("the locked error did not carry the password hint: %v", locked)
		}
		if str(locked, "reason") != "NETDISK_NODE_LOCKED" {
			t.Errorf("locked reason = %q", str(locked, "reason"))
		}

		access, _ := anonymous.do(t, http.MethodPost, "/v1/shares/access", map[string]any{
			"token": token, "password": key.encode(t, "Share@123"),
		}, 0)
		if str(access, "node") == "" && access["node"] == nil {
			t.Fatalf("share access returned no node")
		}
		accessToken := str(access, "accessToken")
		if accessToken == "" {
			t.Fatalf("share access returned no access token")
		}
		children, _ := anonymous.do(t, http.MethodPost, "/v1/shares/children/list", map[string]any{
			"token": token, "node_id": docsID, "accessToken": accessToken,
		}, 0)
		if list(children, "nodes") == nil {
			t.Errorf("anonymous listing returned no nodes field")
		}
	})

	t.Run("concurrent writes", func(t *testing.T) {
		// SQLite serialises writers and the server serialises them again with
		// an in-process lock, so a burst of parallel mutations must all
		// succeed. This is the regression guard for the deferred transaction
		// upgrade that used to fail with "database is locked".
		const workers, perWorker = 8, 4
		root, _ := admin.do(t, http.MethodPost, "/v1/nodes/folders/create", map[string]any{
			"folder": map[string]any{"name": "concurrent-" + run},
		}, 0)
		rootID := str(root, "id")
		if rootID == "" {
			t.Fatalf("could not create the concurrent write root")
		}

		var wg sync.WaitGroup
		errs := make(chan error, workers*perWorker)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					body := map[string]any{
						"folder": map[string]any{
							"name":      fmt.Sprintf("w%d-%d", worker, i),
							"parent_id": rootID,
						},
					}
					if err := admin.try(http.MethodPost, "/v1/nodes/folders/create", body); err != nil {
						errs <- fmt.Errorf("worker %d item %d: %w", worker, i, err)
						return
					}
				}
			}(w)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("concurrent write failed: %v", err)
		}

		listing, _ := admin.do(t, http.MethodGet, "/v1/nodes/list?parent_id="+rootID+"&page_size=100", nil, 0)
		if got := len(list(listing, "nodes")); got != workers*perWorker {
			t.Errorf("after the burst the folder holds %d children, want %d", got, workers*perWorker)
		}
	})
	t.Run("audit trail", func(t *testing.T) {
		// The auditor batches writes, so give it a moment to flush.
		deadline := time.Now().Add(5 * time.Second)
		var logs map[string]any
		for time.Now().Before(deadline) {
			logs, _ = admin.do(t, http.MethodGet, "/v1/audit/logs/list?page_size=50", nil, 0)
			if num(logs, "totalSize") > 0 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if num(logs, "totalSize") == 0 {
			t.Fatalf("the audit trail stayed empty")
		}
		entries := list(logs, "logs")
		found := map[string]bool{}
		for _, raw := range entries {
			entry, _ := raw.(map[string]any)
			found[str(entry, "action")] = true
			if str(entry, "actionDisplay") == "" {
				t.Errorf("action %q has no display label", str(entry, "action"))
			}
		}
		for _, want := range []string{"auth.login", "user.create", "node.create_folder", "node.delete", "share.create"} {
			if !found[want] {
				t.Errorf("audit trail is missing %q", want)
			}
		}

		// A caller without the audit permission only sees its own entries.
		guestLogs, _ := guest.do(t, http.MethodGet, "/v1/audit/logs/list", nil, 0)
		for _, raw := range list(guestLogs, "logs") {
			entry, _ := raw.(map[string]any)
			if str(entry, "actorName") != "guest" {
				t.Errorf("guest saw an entry from %q", str(entry, "actorName"))
			}
		}
	})

	t.Run("system and maintenance", func(t *testing.T) {
		stats, _ := admin.do(t, http.MethodGet, "/v1/system/storage/stats", nil, 0)
		if num(stats, "totalFolders") == 0 {
			t.Errorf("storage stats reported no folders")
		}
		guest.do(t, http.MethodGet, "/v1/system/storage/stats", nil, http.StatusForbidden)

		report, _ := admin.do(t, http.MethodPost, "/v1/system/maintenance/run", map[string]any{"dry_run": true}, 0)
		if len(list(report, "tasks")) == 0 {
			t.Errorf("maintenance dry run reported no tasks")
		}
		settings, _ := admin.do(t, http.MethodGet, "/v1/system/settings/list", nil, 0)
		if len(list(settings, "settings")) == 0 {
			t.Errorf("no system settings are exposed")
		}
		admin.do(t, http.MethodPut, "/v1/system/settings/update", map[string]any{
			"settings": []map[string]any{{"key": "system.version", "value": "hacked"}},
		}, http.StatusForbidden)
	})
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func bools(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func mustGet(t *testing.T, c *client, path string) map[string]any {
	t.Helper()
	out, _ := c.do(t, http.MethodGet, path, nil, 0)
	return out
}
