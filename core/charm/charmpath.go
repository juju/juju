// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// NewCharmAtPath returns the charm represented by this path,
// and a URL that describes it.
// Deploying from a directory is no longer supported.
func NewCharmAtPath(path string) (charm.Charm, *charm.URL, error) {
	if path == "" {
		return nil, nil, errors.Errorf("empty charm path %w", coreerrors.NotValid)
	}

	if info, err := os.Stat(path); isNotExistsError(err) {
		return nil, nil, os.ErrNotExist
	} else if err == nil && !isValidCharmOrBundlePath(path) {
		return nil, nil, InvalidPath(path)
	} else if info.IsDir() {
		return nil, nil, errors.Errorf("deploying from directory %w", coreerrors.NotSupported)
	}

	ch, err := charm.ReadCharmArchive(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, nil, CharmNotFound(path)
		}
		return nil, nil, err
	}
	if err := charm.CheckMeta(ch); err != nil {
		return nil, nil, errors.Capture(err)
	}

	url := &charm.URL{
		Schema:   "local",
		Name:     ch.Meta().Name,
		Revision: ch.Revision(),
	}
	return ch, url, nil
}

func isNotExistsError(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	// On Windows, we get a path error due to a GetFileAttributesEx syscall.
	// To avoid being too proscriptive, we'll simply check for the error
	// type and not any content.
	if _, ok := err.(*os.PathError); ok {
		return true
	}
	return false
}

func isValidCharmOrBundlePath(path string) bool {
	//Exclude relative paths.
	return strings.HasPrefix(path, ".") || filepath.IsAbs(path)
}

// CharmNotFound returns an error indicating that the
// charm at the specified URL does not exist.
func CharmNotFound(url string) error {
	return errors.New("charm not found: " + url).Add(coreerrors.NotFound)
}

// InvalidPath returns an invalidPathError.
func InvalidPath(path string) error {
	return &invalidPathError{path}
}

// invalidPathError represents an error indicating that the requested
// charm or bundle path is not valid as a charm or bundle path.
type invalidPathError struct {
	path string
}

func (e *invalidPathError) Error() string {
	return fmt.Sprintf("path %q can not be a relative path", e.path)
}

func IsInvalidPathError(err error) bool {
	_, ok := err.(*invalidPathError)
	return ok
}

// unsupportedBaseError represents an error indicating that the requested base
// is not supported by the charm.
type unsupportedBaseError struct {
	requestedBase  base.Base
	supportedBases []base.Base
}

func (e *unsupportedBaseError) Error() string {
	return fmt.Sprintf(
		"base %q not supported by charm, the charm supported bases are: %s",
		e.requestedBase.DisplayString(), printBases(e.supportedBases),
	)
}

// NewUnsupportedBaseError returns an error indicating that the requested series
// is not supported by a charm.
func NewUnsupportedBaseError(requestedBase base.Base, supportedBases []base.Base) error {
	return &unsupportedBaseError{requestedBase, supportedBases}
}

// IsUnsupportedBaseError returns true if err is an UnsupportedSeriesError.
func IsUnsupportedBaseError(err error) bool {
	_, ok := err.(*unsupportedBaseError)
	return ok
}
