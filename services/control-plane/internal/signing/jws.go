// Package signing provides JWS / EdDSA / JCS signing of manifests
// (arc42 §5.8, ADR-0003). Used by:
//   - the `oc-sign` CLI (release manager flow),
//   - the test suite (golden fixture generation),
//   - and — symmetrically — by the Rust manifest-verifier.
//
// The implementation is deliberately small and dependency-free apart from
// the standard library: ed25519, base64url, JSON.
package signing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Header is the JOSE header for our manifests (§5.8).
type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
	Ver int    `json:"ver"`
}

// Sign returns a JWS-compact-serialised manifest:
//
//	BASE64URL(UTF8(header)) "." BASE64URL(JCS(payload)) "." BASE64URL(sig)
//
// `payload` MAY be any JSON-encodable value; it will be JCS-canonicalised
// before signing. `kid` MUST resolve to the public key in the verifier's
// trust store.
func Sign(priv ed25519.PrivateKey, kid string, payload any) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", errors.New("invalid ed25519 private key")
	}
	hdr := Header{Alg: "EdDSA", Typ: "otap-desired-state+json", Kid: kid, Ver: 1}
	hdrBytes, err := json.Marshal(hdr)
	if err != nil {
		return "", fmt.Errorf("encode header: %w", err)
	}
	payloadJSON, err := JCS(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize payload: %w", err)
	}
	signing := signingInput(hdrBytes, payloadJSON)
	sig := ed25519.Sign(priv, signing)
	return string(signing) + "." + string(b64(sig)), nil
}

// SigningInput returns the input that the Ed25519 signer signs over
// (for cross-checks against the Rust verifier).
func SigningInput(hdr, payloadJCS []byte) []byte {
	return signingInput(hdr, payloadJCS)
}

func signingInput(hdr, payloadJCS []byte) []byte {
	out := make([]byte, 0, 64+len(hdr)+len(payloadJCS))
	out = append(out, b64(hdr)...)
	out = append(out, '.')
	out = append(out, b64(payloadJCS)...)
	return out
}

func b64(in []byte) []byte {
	enc := base64.RawURLEncoding
	out := make([]byte, enc.EncodedLen(len(in)))
	enc.Encode(out, in)
	return out
}

// --- JCS (RFC 8785) — minimal implementation tailored to our manifests ---

// JCS canonicalises an arbitrary JSON value per RFC 8785:
//   - object keys sorted lexicographically by UTF-16 code units (ASCII for our
//     schema, equivalent to Go string-compare),
//   - no insignificant whitespace,
//   - numbers serialised in shortest round-trip form (covered by ECMA-262 §7.1.12.1
//     for integers, which is all we use; floats in this codebase round-trip
//     through %g).
//
// Manifests in this project use only strings, ints, bools, arrays, and objects;
// floats are not part of the schema.
func JCS(v any) ([]byte, error) {
	// Stage 1: encode to JSON, decode into a generic value to drop any
	// formatting choices the original encoder made.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jcsEncode(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jcsEncode(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return encodeJSONString(buf, x)
	case json.Number:
		// Integers come through as decimal digits; pass through.
		// (This codebase doesn't use floats in canonicalised payloads.)
		s := x.String()
		if _, err := strconv.ParseInt(s, 10, 64); err == nil {
			buf.WriteString(s)
			return nil
		}
		// Fall back to canonical float form via ECMA-262 round trip.
		f, err := x.Float64()
		if err != nil {
			return err
		}
		buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := jcsEncode(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys) // ASCII keys → equivalent to UTF-16 codepoint sort
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeJSONString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := jcsEncode(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("jcs: unsupported type %T", v)
	}
	return nil
}

func encodeJSONString(buf *bytes.Buffer, s string) error {
	// Use a json.Encoder with HTML escaping disabled so that '<', '>', '&'
	// are emitted as literal characters (RFC 8785 §3.2.2.2 / RFC 8259 §7),
	// matching the Rust verifier's output and avoiding cross-language
	// canonicalisation drift. json.Encoder also appends a trailing newline
	// which we strip.
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	out := tmp.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	buf.Write(out)
	return nil
}

// SplitCompact splits "h.p.s" returning raw (decoded) header, payload, sig.
func SplitCompact(jws string) (hdr, payload, sig []byte, err error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, nil, nil, errors.New("not a JWS compact serialisation")
	}
	if hdr, err = base64.RawURLEncoding.DecodeString(parts[0]); err != nil {
		return nil, nil, nil, fmt.Errorf("decode header: %w", err)
	}
	if payload, err = base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		return nil, nil, nil, fmt.Errorf("decode payload: %w", err)
	}
	if sig, err = base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
		return nil, nil, nil, fmt.Errorf("decode signature: %w", err)
	}
	return
}

// CompactParts returns the encoded parts of a JWS compact serialisation
// without decoding (useful when re-running the verifier's signing-input check).
func CompactParts(jws string) (encHdr, encPayload, encSig string, err error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return "", "", "", errors.New("not a JWS compact serialisation")
	}
	return parts[0], parts[1], parts[2], nil
}
