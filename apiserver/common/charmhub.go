// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/state"
)

// createCharmhubClient creates a new charmhub Client based on this model's
// config.
func CharmhubClient(st *state.State, logger loggo.Logger) (*charmhub.Client, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, _ := modelConfig.CharmHubURL()
	config, err := charmhub.CharmHubConfigFromURL(url, logger.Child("charmhub"))
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := charmhub.NewClient(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
