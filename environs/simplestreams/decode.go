// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

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

	_, err = openpgp.CheckDetachedSignature(keyring, bytes.NewBuffer(b.Bytes), b.ArmoredSignature.Body)
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
