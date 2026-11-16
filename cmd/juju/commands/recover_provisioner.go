// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"maps"
	"os"
	"os/signal"
	"sort"
	"time"

	"github.com/juju/names/v6"
	jujusshtools "github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/api/jujuclient"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	jujussh "github.com/juju/juju/internal/ssh"
	internalstorage "github.com/juju/juju/internal/storage"
)

const (
	// recoveryAllocationTimeout bounds provider preparation, replacement
	// allocation and agent startup after the client has resolved the cloud
	// and credentials.
	recoveryAllocationTimeout = 20 * time.Minute

	// recoveryDialRetryDelay is the delay between attempts to connect to
	// the replacement instance during bootstrap configuration.
	recoveryDialRetryDelay = 5 * time.Second

	// recoveryDialAddressesDelay is the delay between refreshing the
	// replacement instance addresses during bootstrap configuration.
	recoveryDialAddressesDelay = 5 * time.Second
)

// provisionRecovery prepares and starts a replacement controller agent from
// the recovery archive through the ordinary bootstrap pipeline: a machine for
// machine clouds, or a controller pod in the replacement namespace for
// Kubernetes clouds. State import and client registration are left to later
// recovery stages.
func provisionRecovery(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	archivePath string,
	info *recovery.ArchiveInfo,
) error {
	// The replacement runs this client's own agent binary when no packaged
	// tools match the archive, so recovery requires this client to be the
	// exact archived agent version, build number included.
	if err := info.CheckAgentVersion(jujuversion.Current); err != nil {
		return errors.Capture(err)
	}
	if jujucloud.CloudTypeIsCAAS(info.CloudType) {
		return provisionK8sRecovery(ctx, store, info)
	}
	return provisionMachineRecovery(ctx, store, archivePath, info)
}

// provisionMachineRecovery starts a replacement controller machine, uploads
// the archive and initialises the replacement Dqlite database.
func provisionMachineRecovery(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	archivePath string,
	info *recovery.ArchiveInfo,
) error {
	params, err := recoveryParamsFromArchive(info)
	if err != nil {
		return errors.Capture(err)
	}
	if !names.IsValidMachine(info.MachineName) {
		return errors.New("archive does not record a usable controller machine")
	}
	params.MachineID = info.MachineName

	// The recovering client's public keys are installed on the replacement
	// instance so bootstrap configuration can connect to it.
	keyFiles := jujusshtools.PrivateKeyFiles()
	if len(keyFiles) == 0 {
		return errors.New("no Juju SSH keys available for recovery")
	}
	sort.Strings(keyFiles)
	authorizedKeys, err := jujussh.PublicKeysForPrivateKeyFiles(keyFiles)
	if err != nil {
		return errors.Errorf("reading Juju recovery SSH key: %w", err)
	}

	allocation, err := openRecoveryAllocation(ctx, store, info)
	if err != nil {
		return errors.Capture(err)
	}
	defer allocation.stop()

	adminSecret, err := password.RandomPassword()
	if err != nil {
		return errors.Capture(err)
	}

	allocation.apply(&params.AllocationParams)
	params.CloudCredentialName = allocation.target.credentialName
	params.CAPrivateKey = info.CAPrivateKey
	params.AdminSecret = adminSecret
	params.BootstrapSSHAuthorizedKeys = authorizedKeys
	params.DialOpts = environs.BootstrapDialOpts{
		Timeout:        recoveryAllocationTimeout,
		IdentityFiles:  keyFiles,
		RetryDelay:     recoveryDialRetryDelay,
		AddressesDelay: recoveryDialAddressesDelay,
	}
	params.ArchivePath = archivePath
	params.ArchiveSHA256 = info.Checksum
	params.SystemIdentity = info.SystemIdentity
	params.BuildAgentTarball = sync.BuildAgentTarball

	started, err := bootstrap.RecoverMachine(allocation.ctx, allocation.environ, params)
	if err != nil {
		return errors.Capture(err)
	}
	return recoveryImportUnavailable("initialised replacement database on instance " + string(started))
}

// provisionK8sRecovery prepares and starts a replacement controller pod in
// the replacement namespace from the recovery archive.
func provisionK8sRecovery(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	info *recovery.ArchiveInfo,
) error {
	params, err := k8sRecoveryParamsFromArchive(info)
	if err != nil {
		return errors.Capture(err)
	}

	allocation, err := openRecoveryAllocation(ctx, store, info)
	if err != nil {
		return errors.Capture(err)
	}
	defer allocation.stop()

	adminSecret, err := password.RandomPassword()
	if err != nil {
		return errors.Capture(err)
	}

	allocation.apply(&params.AllocationParams)
	params.CloudCredentialName = allocation.target.credentialName
	params.CAPrivateKey = info.CAPrivateKey
	params.AdminSecret = adminSecret
	params.DialOpts.Timeout = recoveryAllocationTimeout

	namespace, err := bootstrap.RecoverK8s(allocation.ctx, allocation.environ, params)
	if err != nil {
		return errors.Capture(err)
	}
	return recoveryImportUnavailable("started replacement agent in namespace " + namespace)
}

