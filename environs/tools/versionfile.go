// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// VersionHash contains the SHA256 of one jujud version.
type VersionHash struct {
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256"`
}

// Versions stores the content of a jujud signature file.
type Versions struct {
	Versions []VersionHash `yaml:"versions"`
}

// ParseVersions constructs a versions object from a reader..
func ParseVersions(r io.Reader) (*Versions, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var results Versions
	err = yaml.Unmarshal(data, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &results, nil
}

// VersionsMatching returns all version numbers for which the SHA256
// matches the content of the reader passed in.
func (v *Versions) VersionsMatching(r io.Reader) ([]string, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return v.versionsMatchingHash(hex.EncodeToString(hash.Sum(nil))), nil
}

// versionsMatchingHash returns all version numbers for which the SHA256
// matches the hash passed in.
func (v *Versions) versionsMatchingHash(h string) []string {
	logger.Debugf(context.Background(), "looking for sha256 %s", h)
	var results []string
	for i := range v.Versions {
		if v.Versions[i].SHA256 == h {
			results = append(results, v.Versions[i].Version)
		}
	}
	return results
}
