// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
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
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	jujunames "github.com/juju/juju/juju/names"
)

// MachineRecoveryParams contains only the inputs needed to allocate and start
// a replacement machine controller from a recovery archive. State import and
// client registration belong to later recovery stages.
type MachineRecoveryParams struct {
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

// Validate checks the inputs for replacement machine allocation.
func (p MachineRecoveryParams) Validate() error {
	if cloud.CloudIsCAAS(p.Cloud) {
		return errors.Errorf("machine recovery for a Kubernetes cloud %w", coreerrors.NotValid)
	}
	if err := validateCommonRecoveryParams(p.AllocationParams, p.AgentVersion); err != nil {
		return err
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
func (p MachineRecoveryParams) bootstrapParams() BootstrapParams {
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

// K8sRecoveryParams contains only the inputs needed to allocate a replacement
// Kubernetes controller and start its agent from a recovery archive. State
// import and client registration belong to later recovery stages.
type K8sRecoveryParams struct {
	AllocationParams

	// AgentVersion is the exact archived controller agent version.
	AgentVersion semversion.Number

	// CAPrivateKey is the archived CA private key used to mint the
	// replacement controller certificate.
	CAPrivateKey string

	// AdminSecret is the administrator password used until state import
	// reconciles it.
	AdminSecret string

	// CloudCredentialName is the name of the cloud credential used to
	// allocate the replacement.
	CloudCredentialName string

	// DialOpts carries the timeout for the replacement deployment.
	DialOpts environs.BootstrapDialOpts
}

// Validate checks the inputs for replacement Kubernetes namespace allocation.
func (p K8sRecoveryParams) Validate() error {
	if !cloud.CloudIsCAAS(p.Cloud) {
		return errors.Errorf("Kubernetes recovery for a machine cloud %w", coreerrors.NotValid)
	}
	if err := validateCommonRecoveryParams(p.AllocationParams, p.AgentVersion); err != nil {
		return err
	}
	if !p.BootstrapBase.Empty() {
		return errors.Errorf("recovery with a bootstrap base for a Kubernetes controller %w", coreerrors.NotValid)
	}
	if p.CAPrivateKey == "" {
		return errors.Errorf("recovery without a CA private key %w", coreerrors.NotValid)
	}
	if p.AdminSecret == "" {
		return errors.Errorf("recovery without an admin secret %w", coreerrors.NotValid)
	}
	if p.ControllerName == "" {
		return errors.Errorf("recovery without a controller name %w", coreerrors.NotValid)
	}
	return nil
}

// bootstrapParams exposes the recovery inputs through the bootstrap
// configuration view the shared Kubernetes finalisation helpers consume.
func (p K8sRecoveryParams) bootstrapParams() BootstrapParams {
	return BootstrapParams{
		AllocationParams:    p.AllocationParams,
		AgentVersion:        &p.AgentVersion,
		CAPrivateKey:        p.CAPrivateKey,
		AdminSecret:         p.AdminSecret,
		CloudCredentialName: p.CloudCredentialName,
		DialOpts:            p.DialOpts,
	}
}

func validateCommonRecoveryParams(p AllocationParams, agentVersion semversion.Number) error {
	if err := p.validateCommon(); err != nil {
		return err
	}
	if agentVersion == (semversion.Number{}) {
		return errors.Errorf("recovery without an agent version %w", coreerrors.NotValid)
	}
	return nil
}

// RecoverMachine allocates a replacement machine controller instance from a
// recovery archive through the allocation stages shared with Bootstrap,
// preserving the archived agent version and base, installs and starts its
// agent, and stops before importing state. A fresh controller bootstrap is
// done by Bootstrap.
//
// RecoverMachine returns the replacement instance ID whenever the provider
// reports a successful allocation, even if a later check fails. In that case
// the error also names the instance for manual cleanup. If the provider itself
// fails, RecoverMachine returns an empty ID and preserves the provider's error.
func RecoverMachine(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args MachineRecoveryParams,
) (instance.Id, error) {
	if err := args.Validate(); err != nil {
		return "", errors.Errorf("validating recovery parameters: %w", err)
	}

	params := providerAllocationParams(args.AllocationParams)
	// Pass the archived logical machine ID into provider allocation so
	// StartInstance creates per-machine resources (hostnames, firewall
	// groups, tags) for that machine rather than assuming machine "0".
	params.MachineID = args.MachineID
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
	allocated := result.InstanceID
	err = checkMachineRecoveryResult(args, result)
	if err == nil {
		var instanceConfig *instancecfg.InstanceConfig
		instanceConfig, err = finishMachineBootstrap(
			ctx, environ, args.bootstrapParams(), bootstrapParams, result, prep.customImageMetadata,
		)
		if err == nil {
			instanceConfig.MachineId = args.MachineID
			instanceConfig.MachineAgentServiceName = jujunames.JujuAgentd + "-" +
				names.NewMachineTag(args.MachineID).String()
			instanceConfig.Bootstrap.StateInitializationParams.RecoveryMode = true
			err = result.CloudBootstrapFinalizer(ctx, instanceConfig, args.DialOpts)
		}
	}
	if err == nil && IsContextDone(ctx) {
		err = Cancelled()
	}
	if err != nil {
		// Wrapping rather than replacing the error keeps its
		// classification, including the cancellation sentinel,
		// intact.
		return allocated, errors.Errorf(
			"replacement instance %q may require manual removal: %w",
			allocated, err,
		)
	}

	ctx.Infof("Replacement agent now started on instance %q", allocated)
	return allocated, nil
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
	args MachineRecoveryParams,
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

// k8sRecoveryEnviron supplies the recovery-specific operations after deployment.
type k8sRecoveryEnviron interface {
	Namespace() string

	// WaitForRecoveryAgent returns when the replacement agent has reached its
	// import fence, or when startup fails, times out or is cancelled.
	WaitForRecoveryAgent(context.Context, *podcfg.ControllerPodConfig) error
}

// RecoverK8s prepares the replacement Kubernetes controller allocation inputs
// and starts its agent, stopping before importing state. A fresh controller
// bootstrap is done by Bootstrap.
//
// The caller must have prepared the environ with PrepareForBootstrap using
// args.ControllerName so the provider knows the target namespace.
//
// RecoverK8s returns the prepared replacement namespace whenever finalisation
// runs and fails or observes cancellation afterwards, so the caller can name
// it for manual cleanup. If bootstrap fails before finalisation runs,
// RecoverK8s returns an empty namespace and preserves the provider's error.
func RecoverK8s(
	ctx environs.BootstrapContext,
	environ environs.BootstrapEnviron,
	args K8sRecoveryParams,
) (string, error) {
	if err := args.Validate(); err != nil {
		return "", errors.Errorf("validating recovery parameters: %w", err)
	}

	target, ok := environ.(k8sRecoveryEnviron)
	if !ok {
		return "", errors.Errorf("Kubernetes recovery allocation for %T %w", environ, coreerrors.NotSupported)
	}

	bootstrapParams := providerAllocationParams(args.AllocationParams)
	bootstrapConstraints, err := prepareK8sConstraints(ctx, environ, args.AllocationParams)
	if err != nil {
		return "", errors.Capture(err)
	}
	bootstrapParams.BootstrapConstraints = bootstrapConstraints

	result, err := environ.Bootstrap(ctx, bootstrapParams)
	if err != nil {
		return "", errors.Capture(err)
	}
	if result == nil || result.CaasBootstrapFinalizer == nil {
		return "", errors.New("provider returned no Kubernetes controller finaliser")
	}

	podConfig, err := finishK8sBootstrap(
		ctx, environ, args.bootstrapParams(), result, bootstrapConstraints,
	)
	if err != nil {
		return "", errors.Capture(err)
	}
	podConfig.Bootstrap.StateInitializationParams.RecoveryMode = true
	err = result.CaasBootstrapFinalizer(ctx, podConfig, args.DialOpts)

	namespace := target.Namespace()
	if err == nil {
		if namespace == "" {
			return "", errors.New("recovery provisioning returned no replacement namespace")
		}
		if IsContextDone(ctx) {
			err = Cancelled()
		} else {
			err = target.WaitForRecoveryAgent(ctx, podConfig)
			switch {
			case IsContextDone(ctx):
				err = Cancelled()
			case err != nil:
				err = errors.Errorf("waiting for replacement agent: %w", err)
			}
		}
	}
	if err != nil {
		if namespace != "" {
			// Wrapping rather than replacing the error keeps its
			// classification, including the cancellation sentinel, intact.
			return namespace, errors.Errorf(
				"replacement namespace %q may require manual removal: %w",
				namespace, err,
			)
		}
		return "", errors.Capture(err)
	}

	ctx.Infof("Replacement agent now started in namespace %q", namespace)
	return namespace, nil
}

// checkMachineRecoveryResult verifies that the provider allocated a
// replacement machine controller matching the archived base and architecture.
// The result is non-nil and identifies an allocated instance.
func checkMachineRecoveryResult(
	args MachineRecoveryParams,
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
