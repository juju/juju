// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/model"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	credentialservice "github.com/juju/juju/domain/credential/service"
	modelservice "github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASOperatorUpgrader", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateCAASOperatorUpgraderAPI(stdCtx, ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newStateCAASOperatorUpgraderAPI provides the signature required for facade registration.
func newStateCAASOperatorUpgraderAPI(stdCtx context.Context, ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	domainServices := ctx.DomainServices()
	modelInfoService := domainServices.ModelInfo()
	cloudService := domainServices.Cloud()
	credentialService := domainServices.Credential()
	modelConfigService := domainServices.Config()

	cloudSpec, err := CloudSpecForModel(stdCtx, modelInfoService, cloudService, credentialService)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := modelConfigService.ModelConfig(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := caas.New(stdCtx, environs.OpenParams{
		ControllerUUID: modelInfo.ControllerUUID.String(),
		Cloud:          cloudSpec,
		Config:         cfg,
	}, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	return NewCAASOperatorUpgraderAPI(authorizer, broker, ctx.Logger().Child("caasoperatorupgrader"))
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(
	ctx context.Context,
	modelInfoService *modelservice.ProviderModelService,
	cloudService *cloudservice.WatchableService,
	credentialService *credentialservice.WatchableService,
) (environscloudspec.CloudSpec, error) {
	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	cld, err := cloudService.Cloud(ctx, modelInfo.Cloud)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := modelInfo.CloudRegion
	credentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: model.ControllerModelOwnerUsername,
		Name:  modelInfo.CredentialName,
	}
	cred, err := credentialService.CloudCredential(ctx, credentialKey)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	return environscloudspec.MakeCloudSpec(*cld, regionName, &cred)
}
