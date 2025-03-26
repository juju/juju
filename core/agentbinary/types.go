// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"fmt"

	"github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/version"
)

// Version represents the version of an agent binary. [Version] was created so
// that Juju can move itself off of [version.Binary] as this contains the
// release field that we no longer want.
type Version struct {
	// Number is the juju version number.
	Number version.Number

	// Arch is the architecture of the agent binary.
	Arch arch.Arch
}

// Validate checks that the version is valid by checking that the version
// number is a non zero value and the architecture is supported. If these checks
// aren't satisfied an error satisfying [coreerrors.NotValid] will be returned.
func (v Version) Validate() error {
	if version.Zero == v.Number {
		return errors.Errorf(
			"version number cannot be zero value",
		).Add(coreerrors.NotValid)
	}

	if !arch.IsSupportedArch(v.Arch) {
		return errors.Errorf(
			"unsupported architecture %q",
			v.Arch,
		).Add(coreerrors.NotValid)
	}
	return nil
}

// String returns the version as a string.
// It formats the version number and architecture in the <version>-<arch> format.
func (v Version) String() string {
	return fmt.Sprintf("%s-%s", v.Number.String(), v.Arch)
}
