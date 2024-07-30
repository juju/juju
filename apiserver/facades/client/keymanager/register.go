// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("KeyManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		facade, err := makeFacadeV1(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot make keymanager facade: %w", err)
		}
		return facade, nil
	}, reflect.TypeOf((*KeyManagerAPI)(nil)))
}

func makeFacadeV1(stdCtx context.Context, ctx facade.ModelContext) (*KeyManagerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	model, err := ctx.ServiceFactory().ModelInfo().GetModelInfo(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("retrieving model info: %w", err)
	}

	cfg, err := ctx.ServiceFactory().ControllerConfig().ControllerConfig(stdCtx)
	if err != nil {
		return nil, fmt.Errorf("retrieving controller config: %w", err)
	}

	authedUser := ctx.Auth().GetAuthTag()

	keyImporterHTTPClient, err := ctx.HTTPClient(facade.HTTPClientPurposeUserSSHImport)
	if err != nil {
		return nil, fmt.Errorf(
			"getting key importer http client: %w",
			err,
		)
	}

	keyImporter := sshimporter.NewImporter(keyImporterHTTPClient)
	return newKeyManagerAPI(
		ctx.ServiceFactory().KeyManager(keyImporter),
		ctx.ServiceFactory().Access(),
		authorizer,
		common.NewBlockChecker(ctx.State()),
		cfg.ControllerUUID(),
		model.UUID,
		authedUser,
	), nil
}

func newKeyManagerAPI(
	keyManagerService KeyManagerService,
	userService UserService,
	authorizer facade.Authorizer,
	check BlockChecker,
	controllerUUID string,
	modelID coremodel.UUID,
	authedUser names.Tag,
) *KeyManagerAPI {
	return &KeyManagerAPI{
		keyManagerService: keyManagerService,
		userService:       userService,
		authorizer:        authorizer,
		check:             check,
		controllerUUID:    controllerUUID,
		modelID:           modelID,
		authedUser:        authedUser,
	}
}

type BlockChecker interface {
	ChangeAllowed(context.Context) error
	RemoveAllowed(context.Context) error
}
