// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/utils"
)

const (
	// DefaultArchitecture defines the architecture for a charm. We currently
	// only support all. This will change in the future.
	DefaultArchitecture = "all"
)

// Action represents the type of refresh is performed.
type Action string

const (
	// InstallAction defines a install action.
	InstallAction Action = "install"

	// RefreshAction defines a refresh action.
	RefreshAction Action = "refresh"

	// RefreshAllAction defines a refresh all action.
	RefreshAllAction Action = "refresh-all"
)

// RefreshClient defines a client for refresh requests.
type RefreshClient struct {
	path   path.Path
	client RESTClient
}

// NewRefreshClient creates a RefreshClient for requesting
func NewRefreshClient(path path.Path, client RESTClient) *RefreshClient {
	return &RefreshClient{
		path:   path,
		client: client,
	}
}

// RefreshConfig defines a type for building refresh requests.
type RefreshConfig interface {
	// Build a refresh request for sending to the API.
	Build() (transport.RefreshRequest, error)

	// Ensure that the request back contains the information we requested.
	Ensure([]transport.RefreshResponse) error
}

// Refresh is used to refresh installed charms to a more suitable revision.
func (c *RefreshClient) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	req, err := config.Build()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var resp transport.RefreshResponses
	if err := c.client.Post(ctx, c.path, req, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	if len(resp.ErrorList) > 0 {
		var combined []string
		for _, err := range resp.ErrorList {
			if err.Message != "" {
				combined = append(combined, err.Message)
			}
		}
		return nil, errors.Errorf(strings.Join(combined, "\n"))
	}

	return resp.Results, config.Ensure(resp.Results)
}

// refreshOne holds the config for making refresh calls to the charmhub API.
type refreshOne struct {
	ID       string
	Revision int
	Channel  string
	OS       string
	Series   string
	// instanceKey is a private unique key that we construct for charmhub API
	// asynchronous calls.
	instanceKey string
}

// RefreshOne creates a request config for requesting only one charm.
func RefreshOne(id string, revision int, channel, os, series string) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return refreshOne{
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    revision,
		Channel:     channel,
		OS:          os,
		Series:      series,
	}, nil
}

// Build a refresh request that can be past to the API.
func (c refreshOne) Build() (transport.RefreshRequest, error) {
	return transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey: c.instanceKey,
			ID:          c.ID,
			Revision:    c.Revision,
			Platform: transport.RefreshRequestPlatform{
				OS:           c.OS,
				Series:       c.Series,
				Architecture: DefaultArchitecture,
			},
			TrackingChannel: c.Channel,
			// TODO (stickupkid): We need to model the refreshed date. It's
			// currently optional, but will be required at some point. This
			// is the installed date of the charm on the system.
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(RefreshAction),
			InstanceKey: c.instanceKey,
			ID:          c.ID,
		}},
	}, nil
}

// Ensure that the request back contains the information we requested.
func (c refreshOne) Ensure(responses []transport.RefreshResponse) error {
	for _, resp := range responses {
		if resp.InstanceKey == c.instanceKey {
			return nil
		}
	}
	return errors.NotValidf("refresh action key")
}

type installOne struct {
	ID       string
	Revision *int
	Channel  *string
	OS       string
	Series   string
	// instanceKey is a private unique key that we construct for charmhub API
	// asynchronous calls.
	instanceKey string
}

// InstallOne creates a request config for requesting only one charm.
func InstallOne(id string, revision int, channel, os, series string) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return installOne{
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		Channel:     &channel,
		OS:          os,
		Series:      series,
	}, nil
}

// InstallOneFromRevision creates a request config for requesting only one
// charm.
func InstallOneFromRevision(id string, revision int, os, series string) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return installOne{
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		OS:          os,
		Series:      series,
	}, nil
}

// InstallOneFromChannel creates a request config for requesting only one charm.
func InstallOneFromChannel(id string, channel, os, series string) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return installOne{
		instanceKey: uuid.String(),
		ID:          id,
		Channel:     &channel,
		OS:          os,
		Series:      series,
	}, nil
}

// Build a refresh request that can be past to the API.
func (c installOne) Build() (transport.RefreshRequest, error) {
	return transport.RefreshRequest{
		// Context is required here, even
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(InstallAction),
			InstanceKey: c.instanceKey,
			ID:          c.ID,
			Revision:    c.Revision,
			Channel:     c.Channel,
			Platform: &transport.RefreshRequestPlatform{
				OS:           c.OS,
				Series:       c.Series,
				Architecture: DefaultArchitecture,
			},
		}},
	}, nil
}

// Ensure that the request back contains the information we requested.
func (c installOne) Ensure(responses []transport.RefreshResponse) error {
	for _, resp := range responses {
		if resp.InstanceKey == c.instanceKey {
			return nil
		}
	}
	return errors.NotValidf("install action key")
}

type refreshMany struct {
	Configs []RefreshConfig
}

// RefreshMany will compose many refresh configs.
func RefreshMany(configs ...RefreshConfig) RefreshConfig {
	return refreshMany{
		Configs: configs,
	}
}

// Build a refresh request that can be past to the API.
func (c refreshMany) Build() (transport.RefreshRequest, error) {
	var result transport.RefreshRequest
	for _, config := range c.Configs {
		req, err := config.Build()
		if err != nil {
			return transport.RefreshRequest{}, errors.Trace(err)
		}
		result.Context = append(result.Context, req.Context...)
		result.Actions = append(result.Actions, req.Actions...)
	}
	return result, nil
}

// Ensure that the request back contains the information we requested.
func (c refreshMany) Ensure(responses []transport.RefreshResponse) error {
	for _, config := range c.Configs {
		if err := config.Ensure(responses); err != nil {
			return errors.Annotatef(err, "missing response")
		}
	}
	return nil
}
