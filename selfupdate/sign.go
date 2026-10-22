package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrSignature is returned when a manifest signature is missing or invalid.
var ErrSignature = errors.New("selfupdate: invalid manifest signature")

// GenerateKey returns a new ed25519 key pair, base64 encoded. Keep the
// private key secret, for example in a CI secret, and embed the public key
// in the program that updates itself.
func GenerateKey() (publicKey, privateKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// ParsePublicKey decodes a base64 public key from GenerateKey.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace([]byte(s))))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("selfupdate: invalid public key")
	}
	return ed25519.PublicKey(b), nil
}

// ParsePrivateKey decodes a base64 private key from GenerateKey.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace([]byte(s))))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("selfupdate: invalid private key")
	}
	return ed25519.PrivateKey(b), nil
}

// Sign returns the contents of a signature file for manifest.
func Sign(manifest []byte, key ed25519.PrivateKey) []byte {
	sig := ed25519.Sign(key, manifest)
	return []byte(base64.StdEncoding.EncodeToString(sig) + "\n")
}

// Verify checks a signature file made by Sign.
func Verify(manifest, signature []byte, key ed25519.PublicKey) error {
	if len(signature) == 0 {
		return fmt.Errorf("%w: no signature published", ErrSignature)
	}

	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(signature)))
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(key, manifest, sig) {
		return ErrSignature
	}
	return nil
}
