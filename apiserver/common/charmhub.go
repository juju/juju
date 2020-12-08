// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/state"
)

// createCharmhubClient creates a new charmhub Client based on this model's
// config.
func CharmhubClient(st *state.State, logger loggo.Logger, metadata map[string]string) (*charmhub.Client, error) {
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

	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		config.Headers.Add(charmhub.MetadataHeader, k+"="+metadata[k])
	}

	client, err := charmhub.NewClient(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
