// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/secrets"
	"github.com/juju/juju/v3/secrets/provider"
	"github.com/juju/juju/v3/secrets/provider/juju"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newSecretManagerAPI(ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// newSecretManagerAPI creates a SecretsManagerAPI.
func newSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() && !context.Auth().AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// Work out the app name associated with the agent since this is
	// the secret owner for newly created secrets.
	agentTag := context.Auth().GetAuthTag()
	agentName := agentTag.Id()
	if agentTag.Kind() == names.UnitTagKind {
		agentName, _ = names.UnitApplication(agentName)
	}

	// For now we just support the Juju secrets provider.
	service, err := provider.NewSecretProvider(juju.Provider, secrets.ProviderConfig{
		juju.ParamBackend: context.State(),
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating juju secrets service")
	}
	return &SecretsManagerAPI{
		authOwner:       names.NewApplicationTag(agentName),
		controllerUUID:  context.State().ControllerUUID(),
		modelUUID:       context.State().ModelUUID(),
		secretsService:  service,
		resources:       context.Resources(),
		secretsRotation: context.State(),
		accessSecret:    secretAccessor(agentName),
	}, nil
}
