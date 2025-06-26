// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("CrossModelSecrets", 1, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return makeStateCrossModelSecretsAPIV1(stdCtx, ctx)
	}, reflect.TypeOf((*CrossModelSecretsAPIV1)(nil)))
	registry.MustRegisterForMultiModel("CrossModelSecrets", 2, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		api, err := makeStateCrossModelSecretsAPI(stdCtx, ctx)
		return api, fmt.Errorf("creating CrossModelSecrets facade: %w", err)
	}, reflect.TypeOf((*CrossModelSecretsAPI)(nil)))
}

// makeStateCrossModelSecretsAPIV1 creates a new server-side CrossModelSecrets V1 API facade.
func makeStateCrossModelSecretsAPIV1(stdCtx context.Context, ctx facade.MultiModelContext) (*CrossModelSecretsAPIV1, error) {
	api, err := makeStateCrossModelSecretsAPI(stdCtx, ctx)
	if err != nil {
		return nil, fmt.Errorf("creating CrossModelSecrets V1 facade: %w", err)
	}
	return &CrossModelSecretsAPIV1{CrossModelSecretsAPI: api}, nil
}

// makeStateCrossModelSecretsAPI creates a new server-side CrossModelSecrets API facade
// backed by global state.
func makeStateCrossModelSecretsAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*CrossModelSecretsAPI, error) {
	return NewCrossModelSecretsAPI()
}
