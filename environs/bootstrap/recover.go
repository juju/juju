// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	coretools "github.com/juju/juju/internal/tools"
)

// RecoveryParams holds the parameters for allocating a replacement controller
// from a recovery archive. It embeds the ordinary bootstrap parameters and
// adds the recovery-only inputs. It is always used as a whole: a
// RecoveryParams value means recovery, so there is no mode flag to check.
type RecoveryParams struct {
	BootstrapParams

	// AvailableTools is the exact archived agent set for IAAS recovery.
	// It bypasses packaged-tool discovery and local tool building.
	AvailableTools coretools.List

	// InstanceStarted, when non-nil, observes the replacement instance ID.
	InstanceStarted func(instance.Id)
}

// Validate validates the recovery parameters, including the shared bootstrap
// parameters and the recovery-only combinations.
func (p RecoveryParams) Validate() error {
	if err := p.BootstrapParams.Validate(); err != nil {
		return errors.Trace(err)
	}
	if p.AgentVersion == nil {
		return errors.NotValidf("recovery without an agent version")
	}
	if p.BuildAgent {
		return errors.NotValidf("recovery with agent building enabled")
	}
	if p.BootstrapBase.Empty() {
		return errors.NotValidf("recovery without a bootstrap base")
	}
	if p.BootstrapConstraints.Arch == nil {
		return errors.NotValidf("recovery without a bootstrap architecture")
	}
	hasTool := false
	for _, tool := range p.AvailableTools {
		if tool == nil {
			continue
		}
		hasTool = true
		if tool.Version.Number != *p.AgentVersion {
			return errors.NotValidf(
				"recovery tool version %q not matching agent version %q",
				tool.Version.Number, *p.AgentVersion,
			)
		}
	}
	if !hasTool {
		return errors.NotValidf("recovery without any agent tools")
	}
	return nil
}

// checkRecoveryBootstrapResult verifies that the provider allocated a
// replacement controller matching the archived base and architecture.
func checkRecoveryBootstrapResult(
	args RecoveryParams,
	result *environs.BootstrapResult,
) error {
	if result == nil {
		return errors.New("provider returned no replacement instance details")
	}
	if result.Base != args.BootstrapBase {
		return errors.New("replacement base differs from archived controller")
	}
	if result.Arch != *args.BootstrapConstraints.Arch {
		return errors.New("replacement architecture differs from archived agent")
	}
	return nil
}

// Recover allocates a replacement controller instance from a recovery
// archive and always stops before installing an agent or initialising its
// database. It shares the allocation stages of Bootstrap, but always
// performs recovery: the archived agent version and base are preserved. A
// fresh controller bootstrap is done by Bootstrap.
func Recover(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
) error {
	if err := args.Validate(); err != nil {
		return errors.Annotate(err, "validating recovery parameters")
	}

	bootstrapParams := providerBootstrapParams(args.BootstrapParams)
	if err := recoverIAAS(ctx, environ, args, bootstrapParams); err != nil {
		return errors.Trace(err)
	}
	if IsContextDone(ctx) {
		ctx.Infof("Recovery allocation cancelled, you may need to manually remove the replacement instance")
		return Cancelled()
	}

	ctx.Infof("Replacement instance now allocated")
	return nil
}

// recoverIAAS allocates a replacement IAAS controller instance from the
// archive, using the exact archived agent binaries and base.
func recoverIAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
	bootstrapParams environs.BootstrapParams,
) error {
	if _, err := prepareIAASBootstrap(ctx, environ, args.BootstrapParams, &bootstrapParams); err != nil {
		return errors.Trace(err)
	}
	bootstrapParams.AvailableTools = args.AvailableTools

	if err := prepareProviderBootstrap(ctx, environ, args.BootstrapParams, &bootstrapParams, *args.AgentVersion); err != nil {
		return errors.Trace(err)
	}

	var started instance.Id
	bootstrapParams.InstanceStarted = func(id instance.Id) {
		started = id
		if args.InstanceStarted != nil {
			args.InstanceStarted(id)
		}
	}

	ctx.Verbosef("Starting new instance for replacement controller")
	result, err := environ.Bootstrap(ctx, bootstrapParams)
	if err != nil {
		return errors.Trace(err)
	}
	if started == "" {
		return errors.New("recovery provisioning returned no replacement instance")
	}
	return errors.Trace(checkRecoveryBootstrapResult(args, result))
}
