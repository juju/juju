// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v2"
	"github.com/kr/pretty"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

// Action represents the type of refresh is performed.
type Action string

const (
	// InstallAction defines a install action.
	InstallAction Action = "install"

	// DownloadAction defines a download action.
	DownloadAction Action = "download"

	// RefreshAction defines a refresh action.
	RefreshAction Action = "refresh"
)

// RefreshPlatform defines a platform refreshing charms.
type RefreshPlatform struct {
	Architecture string
	OS           string
	Series       string
}

func (p RefreshPlatform) String() string {
	path := p.Architecture
	if p.Series != "" {
		if p.OS != "" {
			path = fmt.Sprintf("%s/%s", path, p.OS)
		}
		path = fmt.Sprintf("%s/%s", path, p.Series)
	}
	return path
}

// RefreshClient defines a client for refresh requests.
type RefreshClient struct {
	path   path.Path
	client RESTClient
	logger Logger
}

// NewRefreshClient creates a RefreshClient for requesting
func NewRefreshClient(path path.Path, client RESTClient, logger Logger) *RefreshClient {
	return &RefreshClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// RefreshConfig defines a type for building refresh requests.
type RefreshConfig interface {
	// Build a refresh request for sending to the API.
	Build() (transport.RefreshRequest, error)

	// Ensure that the request back contains the information we requested.
	Ensure([]transport.RefreshResponse) error

	// String describes the underlying refresh config.
	String() string
}

// Refresh is used to refresh installed charms to a more suitable revision.
func (c *RefreshClient) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	c.logger.Tracef("Refresh(%s)", pretty.Sprint(config))
	req, err := config.Build()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var resp transport.RefreshResponses
	restResp, err := c.client.Post(ctx, c.path, req, &resp)

	if err != nil {
		return nil, errors.Trace(err)
	}

	if resultErr := resp.ErrorList.Combine(); resultErr != nil {
		if restResp.StatusCode == http.StatusNotFound {
			return nil, errors.NewNotFound(resultErr, "")
		}
		return nil, errors.Trace(resultErr)
	}

	c.logger.Tracef("Refresh() unmarshalled: %s", pretty.Sprint(resp.Results))
	return resp.Results, config.Ensure(resp.Results)
}

// refreshOne holds the config for making refresh calls to the CharmHub API.
type refreshOne struct {
	ID       string
	Revision int
	Channel  string
	Platform RefreshPlatform
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	instanceKey string
}

func (c refreshOne) String() string {
	return fmt.Sprintf("Refresh one (instanceKey: %s): using ID %s revision %+v, with channel %s and platform %v",
		c.instanceKey, c.ID, c.Revision, c.Channel, c.Platform.String())
}

// RefreshOne creates a request config for requesting only one charm.
func RefreshOne(id string, revision int, channel string, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return refreshOne{
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    revision,
		Channel:     channel,
		Platform:    platform,
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
				OS:           c.Platform.OS,
				Series:       c.Platform.Series,
				Architecture: c.Platform.Architecture,
			},
			TrackingChannel: c.Channel,
			// TODO (stickupkid): We need to model the refreshed date. It's
			// currently optional, but will be required at some point. This
			// is the installed date of the charm on the system.
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(RefreshAction),
			InstanceKey: c.instanceKey,
			ID:          &c.ID,
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

type executeOne struct {
	ID       string
	Name     string
	Revision *int
	Channel  *string
	Platform RefreshPlatform
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	action      Action
	instanceKey string
}

// InstallOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func InstallOneFromRevision(name string, revision int, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      InstallAction,
		instanceKey: uuid.String(),
		Name:        name,
		Revision:    &revision,
		Platform:    platform,
	}, nil
}

// InstallOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func InstallOneFromChannel(name string, channel string, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      InstallAction,
		instanceKey: uuid.String(),
		Name:        name,
		Channel:     &channel,
		Platform:    platform,
	}, nil
}

// DownloadOne creates a request config for requesting only one charm.
func DownloadOne(id string, revision int, channel string, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      DownloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		Channel:     &channel,
		Platform:    platform,
	}, nil
}

// DownloadOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func DownloadOneFromRevision(id string, revision int, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      DownloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		Platform:    platform,
	}, nil
}

// DownloadOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func DownloadOneFromChannel(name string, channel string, platform RefreshPlatform) (RefreshConfig, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      DownloadAction,
		instanceKey: uuid.String(),
		Name:        name,
		Channel:     &channel,
		Platform:    platform,
	}, nil
}

// Build a refresh request that can be past to the API.
func (c executeOne) Build() (transport.RefreshRequest, error) {
	return transport.RefreshRequest{
		// Context is required here, even if it looks optional.
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(c.action),
			InstanceKey: c.instanceKey,
			Name:        &c.Name,
			Revision:    c.Revision,
			Channel:     c.Channel,
			Platform: &transport.RefreshRequestPlatform{
				OS:           c.Platform.OS,
				Series:       c.Platform.Series,
				Architecture: c.Platform.Architecture,
			},
		}},
	}, nil
}

// Ensure that the request back contains the information we requested.
func (c executeOne) Ensure(responses []transport.RefreshResponse) error {
	for _, resp := range responses {
		if resp.InstanceKey == c.instanceKey {
			return nil
		}
	}
	return errors.NotValidf("%v action key", string(c.action))
}

func (c executeOne) String() string {
	var channel string
	if c.Channel != nil {
		channel = *c.Channel
	}
	return fmt.Sprintf("Execute One (action: %s, instanceKey: %s): using Name: %s with revision: %+v, channel %v and platform %s",
		c.action, c.instanceKey, c.Name, c.Revision, channel, c.Platform)
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

func (c refreshMany) String() string {
	plans := make([]string, len(c.Configs))
	for i, config := range c.Configs {
		plans[i] = config.String()
	}
	return strings.Join(plans, "\n")
}
