// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// hash returns a hash of the yaml serialized settings.
// If the settings are not able to be serialized an error is returned.
func hash(settings map[string]interface{}) (string, error) {
	bytes, err := yaml.Marshal(settings)
	if err != nil {
		return "", errors.Trace(err)
	}
	hash := sha256.New()
	_, err = hash.Write(bytes)
	if err != nil {
		return "", errors.Trace(err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
