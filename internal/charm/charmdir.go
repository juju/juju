// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/errors"
)

// ReadCharmDirMetadata reads and parses the metadata file for a charm directory.
func ReadCharmDirMetadata(path string) (*Meta, error) {
	reader, err := os.Open(filepath.Join(path, "metadata.yaml"))
	if _, ok := err.(*os.PathError); ok {
		return nil, errors.Errorf("metadata.yaml: %w", FileNotFound)
	} else if err != nil {
		return nil, errors.Errorf(`reading "metadata.yaml" file: %w`, err)
	}
	defer reader.Close()

	meta, err := ReadMeta(reader)
	if err != nil {
		return nil, errors.Errorf(`parsing "metadata.yaml" file: %w`, err)
	}

	return meta, nil
}

// ReadCharmDirManifest reads and parses the manifest file for a charm directory.
func ReadCharmDirManifest(path string) (*Manifest, error) {
	reader, err := os.Open(filepath.Join(path, "manifest.yaml"))
	if _, ok := err.(*os.PathError); ok {
		return nil, errors.Errorf("manifest.yaml: %w", FileNotFound)
	} else if err != nil {
		return nil, errors.Errorf(`reading "manifest.yaml" file: %w`, err)
	}
	defer reader.Close()

	manifest, err := ReadManifest(reader)
	if err != nil {
		return nil, errors.Errorf(`parsing "manifest.yaml" file: %w`, err)
	}

	return manifest, nil
}

// ReadCharmDirConfig reads and parses the config file for a charm directory.
func ReadCharmDirConfig(path string) (*ConfigSpec, error) {
	reader, err := os.Open(filepath.Join(path, "config.yaml"))
	if _, ok := err.(*os.PathError); ok {
		return nil, errors.Errorf("config.yaml: %w", FileNotFound)
	} else if err != nil {
		return nil, errors.Errorf(`reading "config.yaml" file: %w`, err)
	}
	defer reader.Close()

	config, err := ReadConfig(reader)
	if err != nil {
		return nil, errors.Errorf(`parsing "config.yaml" file: %w`, err)
	}

	return config, nil
}

// ReadCharmDirActions reads and parses the actions file for a charm directory.
func ReadCharmDirActions(charmName string, path string) (*Actions, error) {
	reader, err := os.Open(filepath.Join(path, "actions.yaml"))
	if _, ok := err.(*os.PathError); ok {
		return nil, errors.Errorf("actions.yaml: %w", FileNotFound)
	} else if err != nil {
		return nil, errors.Errorf(`reading "actions.yaml" file: %w`, err)
	}
	defer reader.Close()

	actions, err := ReadActionsYaml(charmName, reader)
	if err != nil {
		return nil, errors.Errorf(`parsing "actions.yaml" file: %w`, err)
	}
	return actions, nil
}

// ReadCharmDirRevision reads the revision file for a charm directory.
func ReadCharmDirRevision(path string) (int, error) {
	reader, err := os.Open(filepath.Join(path, "revision"))
	if _, ok := err.(*os.PathError); ok {
		return 0, errors.Errorf("revision: %w", FileNotFound)
	} else if err != nil {
		return 0, errors.Errorf(`reading "revision" file: %w`, err)
	}
	defer reader.Close()

	var revision int
	_, err = fmt.Fscan(reader, &revision)
	if err != nil {
		return 0, errors.Errorf(`parsing "revision" file: %w`, err)
	}

	return revision, nil
}

func ReadCharmDirLXDProfile(path string) (*LXDProfile, error) {
	reader, err := os.Open(filepath.Join(path, "lxd-profile.yaml"))
	if _, ok := err.(*os.PathError); ok {
		return nil, errors.Errorf("lxd-profile.yaml: %w", FileNotFound)
	} else if err != nil {
		return nil, errors.Errorf(`reading "lxd-profile.yaml" file: %w`, err)
	}
	defer reader.Close()

	lxdProfile, err := ReadLXDProfile(reader)
	if err != nil {
		return nil, errors.Errorf(`parsing "lxd-profile.yaml" file: %w`, err)
	}

	return lxdProfile, nil
}

func ReadCharmDirVersion(path string) (string, error) {
	reader, err := os.Open(filepath.Join(path, "version"))
	if _, ok := err.(*os.PathError); ok {
		return "", errors.Errorf("version: %w", FileNotFound)
	} else if err != nil {
		return "", errors.Errorf(`reading "version" file: %w`, err)
	}
	defer reader.Close()

	return readVersion(reader)
}
