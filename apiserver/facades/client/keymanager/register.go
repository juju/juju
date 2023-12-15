// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("KeyManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*KeyManagerAPI)(nil)))
}

func newFacadeV1(ctx facade.Context) (*KeyManagerAPI, error) {
	// Only clients can access the key manager service.
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newKeyManagerAPI(
		m,
		authorizer,
		common.NewBlockChecker(st),
		st.ControllerTag(),
	), nil
}

func newKeyManagerAPI(
	model Model,
	authorizer facade.Authorizer,
	check BlockChecker,
	controllerTag names.ControllerTag,
) *KeyManagerAPI {
	return &KeyManagerAPI{
		model:         model,
		authorizer:    authorizer,
		check:         check,
		controllerTag: controllerTag,
	}
}

type Model interface {
	ModelTag() names.ModelTag
	ModelConfig() (*config.Config, error)
	UpdateModelConfig(map[string]interface{}, []string, ...state.ValidateConfigFunc) error
}

type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}
