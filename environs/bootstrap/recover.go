// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// RecoveryParams holds the parameters for allocating a replacement
// controller from a recovery archive. It embeds the ordinary bootstrap
// parameters and adds the recovery-only inputs.
type RecoveryParams struct {
	BootstrapParams

	// AvailableTools is the exact archived agent set for IAAS recovery.
	// It bypasses packaged-tool discovery and local tool building.
	AvailableTools coretools.List
}

// Validate validates the recovery parameters, including the shared bootstrap
// parameters and the recovery-only combinations.
func (p RecoveryParams) Validate() error {
	// TODO(k8s): support recovering a Kubernetes controller by
	// allocating a replacement controller namespace. Only machine
	// clouds are recoverable for now; the guard runs before the
	// embedded validation, whose supported machine bases are
	// machine-only requirements.
	if cloud.CloudIsCAAS(p.Cloud) {
		return errors.Errorf("recovery of a Kubernetes controller %w", coreerrors.NotSupported)
	}
	if err := p.BootstrapParams.Validate(); err != nil {
		return errors.Capture(err)
	}
	if p.AgentVersion == nil {
		return errors.Errorf("recovery without an agent version %w", coreerrors.NotValid)
	}
	if p.BuildAgent {
		return errors.Errorf("recovery with agent building enabled %w", coreerrors.NotValid)
	}
	if p.BootstrapBase.Empty() {
		return errors.Errorf("recovery without a bootstrap base %w", coreerrors.NotValid)
	}
	if p.BootstrapConstraints.Arch == nil {
		return errors.Errorf("recovery without a bootstrap architecture %w", coreerrors.NotValid)
	}
	hasTool := false
	for _, tool := range p.AvailableTools {
		if tool == nil {
			continue
		}
		hasTool = true
		if tool.Version.Number != *p.AgentVersion {
			return errors.Errorf(
				"recovery tool version %q not matching agent version %q %w",
				tool.Version.Number, *p.AgentVersion, coreerrors.NotValid,
			)
		}
	}
	if !hasTool {
		return errors.Errorf("recovery without any agent tools %w", coreerrors.NotValid)
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

	bootstrapParams := providerBootstrapParams(args.BootstrapParams)
	allocated, err := recoverIAAS(ctx, environ, args, bootstrapParams)
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
// archive, using the exact archived agent binaries and base. It returns
// the ID of the allocated instance, which is empty when the provider
// reported no valid allocation, and reports the same ID alongside errors
// detected after a valid allocation, so the caller can name the instance.
func recoverIAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
	bootstrapParams environs.BootstrapParams,
) (instance.Id, error) {
	if _, err := prepareIAASBootstrap(ctx, environ, args.BootstrapParams, &bootstrapParams); err != nil {
		return "", errors.Capture(err)
	}
	bootstrapParams.AvailableTools = args.AvailableTools

	if err := prepareProviderBootstrap(ctx, environ, args.BootstrapParams, &bootstrapParams, *args.AgentVersion); err != nil {
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
	if result.Base != args.BootstrapBase {
		return errors.New("replacement base differs from archived controller")
	}
	if result.Arch != *args.BootstrapConstraints.Arch {
		return errors.New("replacement architecture differs from archived agent")
	}
	return nil
}
