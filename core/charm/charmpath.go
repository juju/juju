// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/charm"

	"github.com/juju/juju/core/base"
)

// NewCharmAtPath returns the charm represented by this path,
// and a URL that describes it. If the base is empty,
// the charm's default base is used, if any.
// Otherwise, the base is validated against those the
// charm declares it supports.
func NewCharmAtPath(path string, b base.Base) (charm.Charm, *charm.URL, error) {
	return NewCharmAtPathForceBase(path, b, false)
}

// NewCharmAtPathForceBase returns the charm represented by this path,
// and a URL that describes it. If the base is empty,
// the charm's default base is used, if any.
// Otherwise, the base is validated against those the
// charm declares it supports. If force is true, then any
// base validation errors are ignored and the requested
// base is used regardless.
func NewCharmAtPathForceBase(path string, b base.Base, force bool) (charm.Charm, *charm.URL, error) {
	if path == "" {
		return nil, nil, errors.New("empty charm path")
	}
	_, err := os.Stat(path)
	if isNotExistsError(err) {
		return nil, nil, os.ErrNotExist
	} else if err == nil && !isValidCharmOrBundlePath(path) {
		return nil, nil, InvalidPath(path)
	}
	ch, err := charm.ReadCharm(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, nil, CharmNotFound(path)
		}
		return nil, nil, err
	}
	name := ch.Meta().Name

	baseToUse, err := charmBase(b, force, ch)
	if err != nil {
		return nil, nil, err
	}
	seriesToUse, err := base.GetSeriesFromBase(baseToUse)
	if err != nil {
		return nil, nil, err
	}

	url := &charm.URL{
		Schema:   "local",
		Name:     name,
		Series:   seriesToUse,
		Revision: ch.Revision(),
	}
	return ch, url, nil
}

func charmBase(b base.Base, force bool, cm charm.CharmMeta) (base.Base, error) {
	if force && !b.Empty() {
		return b, nil
	}
	computedBases, err := ComputedBases(cm)
	logger.Tracef("base %q, %v", b, computedBases)
	if err != nil {
		return base.Base{}, err
	}
	if len(computedBases) == 0 {
		if b.Empty() {
			return base.Base{}, errors.New("base not specified and charm does not define any")
		} else {
			// old style charm with no base in metadata.
			return b, nil
		}
	}
	if !b.Empty() {
		for _, computedBase := range computedBases {
			if b == computedBase {
				return b, nil
			}
		}
		return base.Base{}, NewUnsupportedBaseError(b, computedBases)
	}
	if len(computedBases) > 0 {
		return computedBases[0], nil
	}
	return base.Base{}, errors.Errorf("base not specified and charm does not define any")
}

func isNotExistsError(err error) bool {
	if os.IsNotExist(errors.Cause(err)) {
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
	return errors.NewNotFound(nil, "charm not found: "+url)
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

// UnsupportedSeriesError represents an error indicating that the requested series
// is not supported by the charm.
type unsupportedSeriesError struct {
	requestedSeries string
	supportedSeries []string
}

func (e *unsupportedSeriesError) Error() string {
	return fmt.Sprintf(
		"series %q not supported by charm, the charm supported series are: %s",
		e.requestedSeries, strings.Join(e.supportedSeries, ","),
	)
}

// NewUnsupportedSeriesError returns an error indicating that the requested series
// is not supported by a charm.
func NewUnsupportedSeriesError(requestedSeries string, supportedSeries []string) error {
	return &unsupportedSeriesError{requestedSeries, supportedSeries}
}

// IsUnsupportedSeriesError returns true if err is an UnsupportedSeriesError.
func IsUnsupportedSeriesError(err error) bool {
	_, ok := err.(*unsupportedSeriesError)
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
