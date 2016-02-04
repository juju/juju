// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"
	"os"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/tools"
)

// BootstrapParams holds the parameters for bootstrapping an environment.
type BootstrapParams struct {
	// EnvironConstraints are merged with the bootstrap constraints
	// to choose the initial instance, and will be stored in the new
	// environment's state.
	EnvironConstraints constraints.Value

	// BootstrapConstraints, in conjunction with EnvironConstraints,
	// are used to choose the initial instance. BootstrapConstraints
	// will not be stored in state for the environment.
	BootstrapConstraints constraints.Value

	// Placement, if non-empty, holds an environment-specific placement
	// directive used to choose the initial instance.
	Placement string

	// AvailableTools is a collection of tools which the Bootstrap method
	// may use to decide which architecture/series to instantiate.
	AvailableTools tools.List

	// ContainerBridgeName, if non-empty, overrides the default
	// network bridge device to use for LXC and KVM containers. See
	// also instancecfg.DefaultBridgeName.
	ContainerBridgeName string

	// ImageMetadata contains simplestreams image metadata for providers
	// that rely on it for selecting images. This will be empty for
	// providers that do not implements simplestreams.HasRegion.
	ImageMetadata []*imagemetadata.ImageMetadata
}

// BootstrapFinalizer is a function returned from Environ.Bootstrap.
// The caller must pass a InstanceConfig with the Tools field set.
type BootstrapFinalizer func(BootstrapContext, *instancecfg.InstanceConfig) error

// BootstrapResult holds the data returned by calls to Environ.Bootstrap.
type BootstrapResult struct {
	// Arch is the instance's architecture.
	Arch string

	// Series is the instance's series.
	Series string

	// Finalize is a function that must be called to finalize the
	// bootstrap process by transferring the tools and installing the
	// initial Juju controller.
	Finalize BootstrapFinalizer
}

// BootstrapContext is an interface that is passed to
// Environ.Bootstrap, providing a means of obtaining
// information about and manipulating the context in which
// it is being invoked.
type BootstrapContext interface {
	GetStdin() io.Reader
	GetStdout() io.Writer
	GetStderr() io.Writer
	Infof(format string, params ...interface{})
	Verbosef(format string, params ...interface{})

	// InterruptNotify starts watching for interrupt signals
	// on behalf of the caller, sending them to the supplied
	// channel.
	InterruptNotify(sig chan<- os.Signal)

	// StopInterruptNotify undoes the effects of a previous
	// call to InterruptNotify with the same channel. After
	// StopInterruptNotify returns, no more signals will be
	// delivered to the channel.
	StopInterruptNotify(chan<- os.Signal)

	// ShouldVerifyCredentials indicates whether the caller's cloud
	// credentials should be verified.
	ShouldVerifyCredentials() bool
}
