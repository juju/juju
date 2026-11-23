// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/juju/names/v6"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	jujunames "github.com/juju/juju/juju/names"
)

// RecoveryParams contains only the inputs needed to allocate and start a
// replacement machine controller from a recovery archive. State import and
// client registration belong to later recovery stages.
type RecoveryParams struct {
	AllocationParams

	// AgentVersion is the exact archived controller agent version.
	AgentVersion semversion.Number

	// Arch is the exact archived controller architecture, enforced on both
	// the allocation request and the provider result.
	Arch string

	// MachineID is the archived logical machine identity of the controller,
	// for example "0" or "7".
	MachineID string

	// AvailableTools, when non-empty, supplies pre-resolved agent tools
	// directly (for example in tests). When empty, recovery resolves packaged
	// tools matching AgentVersion and falls back to packaging the local
	// jujuagentd binary via BuildAgentTarball.
	AvailableTools coretools.List

	// BuildAgentTarball packages the local jujuagentd binary when no packaged
	// tools match AgentVersion. It is called with build=false so official
	// snap/package installs copy the shipped binary instead of trying to
	// compile from source.
	BuildAgentTarball sync.BuildAgentTarballFunc

	// CAPrivateKey is the archived CA private key used to mint the
	// replacement controller certificate.
	CAPrivateKey string

	// AdminSecret is the administrator password used until state import
	// reconciles it.
	AdminSecret string

	// CloudCredentialName is the name of the cloud credential used to
	// allocate the replacement.
	CloudCredentialName string

	// BootstrapSSHAuthorizedKeys are installed on the replacement instance so
	// bootstrap configuration can connect to it.
	BootstrapSSHAuthorizedKeys []string

	// DialOpts controls the connection to the replacement instance during
	// bootstrap configuration.
	DialOpts environs.BootstrapDialOpts
}

// Validate checks the inputs for replacement machine allocation and agent
// startup.
func (p RecoveryParams) Validate() error {
	if cloud.CloudIsCAAS(p.Cloud) {
		return errors.Errorf("recovery of a Kubernetes controller %w", coreerrors.NotSupported)
	}
	if err := p.AllocationParams.validateCommon(); err != nil {
		return err
	}
	if p.AgentVersion == (semversion.Number{}) {
		return errors.Errorf("recovery without an agent version %w", coreerrors.NotValid)
	}
	if p.BootstrapBase.Empty() {
		return errors.Errorf("recovery without a controller base %w", coreerrors.NotValid)
	}
	if !arch.IsSupportedArch(p.Arch) {
		return errors.Errorf("recovery controller architecture %q %w", p.Arch, coreerrors.NotValid)
	}
	if !names.IsValidMachine(p.MachineID) {
		return errors.Errorf("recovery machine ID %q %w", p.MachineID, coreerrors.NotValid)
	}
	if p.CAPrivateKey == "" {
		return errors.Errorf("recovery without a CA private key %w", coreerrors.NotValid)
	}
	if p.AdminSecret == "" {
		return errors.Errorf("recovery without an admin secret %w", coreerrors.NotValid)
	}
	if len(p.AvailableTools) == 0 && p.BuildAgentTarball == nil {
		return errors.Errorf("recovery without agent tools or builder %w", coreerrors.NotValid)
	}
	for _, tool := range p.AvailableTools {
		if tool == nil {
			return errors.Errorf("recovery with nil agent tool entry %w", coreerrors.NotValid)
		}
		if tool.Version.Number != p.AgentVersion {
			return errors.Errorf(
				"recovery tool version %q not matching agent version %q %w",
				tool.Version.Number, p.AgentVersion, coreerrors.NotValid)
		}
	}
	// The archived architecture is authoritative: an explicit constraint
	// naming a different one would silently provision the wrong platform.
	if p.BootstrapConstraints.Arch != nil && *p.BootstrapConstraints.Arch != "" &&
		*p.BootstrapConstraints.Arch != p.Arch {
		return errors.Errorf(
			"recovery controller architecture %q conflicts with constraint architecture %q %w",
			p.Arch, *p.BootstrapConstraints.Arch, coreerrors.NotValid)
	}
	return nil
}

// bootstrapParams exposes the recovery inputs through the bootstrap
// configuration view the shared machine finalisation helpers consume.
func (p RecoveryParams) bootstrapParams() BootstrapParams {
	return BootstrapParams{
		AllocationParams:           p.AllocationParams,
		AgentVersion:               &p.AgentVersion,
		CAPrivateKey:               p.CAPrivateKey,
		AdminSecret:                p.AdminSecret,
		CloudCredentialName:        p.CloudCredentialName,
		BootstrapSSHAuthorizedKeys: p.BootstrapSSHAuthorizedKeys,
		DialOpts:                   p.DialOpts,
	}
}

// Recover allocates a replacement machine controller instance from a recovery
// archive through the allocation stages shared with Bootstrap, preserving the
// archived agent version and base, installs and starts its agent, and stops
// before importing state. A fresh controller bootstrap is done by Bootstrap.
//
// Recover returns the replacement instance ID whenever the provider reports a
// successful allocation, even if a later check or finalisation fails. In that
// case the error also names the instance for manual cleanup. If the provider
// itself fails, Recover returns an empty ID and preserves the provider's error.
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

	ctx.Infof("Replacement agent now started on instance %q", allocated)
	return allocated, nil
}

