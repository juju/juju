// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"bytes"
	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
	"fmt"
	"io"
	"io/ioutil"
)

// simpleStreamSigningKey is the public key required to
// authenticate the simple streams data on http://cloud-images.ubuntu.com.
// Declared as a var so it can be overidden for testing.
// See http://bazaar.launchpad.net/~smoser/simplestreams/trunk/view/head:/examples/keys/cloud-images.pub
var simpleStreamSigningKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.12 (GNU/Linux)

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
=+MFT
-----END PGP PUBLIC KEY BLOCK-----
`

// DecodeCheckSignature parses the inline signed PGP text, checks the signature,
// and returns plain text if the signature matches.
func DecodeCheckSignature(r io.Reader) ([]byte, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b, _ := clearsign.Decode(data)
	if b == nil {
		return nil, &NotPGPSignedError{}
	}
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(simpleStreamSigningKey))
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
