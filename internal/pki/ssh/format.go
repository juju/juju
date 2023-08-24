// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
	cryptossh "golang.org/x/crypto/ssh"
)

func encodePrivateKey(pk crypto.PrivateKey) (string, error) {
	pkcs, err := x509.MarshalPKCS8PrivateKey(pk)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs,
	}
	return string(pem.EncodeToMemory(block)), nil
}

func encodePublicKey(pk crypto.PublicKey, comment string) (string, error) {
	sshPublicKey, err := cryptossh.NewPublicKey(pk)
	if err != nil {
		return "", errors.NewNotValid(err, "public key")
	}
	encoded := string(cryptossh.MarshalAuthorizedKey(sshPublicKey))
	if comment != "" {
		encoded = fmt.Sprintf("%s %s\n", strings.TrimRight(encoded, "\n"), comment)
	}
	return encoded, nil
}

type privateKey interface {
	Public() crypto.PublicKey
}

// FormatKey formats the crypto key into PKCS8 encoded private key and openssh public keyline.
func FormatKey(pk crypto.PrivateKey, comment string) (private string, public string, keyAlgorithm string, err error) {
	sshPrivateKey, _ := pk.(privateKey)
	if sshPrivateKey == nil {
		return "", "", "", errors.NotValidf("private key")
	}
	private, err = encodePrivateKey(sshPrivateKey)
	if err != nil {
		err = errors.Annotate(err, "cannot encode private key")
		return
	}
	public, err = encodePublicKey(sshPrivateKey.Public(), comment)
	if err != nil {
		err = errors.Annotate(err, "cannot encode public key")
		return
	}
	switch k := pk.(type) {
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256():
			keyAlgorithm = cryptossh.KeyAlgoECDSA256
		case elliptic.P384():
			keyAlgorithm = cryptossh.KeyAlgoECDSA384
		case elliptic.P521():
			keyAlgorithm = cryptossh.KeyAlgoECDSA521
		}
	case *ed25519.PrivateKey, ed25519.PrivateKey:
		keyAlgorithm = cryptossh.KeyAlgoED25519
	case *rsa.PrivateKey:
		keyAlgorithm = cryptossh.KeyAlgoRSA
	}
	if keyAlgorithm == "" {
		err = errors.Errorf("unknown private key type %v", reflect.TypeOf(pk))
	}
	return
}