// recoveryImportUnavailable reports how far the replacement got before import.
func recoveryImportUnavailable(progress string) error {
	return errors.Errorf("controller recovery database import is not available in this build; %s", progress)
}

// recoveryAllocation is the replacement environment opened for allocation,
// together with the archived inputs both recovery workflows populate their
// parameters from.
type recoveryAllocation struct {
	// ctx bounds allocation and unwinds on Ctrl-C.
	ctx              environs.BootstrapContext
	environ          environs.BootstrapEnviron
	target           *recoveryTarget
	controllerConfig controller.Config

	// stop releases the allocation context and its interrupt handler.
	stop func()
}

// apply fills the allocation inputs both recovery workflows take from the
// resolved cloud and the archived controller configuration.
func (a *recoveryAllocation) apply(params *bootstrap.AllocationParams) {
	params.Cloud = a.target.cloud
	params.CloudRegion = a.target.spec.Region
	params.CloudCredential = a.target.spec.Credential
	params.ControllerConfig = a.controllerConfig
}

// openRecoveryAllocation resolves the archived cloud, credentials and
// configuration, then opens and prepares the replacement environment for
// allocation. The caller must call stop when allocation finishes.
func openRecoveryAllocation(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	info *recovery.ArchiveInfo,
) (*recoveryAllocation, error) {
	target, err := recoveryCredentials(ctx, store, info)
	if err != nil {
		return nil, errors.Capture(err)
	}
	controllerConfig, modelConfig, err := recoveryConfigs(ctx, info, target.provider)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Let allocation unwind on Ctrl-C so it can report an instance or
	// namespace that may need manual removal.
	interruptCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	stdCtx, cancel := context.WithTimeout(interruptCtx, recoveryAllocationTimeout)
	provisionCtx := environscmd.BootstrapContext(stdCtx, ctx)
	release := func() {
		cancel()
		stop()
	}

	environ, err := bootstrap.OpenController(
		jujucloud.CloudTypeIsCAAS(info.CloudType), provisionCtx, environs.OpenParams{
			Config:         modelConfig,
			ControllerUUID: info.ControllerUUID,
			Cloud:          target.spec,
		})
	if err != nil {
		release()
		return nil, errors.Capture(err)
	}
	if err := environ.PrepareForBootstrap(provisionCtx, info.ControllerName); err != nil {
		release()
		return nil, errors.Capture(err)
	}
	return &recoveryAllocation{
		ctx:              provisionCtx,
		environ:          environ,
		target:           target,
		controllerConfig: controllerConfig,
		stop:             release,
	}, nil
}

// recoveryParamsFromArchive prepares the machine-specific inputs before cloud
// access. The client can run on a different platform from the controller.
func recoveryParamsFromArchive(info *recovery.ArchiveInfo) (bootstrap.MachineRecoveryParams, error) {
	if info.SourceBase == "" {
		return bootstrap.MachineRecoveryParams{}, errors.New("archive does not record a usable controller base")
	}
	targetBase, err := base.ParseBaseFromString(info.SourceBase)
	if err != nil {
		return bootstrap.MachineRecoveryParams{}, errors.Errorf("archive does not record a usable controller base: %w", err)
	}
	targetArch := arch.NormaliseArch(info.SourceArch)
	if !arch.IsSupportedArch(targetArch) {
		return bootstrap.MachineRecoveryParams{}, errors.Errorf("archive does not record a usable controller architecture: %q", info.SourceArch)
	}
	// The archived architecture is authoritative: allocation pins it over
	// whatever the recorded constraint says.
	bootstrapConstraints := info.ControllerConstraints
	bootstrapConstraints.Arch = &targetArch
	return bootstrap.MachineRecoveryParams{
		AllocationParams: bootstrap.AllocationParams{
			ControllerName:          info.ControllerName,
			ModelConstraints:        info.ModelConstraints,
			BootstrapConstraints:    bootstrapConstraints,
			StoragePools:            cloneStoragePools(info.StoragePools),
			BootstrapBase:           targetBase,
			SupportedBootstrapBases: []base.Base{targetBase},
		},
		AgentVersion: info.AgentVersion,
		Arch:         targetArch,
	}, nil
}

// k8sRecoveryParamsFromArchive prepares the Kubernetes allocation inputs
// before cloud access.
func k8sRecoveryParamsFromArchive(info *recovery.ArchiveInfo) (bootstrap.K8sRecoveryParams, error) {
	if info.ControllerName == "" {
		return bootstrap.K8sRecoveryParams{}, errors.New("recovery archive does not record the source controller name")
	}
	return bootstrap.K8sRecoveryParams{
		AllocationParams: bootstrap.AllocationParams{
			ControllerName:          info.ControllerName,
			ModelConstraints:        info.ModelConstraints,
			BootstrapConstraints:    info.ControllerConstraints,
			StoragePools:            cloneStoragePools(info.StoragePools),
			SupportedBootstrapBases: base.ControllerBases(),
		},
		AgentVersion: info.AgentVersion,
	}, nil
}

