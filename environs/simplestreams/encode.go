// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"bytes"
	"io"
	"io/ioutil"

	"code.google.com/p/go.crypto/openpgp"
	"code.google.com/p/go.crypto/openpgp/clearsign"
)

// Encode signs the data returned by the reader and returns an inline signed copy.
func Encode(r io.Reader, armoredPrivateKey, passphrase string) ([]byte, error) {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(armoredPrivateKey))
	if err != nil {
		return nil, err
	}

	privateKey := keyring[0].PrivateKey
	if privateKey.Encrypted {
		err = privateKey.Decrypt([]byte(passphrase))
		if err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	plaintext, err := clearsign.Encode(&buf, privateKey, nil)
	if err != nil {
		return nil, err
	}
	metadata, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	dataToSign := metadata
	if dataToSign[0] == '\n' {
		dataToSign = dataToSign[1:]
	}
	_, err = plaintext.Write([]byte(dataToSign))
	if err != nil {
		return nil, err
	}
	err = plaintext.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
