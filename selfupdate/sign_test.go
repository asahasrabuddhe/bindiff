package selfupdate

import (
	"errors"
	"testing"
)

func TestSignVerify(t *testing.T) {
	pubText, privText, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey(pubText + "\n")
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ParsePrivateKey(privText)
	if err != nil {
		t.Fatal(err)
	}

	manifest := []byte(`{"schema":1}`)
	sig := Sign(manifest, priv)
	if err := Verify(manifest, sig, pub); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	otherPub, _, _ := GenerateKey()
	other, _ := ParsePublicKey(otherPub)

	for name, tc := range map[string]struct {
		manifest, sig []byte
		key           []byte
	}{
		"tampered manifest": {[]byte(`{"schema":2}`), sig, pub},
		"missing signature": {manifest, nil, pub},
		"garbage signature": {manifest, []byte("not base64!"), pub},
		"wrong key":         {manifest, sig, other},
	} {
		t.Run(name, func(t *testing.T) {
			if err := Verify(tc.manifest, tc.sig, tc.key); !errors.Is(err, ErrSignature) {
				t.Fatalf("Verify = %v, want ErrSignature", err)
			}
		})
	}
}

func TestParseKeyErrors(t *testing.T) {
	pub, priv, _ := GenerateKey()
	if _, err := ParsePublicKey(priv); err == nil {
		t.Error("ParsePublicKey accepted a private key")
	}
	if _, err := ParsePrivateKey(pub); err == nil {
		t.Error("ParsePrivateKey accepted a public key")
	}
	if _, err := ParsePublicKey("%%%"); err == nil {
		t.Error("ParsePublicKey accepted garbage")
	}
}