// cloneStoragePools copies the archived storage pools so allocation cannot
// mutate the validated archive summary. The bootstrap representation is shared
// by value, so each pool's attributes are copied too.
func cloneStoragePools(pools map[string]internalstorage.Attrs) map[string]internalstorage.Attrs {
	if len(pools) == 0 {
		return nil
	}
	cloned := make(map[string]internalstorage.Attrs, len(pools))
	for name, attrs := range pools {
		cloned[name] = maps.Clone(attrs)
	}
	return cloned
}

type recoveryTarget struct {
	cloud          jujucloud.Cloud
	spec           cloudspec.CloudSpec
	credentialName string

	// provider is the cloud's provider, kept so configuration building does
	// not look it up a second time.
	provider environs.EnvironProvider
}

// recoveryCredentials resolves the archived cloud against the local cloud
// definition, selected region and credentials used for allocation.
func recoveryCredentials(ctx *cmd.Context, store jujuclient.ClientStore, info *recovery.ArchiveInfo) (*recoveryTarget, error) {
	isK8s := jujucloud.CloudTypeIsCAAS(info.CloudType)
	cloud, err := common.CloudByName(info.CloudName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := info.CheckProviderFamily(cloud.Type); err != nil {
		return nil, errors.Capture(err)
	}
	provider, err := environs.Provider(info.CloudType)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if finalizer, ok := provider.(environs.CloudFinalizer); ok {
		finalized, err := finalizer.FinalizeCloud(ctx, *cloud)
		if err != nil {
			return nil, errors.Capture(err)
		}
		cloud = &finalized
	}
	if isK8s && cloud.HostCloudRegion == "" {
		return nil, errors.New("recovery cloud does not record a host cloud region; re-import it with juju add-k8s --client")
	}
	credentialArgs := modelcmd.GetCredentialsParams{Cloud: *cloud}
	if !isK8s {
		credentialArgs.CloudRegion = info.CloudRegion
	}
	credential, credentialName, regionName, _, err := common.GetOrDetectCredential(
		ctx, store, provider, credentialArgs,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	// Machine clouds pin the archived region: allocating anywhere else would
	// move the replacement controller out of the location the archive
	// records, and a region this cloud definition does not know fails here
	// rather than silently falling back. A Kubernetes cloud names regions
	// after local kubeconfig contexts, so the archived name can be stale
	// once the cloud is re-imported on this client: the locally resolved
	// region wins.
	targetRegion := info.CloudRegion
	if isK8s || targetRegion == "" {
		targetRegion = regionName
	}
	region, err := common.ChooseCloudRegion(*cloud, targetRegion)
	if err != nil {
		return nil, errors.Capture(err)
	}
	spec, err := cloudspec.MakeCloudSpec(*cloud, region.Name, credential)
	if err != nil {
		return nil, errors.Capture(err)
	}
	spec.IsControllerCloud = true
	return &recoveryTarget{
		cloud:          *cloud,
		spec:           spec,
		credentialName: credentialName,
		provider:       provider,
	}, nil
}

// recoveryConfigs builds the controller and controller model configuration the
// replacement is allocated with. Archived settings are preserved so provider
// specific values, such as an LXD project, allocate into the same target as the
// source controller; the archived identity and agent version always win over a
// stale stored value.
func recoveryConfigs(
	ctx context.Context,
	info *recovery.ArchiveInfo,
	provider environs.EnvironProvider,
) (controller.Config, *config.Config, error) {
	rawController := make(map[string]any, len(info.ControllerConfig)+1)
	for k, v := range info.ControllerConfig {
		rawController[k] = v
	}
	if info.ControllerName != "" {
		rawController[controller.ControllerName] = info.ControllerName
	}
	controllerConfig, err := controller.NewConfig(
		info.ControllerUUID, info.CACert, rawController)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	rawModel := make(map[string]any, len(info.ControllerModelConfig)+4)
	for k, v := range info.ControllerModelConfig {
		rawModel[k] = v
	}
	rawModel[config.NameKey] = bootstrap.ControllerModelName
	rawModel[config.TypeKey] = info.CloudType
	rawModel[config.UUIDKey] = info.ControllerModelUUID
	rawModel[config.AgentVersionKey] = info.AgentVersion.String()
	// The local bootstrap name is conventional; the archived UUID identifies
	// the model.
	modelConfig, err := config.New(config.UseDefaults, rawModel)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	if provider != nil {
		// Let the provider coerce and validate its own attributes (such as
		// LXD's project or EC2's vpc-id-force) via its standard Validate path.
		modelConfig, err = provider.Validate(ctx, modelConfig, nil)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
	}
	return controllerConfig, modelConfig, nil
}
