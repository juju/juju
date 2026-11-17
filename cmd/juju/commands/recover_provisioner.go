// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"time"

	"github.com/juju/juju/api/jujuclient"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/tools"
)

// recoveryConfigs builds the controller and controller-model
// configuration for a replacement controller from the recovery archive.
// The archived identity and CA material are preserved so that surviving
// agents keep trusting the replacement controller.
func recoveryConfigs(info *recovery.ArchiveInfo) (controller.Config, *config.Config, error) {
	controllerConfig, err := controller.NewConfig(
		info.ControllerUUID,
		info.CACert,
		map[string]any{controller.ControllerName: info.ControllerName},
	)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	modelConfig, err := config.New(config.UseDefaults, map[string]any{
		"name":                 bootstrap.ControllerModelName,
		"type":                 info.CloudType,
		config.UUIDKey:         info.ControllerModelUUID,
		config.AgentVersionKey: info.AgentVersion.String(),
	})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	return controllerConfig, modelConfig, nil
}

// provisionRecovery allocates a replacement controller machine from the
// recovery archive, through the ordinary bootstrap allocation pipeline, and
// deliberately stops before installing an agent or initialising its
// database.
func provisionRecovery(
	ctx *cmd.Context,
	store jujuclient.ClientStore,
	info *recovery.ArchiveInfo,
) error {
	if info.SourceBase == "" {
		return errors.New("archive does not record a usable controller base")
	}
	sourceBase, err := base.ParseBaseFromString(info.SourceBase)
	if err != nil {
		return errors.Errorf("archive does not record a usable controller base: %w", err)
	}

	cloud, err := common.CloudByName(info.CloudName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := info.CheckProviderFamily(cloud.Type); err != nil {
		return errors.Capture(err)
	}
	if jujucloud.CloudIsCAAS(*cloud) {
		return errors.Errorf("Kubernetes recovery provisioning %w", coreerrors.NotSupported)
	}
	provider, err := environs.Provider(info.CloudType)
	if err != nil {
		return errors.Capture(err)
	}
	if finalizer, ok := provider.(environs.CloudFinalizer); ok {
		finalizedCloud, err := finalizer.FinalizeCloud(ctx, *cloud)
		if err != nil {
			return errors.Capture(err)
		}
		cloud = &finalizedCloud
	}
	credential, credentialName, regionName, _, err := common.GetOrDetectCredential(
		ctx, store, provider, modelcmd.GetCredentialsParams{Cloud: *cloud},
	)
	if err != nil {
		return errors.Capture(err)
	}
	region, err := common.ChooseCloudRegion(*cloud, regionName)
	if err != nil {
		return errors.Capture(err)
	}
	spec, err := cloudspec.MakeCloudSpec(*cloud, region.Name, credential)
	if err != nil {
		return errors.Capture(err)
	}
	spec.IsControllerCloud = true

	controllerConfig, modelConfig, err := recoveryConfigs(info)
	if err != nil {
		return errors.Capture(err)
	}

	stdCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	provisionCtx := environscmd.BootstrapContext(stdCtx, ctx)

	adminPassword, err := password.RandomPassword()
	if err != nil {
		return errors.Capture(err)
	}
	preparation, err := bootstrap.OpenController(false, provisionCtx, bootstrap.PrepareParams{
		ModelConfig:      modelConfig.AllAttrs(),
		ControllerConfig: controllerConfig,
		ControllerName:   info.ControllerName,
		Cloud:            spec,
		CredentialName:   credentialName,
		AdminSecret:      adminPassword,
	})
	if err != nil {
		return errors.Capture(err)
	}
	if err := preparation.Environ.PrepareForBootstrap(provisionCtx, info.ControllerName); err != nil {
		return errors.Capture(err)
	}

	hostArch := arch.HostArch()
	var started instance.Id
	if err := bootstrap.Recover(provisionCtx, preparation.Environ, bootstrap.RecoveryParams{
		ControllerName:          info.ControllerName,
		Cloud:                   *cloud,
		CloudRegion:             region.Name,
		CloudCredentialName:     credentialName,
		CloudCredential:         credential,
		ControllerConfig:        controllerConfig,
		AdminSecret:             adminPassword,
		CAPrivateKey:            info.CAPrivateKey,
		AgentVersion:            &info.AgentVersion,
		BootstrapConstraints:    constraints.Value{Arch: &hostArch},
		BootstrapBase:           sourceBase,
		SupportedBootstrapBases: []base.Base{sourceBase},
		AvailableTools: tools.List{&tools.Tools{
			Version: semversion.Binary{
				Number:  info.AgentVersion,
				Release: coreos.HostOSTypeName(),
				Arch:    hostArch,
			},
		}},
		InstanceStarted: func(id instance.Id) {
			started = id
		},
	}); err != nil {
		return errors.Capture(err)
	}
	return errors.Errorf(
		"controller recovery initialisation is not available in this build; provisioned replacement instance %s",
		started,
	)
}
