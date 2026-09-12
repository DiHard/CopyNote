package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// publicKeyBase64 is the ed25519 key that release binaries are signed with.
// The private half never enters the repository: tools/signrelease keeps it
// in %USERPROFILE%\.copynote-release\signing.key (or COPYNOTE_SIGNING_KEY in
// CI). Rotating the key means shipping a release that carries the new public
// key while still being signed with the old one — otherwise installations
// in the field cannot verify anything signed afterwards.
const publicKeyBase64 = "mCwLtxtUTs9NsSxBhXNokwqg1H0L+2wHCH1KcskVvAk="

// PublicKey is the decoded release signing key. Empty when the constant
// above is not a valid key, which disables self-update (checks still run).
var PublicKey = decodePublicKey(publicKeyBase64)

func decodePublicKey(encoded string) ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(raw)
}

// signedMessage binds the binary to its release version so a signature
// made for one release cannot be replayed under a newer tag (downgrade).
func signedMessage(version string, data []byte) []byte {
	prefix := "copynote release " + strings.TrimPrefix(version, "v") + "\n"
	msg := make([]byte, 0, len(prefix)+len(data))
	msg = append(msg, prefix...)
	return append(msg, data...)
}

// Sign produces the detached signature for a release binary. Used by
// tools/signrelease; kept here so both sides share one message format.
func Sign(priv ed25519.PrivateKey, version string, data []byte) []byte {
	return ed25519.Sign(priv, signedMessage(version, data))
}

// EncodeSignature renders a signature as the .sig asset content: one line
// of standard base64.
func EncodeSignature(sig []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(sig) + "\n")
}

// DecodeSignature parses .sig asset content produced by EncodeSignature.
func DecodeSignature(content []byte) ([]byte, error) {
	text := strings.TrimSpace(string(content))
	if text == "" || len(text) > 128 || strings.ContainsAny(text, " \t\r\n") {
		return nil, errors.New("signature file is malformed")
	}
	sig, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("signature file is malformed: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature has %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}

// Verify checks data against a detached signature for the given version.
func Verify(pub ed25519.PublicKey, version string, data, sig []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("no release signing key is embedded in this build")
	}
	if !ed25519.Verify(pub, signedMessage(version, data), sig) {
		return errors.New("signature does not match the downloaded file")
	}
	return nil
}

// VerifyFile is Verify for a file on disk, using the embedded key.
func VerifyFile(path, version string, sig []byte) error {
	return verifyFile(PublicKey, path, version, sig)
}

func verifyFile(pub ed25519.PublicKey, path, version string, sig []byte) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read downloaded file: %w", err)
	}
	return Verify(pub, version, data, sig)
}
