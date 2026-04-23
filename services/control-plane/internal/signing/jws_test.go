package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
)

func TestSign_RoundTrips(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"schema_version":  1,
		"deployment_id":   "abc",
		"desired_version": "2.4.0",
		"b":               []any{2, 1},
		"a":               "x",
	}
	jws, err := Sign(priv, "release-mgr-2026-04", payload)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 parts, got %d", len(parts))
	}
	hdr, body, sig, err := SplitCompact(jws)
	if err != nil {
		t.Fatal(err)
	}
	var h Header
	if err := json.Unmarshal(hdr, &h); err != nil {
		t.Fatal(err)
	}
	if h.Alg != "EdDSA" || h.Kid != "release-mgr-2026-04" {
		t.Fatalf("bad header: %+v", h)
	}
	encHdr, encBody, _, _ := CompactParts(jws)
	signing := []byte(encHdr + "." + encBody)
	if !ed25519.Verify(pub, signing, sig) {
		t.Fatal("signature verify failed")
	}
	// Body must be JCS-canonical (keys sorted; "a" before "b").
	if !strings.Contains(string(body), `"a":"x","b":[2,1]`) {
		t.Fatalf("payload not JCS-canonical: %s", body)
	}
}

func TestJCS_DeterministicOutput(t *testing.T) {
	a, _ := JCS(map[string]any{"b": 1, "a": 2, "c": []any{3, 1, 2}})
	b, _ := JCS(map[string]any{"a": 2, "c": []any{3, 1, 2}, "b": 1})
	if string(a) != string(b) {
		t.Fatalf("JCS not deterministic:\n%s\n%s", a, b)
	}
	if string(a) != `{"a":2,"b":1,"c":[3,1,2]}` {
		t.Fatalf("unexpected: %s", a)
	}
}

func TestJCS_NestedObjects(t *testing.T) {
	got, _ := JCS(map[string]any{
		"outer": map[string]any{"z": 1, "a": 2},
	})
	if string(got) != `{"outer":{"a":2,"z":1}}` {
		t.Fatalf("nested not sorted: %s", got)
	}
}
