// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"maps"
	"os"
	"os/signal"
	"time"

	"github.com/juju/juju/api/jujuclient"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// recoveryAllocationTimeout bounds provider preparation and machine allocation
// after the client has resolved the cloud and credentials.
const recoveryAllocationTimeout = 20 * time.Minute

// provisionRecovery allocates a replacement controller machine from the
// recovery archive, through the ordinary bootstrap allocation pipeline, and
// deliberately stops before installing an agent or initialising its
// database.
func provisionRecovery(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	info *recovery.ArchiveInfo,
) error {
	// TODO(recovery): dispatch Kubernetes archives to namespace allocation
	// before looking for metadata that only machine controllers record.
	if jujucloud.CloudTypeIsCAAS(info.CloudType) {
		return errors.Errorf("Kubernetes recovery provisioning %w", coreerrors.NotSupported)
	}
	params, err := recoveryParamsFromArchive(info)
	if err != nil {
		return errors.Capture(err)
	}

	credentials, err := recoveryCredentials(ctx, store, info)
	if err != nil {
		return errors.Capture(err)
	}

	controllerConfig, modelConfig, err := recoveryConfigs(ctx, info, credentials.provider)
	if err != nil {
		return errors.Capture(err)
	}

	// Let allocation unwind on Ctrl-C so it can report an instance that
	// may need manual removal.
	interruptCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	stdCtx, cancel := context.WithTimeout(interruptCtx, recoveryAllocationTimeout)
	defer cancel()
	provisionCtx := environscmd.BootstrapContext(stdCtx, ctx)

	environ, err := bootstrap.OpenController(false, provisionCtx, environs.OpenParams{
		Config:         modelConfig,
		ControllerUUID: info.ControllerUUID,
		Cloud:          credentials.spec,
	})
	if err != nil {
		return errors.Capture(err)
	}
	if err := environ.PrepareForBootstrap(provisionCtx, info.ControllerName); err != nil {
		return errors.Capture(err)
	}

	params.Cloud = credentials.cloud
	params.CloudRegion = credentials.spec.Region
	params.CloudCredential = credentials.spec.Credential
	params.ControllerConfig = controllerConfig
	started, err := bootstrap.Recover(provisionCtx, environ, params)
	if err != nil {
		return errors.Capture(err)
	}
	// TODO(recovery): return success once finalisation and registration finish.
	return errors.Errorf(
		"controller recovery initialisation is not available in this build; provisioned replacement instance %s",
		started,
	)
}

// recoveryParamsFromArchive prepares the machine-specific inputs before cloud
// access. The client can run on a different platform from the controller.
func recoveryParamsFromArchive(info *recovery.ArchiveInfo) (bootstrap.RecoveryParams, error) {
	if info.SourceBase == "" {
		return bootstrap.RecoveryParams{}, errors.New("archive does not record a usable controller base")
	}
	targetBase, err := base.ParseBaseFromString(info.SourceBase)
	if err != nil {
		return bootstrap.RecoveryParams{}, errors.Errorf("archive does not record a usable controller base: %w", err)
	}
	targetArch := arch.NormaliseArch(info.SourceArch)
	if !arch.IsSupportedArch(targetArch) {
		return bootstrap.RecoveryParams{}, errors.Errorf("archive does not record a usable controller architecture: %q", info.SourceArch)
	}
	// The archived architecture is authoritative: allocation pins it over
	// whatever the recorded constraint says.
	bootstrapConstraints := info.ControllerConstraints
	bootstrapConstraints.Arch = &targetArch
	return bootstrap.RecoveryParams{
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
	cloud jujucloud.Cloud
	spec  cloudspec.CloudSpec

	// provider is the cloud's provider, kept so configuration building does
	// not look it up a second time.
	provider environs.EnvironProvider
}

// recoveryCredentials resolves the archived cloud against the local cloud
// definition, selected region and credentials used for allocation.
func recoveryCredentials(ctx *cmd.Context, store jujuclient.ClientStore, info *recovery.ArchiveInfo) (*recoveryTarget, error) {
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
	credential, _, regionName, _, err := common.GetOrDetectCredential(
		ctx, store, provider, modelcmd.GetCredentialsParams{
			Cloud:       *cloud,
			CloudRegion: info.CloudRegion,
		},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	// The archived region is the one the controller model lives in:
	// allocating anywhere else would move the replacement controller out of
	// the location the archive records. A region this cloud definition does
	// not know fails here rather than silently falling back.
	targetRegion := info.CloudRegion
	if targetRegion == "" {
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
	return &recoveryTarget{cloud: *cloud, spec: spec, provider: provider}, nil
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
