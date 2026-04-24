# `testdata/keys/`

This directory is intentionally empty.

**Private signing keys must NOT be committed to the repository**, even for
fixtures. Tests that need an Ed25519 keypair generate one in-process at
runtime — see, for example,
[`services/control-plane/internal/signing/jws_test.go`](../../services/control-plane/internal/signing/jws_test.go)
(uses `crypto/ed25519.GenerateKey`) and
[`crates/manifest-verifier/src/lib.rs`](../../crates/manifest-verifier/src/lib.rs)
(seeded `ed25519_dalek::SigningKey::from_bytes`).

For local development with the `oc-sign` CLI, generate a one-off keypair into
this directory (it is `.gitignore`d):

```bash
# PKCS#8 PEM private key.
openssl genpkey -algorithm ed25519 -out testdata/keys/release-mgr.priv.pem
# Matching SubjectPublicKeyInfo PEM public key.
openssl pkey -in testdata/keys/release-mgr.priv.pem -pubout \
    -out testdata/keys/release-mgr.pub.pem
```

Then sign a manifest with:

```bash
go run ./services/control-plane/cmd/oc-sign \
    --key testdata/keys/release-mgr.priv.pem \
    --kid release-mgr-2026-04 \
    --in  testdata/manifests/yocto-wic-v2.4.0.json \
    --out /tmp/manifest.jws
```
