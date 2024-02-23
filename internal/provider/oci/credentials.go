// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
)

const (
	// DefaultConfigFolder is the default folder in which the OCI cli will
	// store its config files and keys
	DefaultConfigFolder = ".oci"

	// FallbackConfigFolder is the fallback config folder. Users that installed
	// an earlier version of the oracle CLI tool will have this folder instead of
	// ~/.oci
	FallbackConfigFolder = ".oraclebmc"
)

func ociConfigFile() (string, error) {
	cfg_file := filepath.Join(utils.Home(), DefaultConfigFolder, "config")
	_, err := os.Stat(cfg_file)
	if err != nil {
		if os.IsNotExist(err) {
			// Check fall back
			cfg_file = filepath.Join(utils.Home(), FallbackConfigFolder, "config")
			if _, err := os.Stat(cfg_file); err != nil {
				return "", errors.Trace(err)
			}
		} else {
			return "", errors.Trace(err)
		}
	}
	return cfg_file, nil
}
