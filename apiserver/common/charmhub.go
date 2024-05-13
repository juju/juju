// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charmhub"
)

// ModelGetter defines an interface for getting a model.
type ModelGetter interface {
	Model() (ConfigModel, error)
}

// ConfigModel defines an interface for getting the config of a model.
type ConfigModel interface {
	Config() (*config.Config, error)
}

// CharmhubClient creates a new charmhub Client based on this model's config.
func CharmhubClient(mg ModelGetter, httpClient charmhub.HTTPClient, logger corelogger.Logger) (*charmhub.Client, error) {
	model, err := mg.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, _ := modelConfig.CharmHubURL()

	client, err := charmhub.NewClient(charmhub.Config{
		URL:        url,
		HTTPClient: httpClient,
		Logger:     logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
