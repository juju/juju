// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/v2/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/v2/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/v2/apiserver/facades/client/charms/services"
	corecharm "github.com/juju/juju/v2/core/charm"
	"github.com/juju/juju/v2/state/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Charms", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("Charms", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("Charms", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacadeV2 provides the signature required for facade V2 registration.
// It is unknown where V1 is.
func newFacadeV2(ctx facade.Context) (*APIv2, error) {
	v4, err := newFacadeV4(ctx)
	if err != nil {
		return nil, nil
	}
	return &APIv2{
		APIv3: &APIv3{
			API: v4,
		},
	}, nil
}

// newFacadeV3 provides the signature required for facade V3 registration.
func newFacadeV3(ctx facade.Context) (*APIv3, error) {
	api, err := newFacadeV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{API: api}, nil
}

// newFacadeV4 provides the signature required for facade V4 registration.
func newFacadeV4(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	commonState := &charmscommon.StateShim{st}
	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		charmInfoAPI: charmInfoAPI,
		authorizer:   authorizer,
		backendState: newStateShim(st),
		backendModel: m,
		newStorage: func(modelUUID string) services.Storage {
			return storage.NewStorage(modelUUID, st.MongoSession())
		},
		newRepoFactory: func(cfg services.CharmRepoFactoryConfig) corecharm.RepositoryFactory {
			return services.NewCharmRepoFactory(cfg)
		},
		newDownloader: func(cfg services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
		tag:             m.ModelTag(),
		requestRecorder: ctx.RequestRecorder(),
	}, nil
}
