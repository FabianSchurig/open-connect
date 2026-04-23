// `oc-sign` — release-manager / test fixture tool that signs a JWS-compact
// manifest using an Ed25519 private key in PEM form.
//
//	oc-sign --key path/to/release-mgr.pem --kid release-mgr-2026-04 \
//	        --in manifest.json --out manifest.jws
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/signing"
)

func main() {
	keyPath := flag.String("key", "", "path to Ed25519 PKCS#8 PEM private key (required)")
	kid := flag.String("kid", "", "JOSE kid header value (required)")
	in := flag.String("in", "", "path to manifest JSON (required)")
	out := flag.String("out", "-", "output path; - for stdout")
	flag.Parse()

	if *keyPath == "" || *kid == "" || *in == "" {
		fmt.Fprintln(os.Stderr, "usage: oc-sign --key <pem> --kid <kid> --in <manifest.json> [--out <file>]")
		os.Exit(2)
	}
	priv, err := loadPrivateKey(*keyPath)
	if err != nil {
		die(err)
	}
	body, err := os.ReadFile(*in)
	if err != nil {
		die(err)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		die(fmt.Errorf("manifest JSON: %w", err))
	}
	jws, err := signing.Sign(priv, *kid, payload)
	if err != nil {
		die(err)
	}
	if *out == "-" {
		fmt.Println(jws)
		return
	}
	if err := os.WriteFile(*out, []byte(jws), 0o644); err != nil {
		die(err)
	}
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("not a PEM file")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8: %w", err)
	}
	pk, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 key (got %T)", key)
	}
	return pk, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
