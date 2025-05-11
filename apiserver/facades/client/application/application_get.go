// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/rpc/params"
)

// Get returns the charm configuration for an application.
func (api *APIBase) getConfig(
	ctx context.Context,
	args params.ApplicationGet,
	describe func(applicationConfig config.ConfigAttributes, charmConfig charm.Config) map[string]interface{},
) (params.ApplicationGetResults, error) {
	// TODO (stickupkid): This should be one call to the application service.
	// There is no reason to split all these calls into multiple DB calls.
	// Once application service is refactored to return the merged config, this
	// should be a single call.

	appID, err := api.applicationService.GetApplicationIDByName(ctx, args.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ApplicationGetResults{}, errors.NotFoundf("application %s", args.ApplicationName)
	} else if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}

	appInfo, err := api.applicationService.GetApplicationAndCharmConfig(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ApplicationGetResults{}, errors.NotFoundf("application %s", args.ApplicationName)
	} else if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}
	mergedCharmConfig := describe(appInfo.ApplicationConfig, appInfo.CharmConfig)

	appSettings := config.ConfigAttributes{
		coreapplication.TrustConfigOptionName: appInfo.Trust,
	}

	providerSchema, providerDefaults, err := ConfigSchema()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	appConfigInfo := describeAppConfig(appSettings, providerSchema, providerDefaults)

	isSubordinate, err := api.applicationService.IsSubordinateApplication(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ApplicationGetResults{}, errors.NotFoundf("application %s", args.ApplicationName)
	} else if err != nil {
		return params.ApplicationGetResults{}, errors.Trace(err)
	}
	var cons constraints.Value
	if !isSubordinate {
		cons, err = api.applicationService.GetApplicationConstraints(ctx, appID)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return params.ApplicationGetResults{}, errors.NotFoundf("application %s", args.ApplicationName)
		} else if err != nil {
			return params.ApplicationGetResults{}, errors.Trace(err)
		}
	}

	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.ApplicationGetResults{}, err
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
		Charm:             appInfo.CharmName,
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

func (api *APIBase) getCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error) {
	locator, err := api.applicationService.GetCharmLocatorByApplicationName(ctx, name)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return applicationcharm.CharmLocator{}, errors.NotFoundf("application %q", name)
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return applicationcharm.CharmLocator{}, errors.NotFoundf("charm for application %q", name)
	} else if err != nil {
		return applicationcharm.CharmLocator{}, errors.Annotate(err, "getting charm id for application")
	}
	return locator, nil
}

func (api *APIBase) getCharmName(ctx context.Context, locator applicationcharm.CharmLocator) (string, error) {
	name, err := api.applicationService.GetCharmMetadataName(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return "", errors.NotFoundf("charm")
	} else if err != nil {
		return "", errors.Annotate(err, "getting charm for application")
	}
	return name, nil
}

func (api *APIBase) getCharm(ctx context.Context, locator applicationcharm.CharmLocator) (Charm, error) {
	charm, resLocator, _, err := api.applicationService.GetCharm(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm %q", locator.Name)
	} else if errors.Is(err, applicationerrors.CharmNameNotValid) {
		return nil, errors.NotValidf("charm %q", locator.Name)
	} else if errors.Is(err, applicationerrors.CharmSourceNotValid) {
		return nil, errors.NotValidf("charm %q", locator.Name)
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm for application")
	}

	available, err := api.applicationService.IsCharmAvailable(ctx, resLocator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm availability for application")
	}

	return &domainCharm{
		charm:     charm,
		locator:   resLocator,
		available: available,
	}, nil
}

func (api *APIBase) getCharmMetadata(ctx context.Context, locator applicationcharm.CharmLocator) (*charm.Meta, error) {
	metadata, err := api.applicationService.GetCharmMetadata(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm")
	} else if err != nil {
		return nil, errors.Annotate(err, "getting charm metadata for application")
	}

	return &metadata, nil
}

func (api *APIBase) getMergedAppAndCharmConfig(ctx context.Context, appName string) (map[string]interface{}, error) {
	// TODO (stickupkid): This should be one call to the application service.
	// Thee application service should return the merged config, this should
	// not happen at the API server level.
	appID, err := api.applicationService.GetApplicationIDByName(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	appInfo, err := api.applicationService.GetApplicationAndCharmConfig(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return describe(appInfo.ApplicationConfig, appInfo.CharmConfig), nil
}

func describeAppConfig(
	appConfig config.ConfigAttributes,
	schemaFields configschema.Fields,
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

func describe(settings config.ConfigAttributes, config charm.Config) map[string]interface{} {
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
	schema := charm.Local.String()
	if c.locator.Source == applicationcharm.CharmHubSource {
		schema = charm.CharmHub.String()
	}

	name := c.charm.Meta().Name
	if name == "" {
		panic(fmt.Sprintf("charm name is empty %+v", c.charm))
	}

	var a string
	switch c.locator.Architecture {
	case architecture.AMD64:
		a = arch.AMD64
	case architecture.ARM64:
		a = arch.ARM64
	case architecture.PPC64EL:
		a = arch.PPC64EL
	case architecture.S390X:
		a = arch.S390X
	case architecture.RISCV64:
		a = arch.RISCV64
	default:
		// If there is no architecture set, we should ignore it.
	}

	curl := &charm.URL{
		Schema:       schema,
		Name:         name,
		Revision:     c.locator.Revision,
		Architecture: a,
	}
	return curl.String()
}

func (c *domainCharm) Version() string {
	return c.charm.Version()
}

func ptr[T any](v T) *T {
	return &v
}
