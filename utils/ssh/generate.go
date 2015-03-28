// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"golang.org/x/crypto/ssh"
)

// rsaGenerateKey allows for tests to patch out rsa key generation
var rsaGenerateKey = rsa.GenerateKey

// KeyBits is used to determine the number of bits to use for the RSA keys
// created using the GenerateKey function.
var KeyBits = 2048

// GenerateKey makes a 2048 bit RSA no-passphrase SSH capable key.  The bit
// size is actually controlled by the KeyBits var. The private key returned is
// encoded to ASCII using the PKCS1 encoding.  The public key is suitable to
// be added into an authorized_keys file, and has the comment passed in as the
// comment part of the key.
func GenerateKey(comment string) (private, public string, err error) {
	key, err := rsaGenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	identity := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})

	public, err = PublicKey(identity, comment)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	return string(identity), public, nil
}

// PublicKey returns the public key for any private key. The public key is
// suitable to be added into an authorized_keys file, and has the comment
// passed in as the comment part of the key.
func PublicKey(privateKey []byte, comment string) (string, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return "", errors.Annotate(err, "failed to load key")
	}

	auth_key := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	// Strip off the trailing new line so we can add a comment.
	auth_key = strings.TrimSpace(auth_key)
	public := fmt.Sprintf("%s %s\n", auth_key, comment)

	return public, nil
}
