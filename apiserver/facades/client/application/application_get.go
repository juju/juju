// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// Get returns the charm configuration for an application.
func (api *APIBase) getConfig(
	ctx context.Context,
	args params.ApplicationGet,
	describe func(settings charm.Settings, config *charm.Config) map[string]interface{},
) (params.ApplicationGetResults, error) {
	// TODO (stickupkid): This should be one call to the application service.
	// There is no reason to split all these calls into multiple DB calls.
	// Once application service is refactored to return the merged config, this
	// should be a single call.

	charmID, err := api.getCharmIDByApplicationName(ctx, args.ApplicationName)
	if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}

	charmName, err := api.getCharmName(ctx, charmID)
	if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}

	charmConfig, err := api.getCharmConfig(ctx, charmID)
	if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}

	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	settings, err := app.CharmConfig()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}

	mergedCharmConfig := describe(settings, charmConfig)

	appConfig, err := app.ApplicationConfig()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}

	providerSchema, providerDefaults, err := ConfigSchema()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	appConfigInfo := describeAppConfig(appConfig, providerSchema, providerDefaults)
	var cons constraints.Value
	if app.IsPrincipal() {
		cons, err = app.Constraints()
		if err != nil {
			return params.ApplicationGetResults{}, err
		}
	}
	endpoints, err := app.EndpointBindings()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}

	allSpaceInfosLookup, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return params.ApplicationGetResults{}, apiservererrors.ServerError(err)
	}

	bindingMap, err := endpoints.MapWithSpaceNames(allSpaceInfosLookup)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}

	var appChannel string

	// If the applications charm origin is from charm-hub, then build the real
	// channel and send that back.
	origin := app.CharmOrigin()
	if corecharm.CharmHub.Matches(origin.Source) && origin.Channel != nil {
		ch := charm.MakePermissiveChannel(origin.Channel.Track, origin.Channel.Risk, origin.Channel.Branch)
		appChannel = ch.String()
	}

	base, err := corebase.ParseBase(origin.Platform.OS, origin.Platform.Channel)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	return params.ApplicationGetResults{
		Application:       args.ApplicationName,
		Charm:             charmName,
		CharmConfig:       mergedCharmConfig,
		ApplicationConfig: appConfigInfo,
		Constraints:       cons,
		Base: params.Base{
			Name:    base.OS,
			Channel: base.Channel.String(),
		},
		Channel:          appChannel,
		EndpointBindings: bindingMap,
	}, nil
}

func (api *APIBase) getCharmID(ctx context.Context, charmURL string) (corecharm.ID, error) {
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return "", errors.Annotate(err, "parsing charm URL")
	}

	charmSource, err := applicationcharm.ParseCharmSchema(charm.Schema(curl.Schema))
	if err != nil {
		return "", errors.Trace(err)
	}
	charmID, err := api.applicationService.GetCharmID(ctx, applicationcharm.GetCharmArgs{
		Name:     curl.Name,
		Revision: ptr(curl.Revision),
		Source:   charmSource,
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return "", errors.NotFoundf("charm %q", charmURL)
	} else if errors.Is(err, applicationerrors.CharmNameNotValid) {
		return "", errors.NotValidf("charm %q", charmURL)
	} else if errors.Is(err, applicationerrors.CharmSourceNotValid) {
		return "", errors.NotValidf("charm %q", charmURL)
	} else if err != nil {
		return "", errors.Annotate(err, "getting charm id")
	}
	return charmID, nil
}

func (api *APIBase) getCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error) {
	charmID, err := api.applicationService.GetCharmIDByApplicationName(ctx, name)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return "", errors.NotFoundf("application %q", name)
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return "", errors.NotFoundf("charm for application %q", name)
	} else if err != nil {
		return "", errors.Annotate(err, "getting charm id for application")
	}
	return charmID, nil
}

func (api *APIBase) getCharmName(ctx context.Context, charmID corecharm.ID) (string, error) {
	name, err := api.applicationService.GetCharmMetadataName(ctx, charmID)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return "", errors.NotFoundf("charm")
	} else if err != nil {
		return "", errors.Annotate(err, "getting charm for application")
	}
	return name, nil
}

