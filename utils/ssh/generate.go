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

	"code.google.com/p/go.crypto/ssh"
)

// KeyBits is used to determine the number of bits to use for the RSA keys
// created using the GenerateKey function.
var KeyBits = 2048

// GenerateKey makes a 2048 bit RSA no-passphrase SSH capable key.  The bit
// size is actually controlled by the KeyBits var. The private key returned is
// encoded to ASCII using the PKCS1 encoding.  The public key is suitable to
// be added into an authorized_keys file, and has the comment passed in as the
// comment part of the key.
func GenerateKey(comment string) (private, public string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return "", "", err
	}

	identity := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})

	signer, err := ssh.ParsePrivateKey(identity)
	if err != nil {
		return "", "", fmt.Errorf("failed to load key: %v", err)
	}

	auth_key := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	// Strip off the trailing new line so we can add a comment.
	auth_key = strings.TrimSpace(auth_key)
	public = fmt.Sprintf("%s %s\n", auth_key, comment)

	return string(identity), public, nil
}
