// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transientfile

import (
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/errors"
)

// Create a transient file with the specified name inside transientDir. The
// function will attempt to create any missing folders leading up to the
// place where the transient file is to be created.
//
// For *nix targets, the caller is expected to provide a suitable transient
// directory (e.g. a tmpfs mount) that will be automatically purged after a
// reboot.
//
// For windows targets, any directory can be specified as transientDir but in
// order to ensure that the file will get removed, the process must be able to
// access the windows registry.
func Create(transientDir, name string) (*os.File, error) {
	if filepath.IsAbs(name) {
		return nil, errors.New("transient file name contains an absolute path")
	}

	transientFilePath := filepath.Join(transientDir, name)

	// Create any missing directories. The MkdirAll call below might fail
	// if the base directory does not exist and multiple agents attempt to
	// create it concurrently. To work around this potential race, we retry
	// the attempt a few times before bailing out with an error.
	//
	// TODO(achilleasa) this retry block is only needed until we complete
	// the unification of the machine/unit juju agents.
	baseDir := filepath.Dir(transientFilePath)
	for attempt := 0; ; attempt++ {
		if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
			if attempt == 10 {
				return nil, errors.Errorf("unable to create directory path for transient file %q: %w", transientFilePath, err)
			}
			continue
		}

		break
	}

	f, err := os.Create(transientFilePath)
	if err != nil {
		return nil, errors.Errorf("unable to create transient file %q: %w", transientFilePath, err)
	}

	// Invoke platform-specific code to ensure that the file is removed
	// after a reboot.
	if err = ensureDeleteAfterReboot(transientFilePath); err != nil {
		_ = f.Close()
		_ = os.Remove(transientFilePath)
		return nil, errors.Errorf("unable to schedule deletion of transient file %q after reboot: %w", transientFilePath, err)
	}

	return f, nil
}
