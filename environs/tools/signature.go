// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/environs/simplestreams"
)

// VersionHash contains the SHA256 of one jujud version.
type VersionHash struct {
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256"`
}

// SignedVersions stores the content of a jujud signature file.
type SignedVersions struct {
	Versions []VersionHash `yaml:"versions"`
}

// ParseSignedVersions checks the signature of the data passed in and
// returns the parsed version data.
func ParseSignedVersions(r io.Reader, armoredPublicKey string) (*SignedVersions, error) {
	data, err := simplestreams.DecodeCheckSignature(r, armoredPublicKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results SignedVersions
	err = yaml.Unmarshal(data, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &results, nil
}

// VersionsMatching returns all version numbers for which the SHA256
// matches the content of the reader passed in.
func (v *SignedVersions) VersionsMatching(r io.Reader) ([]string, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return v.VersionsMatchingHash(hex.EncodeToString(hash.Sum(nil))), nil
}

// VersionsMatchingHash returns all version numbers for which the SHA256
// matches the hash passed in.
func (v *SignedVersions) VersionsMatchingHash(h string) []string {
	var results []string
	for i := range v.Versions {
		if v.Versions[i].SHA256 == h {
			results = append(results, v.Versions[i].Version)
		}
	}
	return results
}
