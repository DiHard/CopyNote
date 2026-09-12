package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func testKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestSignatureRoundTrip(t *testing.T) {
	pub, priv := testKeys(t)
	data := []byte("release binary bytes")
	sig, err := DecodeSignature(EncodeSignature(Sign(priv, "1.2.3", data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, "1.2.3", data, sig); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := Verify(pub, "v1.2.3", data, sig); err != nil {
		t.Fatalf("leading v must not matter: %v", err)
	}

	other, _ := testKeys(t)
	tampered := append([]byte(nil), data...)
	tampered[0]++
	for name, err := range map[string]error{
		"tampered file":     Verify(pub, "1.2.3", tampered, sig),
		"other version":     Verify(pub, "1.2.4", data, sig),
		"other key":         Verify(other, "1.2.3", data, sig),
		"no key":            Verify(nil, "1.2.3", data, sig),
		"truncated version": Verify(pub, "1.2", data, sig),
	} {
		if err == nil {
			t.Errorf("%s: signature accepted", name)
		}
	}
}

func TestDecodeSignatureRejectsMalformedContent(t *testing.T) {
	for name, content := range map[string]string{
		"empty":        "",
		"not base64":   "not base64!!",
		"short":        "AAAA",
		"too long":     strings.Repeat("A", 200),
		"two lines":    "QUJD\nREVG",
		"inner spaces": "QUJD REVG",
	} {
		if _, err := DecodeSignature([]byte(content)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEmbeddedPublicKeyIsPresent(t *testing.T) {
	if len(PublicKey) != ed25519.PublicKeySize {
		t.Fatal("no release signing key is embedded; self-update would be disabled in this build")
	}
}
