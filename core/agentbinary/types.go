// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"fmt"

	"github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// AgentStream is an identifier representing the stream that should be used for
// agent binaries.
type AgentStream string

// Metadata describes the data around an available agent binary within the
// system.
type Metadata struct {
	// Version is the version of the agent binary blob.
	Version Version

	// SHA256 is the sha256 sum of the agent binary blob.
	SHA256 string

	// SHA384 string is the sha384 sum of the agent binary blob.
	SHA384 string

	// Size is the number of bytes for the agent binary blob.
	Size int64
}

// Version represents the version of an agent binary. [Version] was created so
// that Juju can move itself off of [version.Binary] as this contains the
// release field that we no longer want.
type Version struct {
	// Number is the juju version number.
	Number semversion.Number

	// Arch is the architecture of the agent binary.
	Arch arch.Arch
}

const (
	// AgentStreamZero represents the zero value AgentStream
	AgentStreamZero = AgentStream("")
	// AgentStreamReleased represents the released stream for agent binaries.
	AgentStreamReleased = AgentStream("released")
	// AgentStreamTesting represents the testing stream for agent binaries.
	AgentStreamTesting = AgentStream("testing")
	// AgentStreamProposed represents the proposed stream for agent binaries.
	AgentStreamProposed = AgentStream("proposed")
	// AgentStreamDevel represents the devel stream for agent binaries.
	AgentStreamDevel = AgentStream("devel")
)

// Validate checks that the version is valid by checking that the version
// number is a non zero value and the architecture is supported. If these checks
// aren't satisfied an error satisfying [coreerrors.NotValid] will be returned.
func (v Version) Validate() error {
	if semversion.Zero == v.Number {
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

// String returns the agent stream as a string. This  function implements the
// stringer interface.
func (a AgentStream) String() string {
	return string(a)
}

// String returns the version as a string.
// It formats the version number and architecture in the <version>-<arch> format.
func (v Version) String() string {
	return fmt.Sprintf("%s-%s", v.Number.String(), v.Arch)
}