// recoverIAAS allocates and starts a replacement IAAS controller agent from
// the archive, using the exact archived agent version and platform. It returns
// the ID of the allocated instance, which is empty when the provider reported
// no valid allocation, and reports the same ID alongside errors detected after
// a valid allocation, so the caller can name the instance.
func recoverIAAS(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args RecoveryParams,
) (instance.Id, error) {
	params := providerAllocationParams(args.AllocationParams)
	// Pass the archived logical machine ID into provider allocation so
	// StartInstance creates per-machine resources (hostnames, firewall
	// groups, tags) for that machine rather than assuming machine "0".
	params.MachineId = args.MachineID
	// The provider allocation does not inherit the bootstrap SSH keys;
	// cloud-init needs them before bootstrap configuration can connect.
	params.AuthorizedKeys = args.BootstrapSSHAuthorizedKeys
	// The archived base is the only base a replacement may use; it is
	// trivially the supported set for this allocation.
	if len(params.SupportedBootstrapBases) == 0 {
		params.SupportedBootstrapBases = []base.Base{args.BootstrapBase}
	}
	// Pin the archived architecture, keeping every other archived
	// constraint so the replacement matches the source instance.
	params.BootstrapConstraints.Arch = &args.Arch
	// Merge the spaces implied by the archived controller space config,
	// exactly as fresh bootstrap does, before the constraints are
	// validated.
	params.BootstrapConstraints.Spaces = args.ControllerConfig.AsSpaceConstraints(
		params.BootstrapConstraints.Spaces)

	prep, err := prepareMachineAllocation(
		ctx, environ, params, args.MetadataDir, args.BootstrapImage)
	if err != nil {
		return "", errors.Capture(err)
	}

	availableTools, builtTools, err := discoverRecoveryTools(ctx, environ, prep, args)
	if err != nil {
		return "", errors.Capture(err)
	}
	if builtTools != nil {
		defer func() { _ = os.RemoveAll(builtTools.Dir) }()
	}
	bootstrapParams := prep.params
	bootstrapParams.AvailableTools = availableTools
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
	instanceConfig, err := finishMachineBootstrap(
		ctx, environ, args.bootstrapParams(), bootstrapParams, result, prep.customImageMetadata,
	)
	if err != nil {
		return result.InstanceID, err
	}
	instanceConfig.MachineId = args.MachineID
	instanceConfig.MachineAgentServiceName = jujunames.JujuAgentd + "-" +
		names.NewMachineTag(args.MachineID).String()
	instanceConfig.Bootstrap.StateInitializationParams.RecoveryMode = true
	if err := result.CloudBootstrapFinalizer(ctx, instanceConfig, args.DialOpts); err != nil {
		return result.InstanceID, err
	}
	return result.InstanceID, nil
}

// discoverRecoveryTools resolves the exact archived agent binary for machine
// recovery. If the caller supplied AvailableTools directly, those are used
// as-is. Otherwise it searches packaged tools first (matching the exact
// archived version, OS and architecture) and falls back to packaging the local
// jujuagentd binary (without building from source) after checking that the
// client host OS and architecture match the archived controller.
func discoverRecoveryTools(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	prep machinePreparation,
	args RecoveryParams,
) (coretools.List, *sync.BuiltAgent, error) {
	if len(args.AvailableTools) > 0 {
		return args.AvailableTools, nil, nil
	}

	ctx.Infof("Looking for packaged Juju agent version %s for %s", args.AgentVersion, prep.arch)
	availableTools, err := findPackagedTools(
		ctx, environ, prep.simplestreams, &args.AgentVersion, &prep.arch, &prep.params.BootstrapBase,
	)
	if err != nil && !errors.Is(err, coreerrors.NotFound) {
		return nil, nil, errors.Capture(err)
	}
	if len(availableTools) > 0 {
		for _, tool := range availableTools {
			ctx.Infof("Located Juju agent version %s at %s", tool.Version, tool.URL)
		}
		return availableTools, nil, nil
	}

	if args.BuildAgentTarball == nil {
		return nil, nil, errors.New("cannot package local agent binary to upload")
	}
	if err := validateUploadAllowed(environ, &prep.arch, &prep.params.BootstrapBase, prep.validator); err != nil {
		return nil, nil, errors.Capture(err)
	}
	// The fallback packages the local jujuagentd binary, so callers must
	// ensure their own agent version equals args.AgentVersion exactly, as
	// the recovering client does before allocation.
	ctx.Infof("No packaged binary found, preparing local Juju agent binary")
	builtTools, err := args.BuildAgentTarball(
		false, "bootstrap", func(semversion.Number) semversion.Number {
			return args.AgentVersion
		},
	)
	if err != nil {
		return nil, nil, errors.Errorf("cannot package recovery agent binary: %w", err)
	}
	return coretools.List{&coretools.Tools{
		Version: semversion.Binary{
			Number:  args.AgentVersion,
			Release: prep.params.BootstrapBase.OS,
			Arch:    prep.arch,
		},
		URL:    "file://" + filepath.Join(builtTools.Dir, builtTools.StorageName),
		SHA256: builtTools.Sha256Hash,
		Size:   builtTools.Size,
	}}, builtTools, nil
}

// checkRecoveryBootstrapResult verifies that the provider allocated a
// replacement controller matching the archived base and architecture. The
// result is non-nil and identifies an allocated instance.
func checkRecoveryBootstrapResult(
	args RecoveryParams,
	result *environs.BootstrapResult,
) error {
	if result.CloudBootstrapFinalizer == nil {
		return errors.New("provider returned no machine controller finaliser")
	}
	if result.Base != args.BootstrapBase {
		return errors.New("replacement base differs from archived controller")
	}
	if result.Arch != args.Arch {
		return errors.New("replacement architecture differs from archived agent")
	}
	return nil
}
