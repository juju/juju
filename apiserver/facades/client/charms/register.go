// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/state/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Charms", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*API)(nil)))
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
