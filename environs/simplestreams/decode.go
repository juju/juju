// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"
)

// PGPPGPSignatureCheckFn can be overridden by tests to allow signatures from
// non-trusted sources to be verified.
var PGPSignatureCheckFn = func(keyring openpgp.KeyRing, signed, signature io.Reader) (*openpgp.Entity, error) {
	return openpgp.CheckDetachedSignature(keyring, signed, signature)
}

// DecodeCheckSignature parses the inline signed PGP text, checks the signature,
// and returns plain text if the signature matches.
func DecodeCheckSignature(r io.Reader, armoredPublicKey string) ([]byte, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b, _ := clearsign.Decode(data)
	if b == nil {
		return nil, &NotPGPSignedError{}
	}
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(armoredPublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	_, err = PGPSignatureCheckFn(keyring, bytes.NewBuffer(b.Bytes), b.ArmoredSignature.Body)
	if err != nil {
		return nil, err
	}
	return b.Plaintext, nil
}

// NotPGPSignedError is used when PGP text does not contain an inline signature.
type NotPGPSignedError struct{}

func (*NotPGPSignedError) Error() string {
	return "no PGP signature embedded in plain text data"
}
