// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/juju/errors"
	ociCommon "github.com/oracle/oci-go-sdk/v65/common"
)

type JujuConfigProvider struct {
	Key         []byte
	Fingerprint string
	Passphrase  string
	Tenancy     string
	User        string
	OCIRegion   string
}

func ValidateKey(key []byte, passphrase string) error {
	keyBlock, _ := pem.Decode(key)
	if keyBlock == nil {
		return errors.Errorf("invalid private key")
	}

	if x509.IsEncryptedPEMBlock(keyBlock) {
		if _, err := x509.DecryptPEMBlock(keyBlock, []byte(passphrase)); err != nil {
			return errors.Annotatef(err, "decrypting private key")
		}
	}

	return nil
}

func (j JujuConfigProvider) TenancyOCID() (string, error) {
	if j.Tenancy == "" {
		return "", errors.Errorf("tenancyOCID is not set")
	}
	return j.Tenancy, nil
}

func (j JujuConfigProvider) UserOCID() (string, error) {
	if j.User == "" {
		return "", errors.Errorf("userOCID is not set")
	}
	return j.User, nil
}

func (j JujuConfigProvider) KeyFingerprint() (string, error) {
	if j.Fingerprint == "" {
		return "", errors.Errorf("Fingerprint is not set")
	}
	return j.Fingerprint, nil
}

func (j JujuConfigProvider) Region() (string, error) {
	if j.OCIRegion == "" {
		return "", errors.Errorf("Region is not set")
	}
	return j.OCIRegion, nil
}

func (j JujuConfigProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	if j.Key == nil {
		return nil, errors.Errorf("private key is not set")
	}

	key, err := ociCommon.PrivateKeyFromBytes(
		j.Key, &j.Passphrase)
	return key, err
}

func (j JujuConfigProvider) KeyID() (string, error) {
	if err := j.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s", j.Tenancy, j.User, j.Fingerprint), nil
}

func (j JujuConfigProvider) AuthType() (ociCommon.AuthConfig, error) {
	return ociCommon.AuthConfig{}, errors.NotImplementedf("AuthType")
}

func (j JujuConfigProvider) Validate() error {
	if j.Tenancy == "" || j.User == "" || j.Fingerprint == "" {
		return errors.Errorf("config provider is not properly initialized")
	}
	if err := ValidateKey(j.Key, j.Passphrase); err != nil {
		return errors.Trace(err)
	}
	return nil
}
