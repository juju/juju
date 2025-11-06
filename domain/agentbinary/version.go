// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// Version represents the complete version information for an agent binary.
type Version struct {
	// Architecture is the architecture of the agent binary.
	Architecture Architecture

	// Number is the juju version number.
	Number semversion.Number
}

// Validate checks that the version is valid by checking that the version
// number is a non zero value and the architecture is supported. If these checks
// aren't satisfied an error satisfying [coreerrors.NotValid] will be returned.
func (v Version) Validate() error {
	if semversion.Zero == v.Number {
		return errors.New("version number cannot be zero value").Add(
			coreerrors.NotValid,
		)
	}

	if !v.Architecture.IsValid() {
		return errors.New("unsupported architecture").Add(coreerrors.NotValid)
	}

	return nil
}
