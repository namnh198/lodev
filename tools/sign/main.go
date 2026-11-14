// Command sign makes and uses the ed25519 key that signs a lodev release's checksums.txt.
//
//	go run ./tools/sign -genkey                                         # once: prints private and public key
//	go run ./tools/sign -key-env LODEV_SIGNING_KEY dist/checksums.txt   # writes dist/checksums.txt.sig
//
// The signature file is base64 of the raw 64-byte signature plus a newline; lodev self-upgrade
// verifies it against the public key built into internal/update.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
)

func genKey() (priv, pub string, err error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(sk), base64.StdEncoding.EncodeToString(pk), nil
}

// sign writes file+".sig" for file, signed with the base64 private key in environment variable keyEnv.
func sign(keyEnv, file string) error {
	b64 := os.Getenv(keyEnv)
	if b64 == "" {
		return fmt.Errorf("environment variable %s is not set", keyEnv)
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("%s is not a base64 ed25519 private key (make one with -genkey)", keyEnv)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n"
	return os.WriteFile(file+".sig", []byte(sig), 0o644)
}

func run() error {
	genkey := flag.Bool("genkey", false, "print a new private and public key")
	keyEnv := flag.String("key-env", "", "name of the environment variable holding the base64 private key")
	flag.Parse()
	if *genkey {
		priv, pub, err := genKey()
		if err != nil {
			return err
		}
		fmt.Println("private:", priv)
		fmt.Println("public:", pub)
		return nil
	}
	if *keyEnv == "" || flag.NArg() != 1 {
		return errors.New("usage: sign -genkey | sign -key-env NAME FILE")
	}
	return sign(*keyEnv, flag.Arg(0))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}
