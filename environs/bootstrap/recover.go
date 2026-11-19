// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// RecoveryParams contains only the inputs needed to allocate a replacement
// machine. Identity restoration, agent installation and client registration
// belong to later recovery stages.
type RecoveryParams struct {
	ControllerConfig controller.Config
	Cloud            cloud.Cloud
	CloudRegion      string
	CloudCredential  *cloud.Credential

	// AgentVersion, Base and Arch are the exact archived controller platform.
	AgentVersion semversion.Number
	Base         base.Base
	Arch         string
}

// Validate checks the inputs for replacement machine allocation.
func (p RecoveryParams) Validate() error {
	if cloud.CloudIsCAAS(p.Cloud) {
		return errors.Errorf("recovery of a Kubernetes controller %w", coreerrors.NotSupported)
	}
	if uuid, _ := p.ControllerConfig[controller.ControllerUUIDKey].(string); uuid == "" {
		return errors.New("controller configuration has no controller UUID")
	}
	if _, ok := p.ControllerConfig.CACert(); !ok {
		return errors.New("controller configuration has no ca-cert")
	}
	if p.AgentVersion == (semversion.Number{}) {
		return errors.Errorf("recovery without an agent version %w", coreerrors.NotValid)
	}
	if p.Base.Empty() {
		return errors.Errorf("recovery without a controller base %w", coreerrors.NotValid)
	}
	if !arch.IsSupportedArch(p.Arch) {
		return errors.Errorf("recovery controller architecture %q %w", p.Arch, coreerrors.NotValid)
	}
	return nil
}

// Recover allocates a replacement controller instance from a recovery
// archive and always stops before installing an agent or initialising its
// database. It shares the allocation stages of Bootstrap, but always
// performs recovery: the archived agent version and base are preserved. A
// fresh controller bootstrap is done by Bootstrap.
//
// Recover returns the replacement instance ID whenever the provider reports
// a successful allocation, even if a later check fails. In that case the
// error also names the instance for manual cleanup. If the provider itself
// fails, Recover returns an empty ID and preserves the provider's error.
func Recover(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
) (instance.Id, error) {
	if err := args.Validate(); err != nil {
		return "", errors.Errorf("validating recovery parameters: %w", err)
	}

	allocated, err := recoverIAAS(ctx, environ, args)
	if err == nil && IsContextDone(ctx) {
		err = Cancelled()
	}
	if err != nil {
		if allocated != "" {
			// Wrapping rather than replacing the error keeps its
			// classification, including the cancellation sentinel,
			// intact.
			return allocated, errors.Errorf(
				"replacement instance %q may require manual removal: %w",
				allocated, err,
			)
		}
		return "", err
	}

	ctx.Infof("Replacement instance %q now allocated", allocated)
	return allocated, nil
}

// recoverIAAS allocates a replacement IAAS controller instance from the
// archive, using the exact archived agent version and platform. It returns
// the ID of the allocated instance, which is empty when the provider
// reported no valid allocation, and reports the same ID alongside errors
// detected after a valid allocation, so the caller can name the instance.
func recoverIAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
) (instance.Id, error) {
	prep, err := prepareMachineAllocation(ctx, environ, environs.BootstrapParams{
		ControllerConfig:        args.ControllerConfig,
		CloudName:               args.Cloud.Name,
		CloudRegion:             args.CloudRegion,
		BootstrapBase:           args.Base,
		SupportedBootstrapBases: []base.Base{args.Base},
		BootstrapConstraints:    constraints.Value{Arch: &args.Arch},
	}, "", "")
	if err != nil {
		return "", errors.Capture(err)
	}

	// The provider allocation API uses tools to select an image. Adapt the
	// archived platform at that boundary; this is not an installable agent.
	// Recovery never calls the fresh-bootstrap finaliser returned below.
	bootstrapParams := prep.params
	bootstrapParams.AvailableTools = coretools.List{{Version: semversion.Binary{
		Number: args.AgentVersion, Release: args.Base.OS, Arch: args.Arch,
	}}}
	bootstrapParams, err = prepareProviderAllocation(ctx, environ, args.CloudCredential, bootstrapParams, args.AgentVersion)
	if err != nil {
		return "", errors.Capture(err)
	}

	ctx.Verbosef("Starting new instance for replacement controller")
	result, err := environ.Bootstrap(ctx, bootstrapParams)
	if err != nil {
		// The provider enriches its own errors with any instance it
		// allocated before failing, so recovery returns them
		// unchanged.
		return "", err
	}
	if result == nil {
		return "", errors.New("provider returned no replacement instance details")
	}
	if result.InstanceID == "" {
		return "", errors.New("recovery provisioning returned no replacement instance")
	}
	if err := checkRecoveryBootstrapResult(args, result); err != nil {
		return result.InstanceID, err
	}
	return result.InstanceID, nil
}

// checkRecoveryBootstrapResult verifies that the provider allocated a
// replacement controller matching the archived base and architecture. The
// result is non-nil and identifies an allocated instance.
func checkRecoveryBootstrapResult(
	args RecoveryParams,
	result *environs.BootstrapResult,
) error {
	if result.Base != args.Base {
		return errors.New("replacement base differs from archived controller")
	}
	if result.Arch != args.Arch {
		return errors.New("replacement architecture differs from archived agent")
	}
	return nil
}