func (api *APIBase) getCharm(ctx context.Context, charmID corecharm.ID) (Charm, error) {
	charm, locator, err := api.applicationService.GetCharm(ctx, charmID)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm for application")
	}

	available, err := api.applicationService.IsCharmAvailable(ctx, charmID)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm availability for application")
	}

	return &domainCharm{
		charm:     charm,
		locator:   locator,
		available: available,
	}, nil
}

func (api *APIBase) getCharmMetadata(ctx context.Context, charmID corecharm.ID) (*charm.Meta, error) {
	metadata, err := api.applicationService.GetCharmMetadata(ctx, charmID)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm metadata for application")
	}

	return &metadata, nil
}

func (api *APIBase) getCharmConfig(ctx context.Context, charmID corecharm.ID) (*charm.Config, error) {
	config, err := api.applicationService.GetCharmConfig(ctx, charmID)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm config for application")
	}

	return &config, nil
}

func (api *APIBase) getMergedAppAndCharmConfig(ctx context.Context, appName string) (map[string]interface{}, error) {
	// TODO (stickupkid): This should be one call to the application service.
	// Thee application service should return the merged config, this should
	// not happen at the API server level.
	app, err := api.backend.Application(appName)
	if err != nil {
		return nil, err
	}
	settings, err := app.CharmConfig()
	if err != nil {
		return nil, err
	}

	charmID, err := api.getCharmIDByApplicationName(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmConfig, err := api.getCharmConfig(ctx, charmID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return describe(settings, charmConfig), nil
}

func describeAppConfig(
	appConfig config.ConfigAttributes,
	schemaFields environschema.Fields,
	defaults schema.Defaults,
) map[string]interface{} {
	results := make(map[string]interface{})
	for name, field := range schemaFields {
		defaultValue := defaults[name]
		info := map[string]interface{}{
			"description": field.Description,
			"type":        field.Type,
			"source":      "unset",
		}
		if defaultValue == schema.Omit {
			results[name] = info
			continue
		}
		set := false
		if value := appConfig[name]; value != nil && defaultValue != value {
			set = true
			info["value"] = value
			info["source"] = "user"
		}
		if defaultValue != nil {
			info["default"] = defaultValue
			if !set {
				info["value"] = defaultValue
				info["source"] = "default"
			}
		}
		results[name] = info
	}
	return results
}

func describe(settings charm.Settings, config *charm.Config) map[string]interface{} {
	results := make(map[string]interface{})
	for name, option := range config.Options {
		info := map[string]interface{}{
			"description": option.Description,
			"type":        option.Type,
			"source":      "unset",
		}
		set := false
		if value := settings[name]; value != nil && option.Default != value {
			set = true
			info["value"] = value
			info["source"] = "user"
		}
		if option.Default != nil {
			info["default"] = option.Default
			if !set {
				info["value"] = option.Default
				info["source"] = "default"
			}
		}
		results[name] = info
	}
	return results
}

type domainCharm struct {
	charm     charm.Charm
	locator   applicationcharm.CharmLocator
	available bool
}

func (c *domainCharm) Manifest() *charm.Manifest {
	return c.charm.Manifest()
}

func (c *domainCharm) Meta() *charm.Meta {
	return c.charm.Meta()
}

func (c *domainCharm) Config() *charm.Config {
	return c.charm.Config()
}

func (c *domainCharm) Actions() *charm.Actions {
	return c.charm.Actions()
}

func (c *domainCharm) Revision() int {
	return c.locator.Revision
}

func (c *domainCharm) IsUploaded() bool {
	return c.available
}

func (c *domainCharm) URL() string {
	schema := "local"
	if c.locator.Source == applicationcharm.CharmHubSource {
		schema = "ch"
	}

	name := c.charm.Meta().Name
	if name == "" {
		panic(fmt.Sprintf("charm name is empty %+v", c.charm))
	}

	var arch string
	switch c.locator.Architecture {
	case architecture.AMD64:
		arch = "amd64"
	case architecture.ARM64:
		arch = "arm64"
	case architecture.PPC64EL:
		arch = "ppc64el"
	case architecture.S390X:
		arch = "s390x"
	case architecture.RISV64:
		arch = "risv64"
	default:
		// If there is no architecture set, we should ignore it.
	}

	curl := &charm.URL{
		Schema:       schema,
		Name:         name,
		Revision:     c.locator.Revision,
		Architecture: arch,
	}
	return curl.String()
}

func ptr[T any](v T) *T {
	return &v
}
