// Command signrelease manages the ed25519 key that signs CopyNote releases.
//
//	go run ./tools/signrelease -generate            # create the key pair (once)
//	go run ./tools/signrelease copynote.exe         # write copynote.exe.sig
//	go run ./tools/signrelease -verify copynote.exe # check copynote.exe.sig
//
// The private key lives outside the repository: by default in
// %USERPROFILE%\.copynote-release\signing.key, or in the COPYNOTE_SIGNING_KEY
// environment variable (base64 seed) for CI. Its public half is embedded in
// internal/updater/signature.go and the updater refuses any binary not
// signed with it, so keep the private key backed up — losing it means one
// more manual release that carries a new public key.
//
// The signature covers the release version (from internal/version, or
// -version for ldflags builds) together with the file, so a signature for
// one release cannot be replayed under another tag.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"copynote/internal/updater"
	"copynote/internal/version"
)

func main() {
	generate := flag.Bool("generate", false, "create a new key pair and print the public key")
	verify := flag.Bool("verify", false, "check <binary>.sig against the embedded public key instead of signing")
	keyPath := flag.String("key", defaultKeyPath(), "private key file (base64 ed25519 seed)")
	ver := flag.String("version", version.Version, "release version the signature is bound to")
	flag.Parse()

	if err := run(*generate, *verify, *keyPath, *ver, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "signrelease:", err)
		os.Exit(1)
	}
}

func defaultKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "signing.key"
	}
	return filepath.Join(home, ".copynote-release", "signing.key")
}

func run(generate, verify bool, keyPath, ver string, args []string) error {
	switch {
	case generate:
		if len(args) != 0 {
			return errors.New("-generate takes no file argument")
		}
		return generateKey(keyPath)
	case len(args) != 1:
		return errors.New("usage: signrelease [-verify] [-key file] [-version X.Y.Z] <binary>")
	case verify:
		return verifyFile(args[0], ver)
	default:
		return signFile(keyPath, args[0], ver)
	}
}

func generateKey(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; move it away first if a new key is really wanted", path)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	if err := os.WriteFile(path, []byte(seed+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("private key written to %s — back it up, never commit it\n", path)
	fmt.Printf("public key for internal/updater/signature.go:\n%s\n", base64.StdEncoding.EncodeToString(pub))
	return nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	encoded, source := os.Getenv("COPYNOTE_SIGNING_KEY"), "COPYNOTE_SIGNING_KEY"
	if encoded == "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private key: %w (run with -generate first)", err)
		}
		encoded, source = string(raw), path
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s does not contain a base64 ed25519 seed", source)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func signFile(keyPath, file, ver string) error {
	priv, err := loadPrivateKey(keyPath)
	if err != nil {
		return err
	}
	if !priv.Public().(ed25519.PublicKey).Equal(updater.PublicKey) {
		return errors.New("private key does not match the public key embedded in internal/updater/signature.go")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sig := updater.Sign(priv, ver, data)
	if err := os.WriteFile(file+".sig", updater.EncodeSignature(sig), 0o644); err != nil {
		return err
	}
	fmt.Printf("signed %s for version %s -> %s.sig\n", file, ver, file)
	return nil
}

func verifyFile(file, ver string) error {
	content, err := os.ReadFile(file + ".sig")
	if err != nil {
		return err
	}
	sig, err := updater.DecodeSignature(content)
	if err != nil {
		return err
	}
	if err := updater.VerifyFile(file, ver, sig); err != nil {
		return err
	}
	fmt.Printf("%s.sig is valid for %s, version %s\n", file, file, ver)
	return nil
}
