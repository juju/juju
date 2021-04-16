// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v2"
	"github.com/kr/pretty"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
	coreseries "github.com/juju/juju/core/series"
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

const (
	// NotAvailable is used a placeholder for Name and Channel for a refresh
	// base request, if the Name and Channel is not known.
	NotAvailable = "NA"
)

// Headers represents a series of headers that we would like to pass to the REST
// API.
type Headers = map[string][]string

// RefreshBase defines a base for selecting a specific charm.
// Continues to exist to allow for incoming bases to be converted
// to bases inside this package.
type RefreshBase struct {
	Architecture string
	Name         string
	Channel      string
}

func (p RefreshBase) String() string {
	path := p.Architecture
	if p.Channel != "" {
		if p.Name != "" {
			path = fmt.Sprintf("%s/%s", path, p.Name)
		}
		path = fmt.Sprintf("%s/%s", path, p.Channel)
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
	Build() (transport.RefreshRequest, Headers, error)

	// Ensure that the request back contains the information we requested.
	Ensure([]transport.RefreshResponse) error

	// String describes the underlying refresh config.
	String() string
}

// Refresh is used to refresh installed charms to a more suitable revision.
func (c *RefreshClient) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	c.logger.Tracef("Refresh(%s)", pretty.Sprint(config))
	req, headers, err := config.Build()
	if err != nil {
		return nil, errors.Trace(err)
	}

	httpHeaders := make(http.Header)
	for k, values := range headers {
		for _, value := range values {
			httpHeaders.Add(MetadataHeader, fmt.Sprintf("%s=%s", k, value))
		}
	}

	var resp transport.RefreshResponses
	restResp, err := c.client.Post(ctx, c.path, httpHeaders, req, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf("refresh")
	}
	if err := handleBasicAPIErrors(resp.ErrorList, c.logger); err != nil {
		return nil, errors.Trace(err)
	}

	c.logger.Tracef("Refresh() unmarshalled: %s", pretty.Sprint(resp.Results))
	return resp.Results, config.Ensure(resp.Results)
}

// refreshOne holds the config for making refresh calls to the CharmHub API.
type refreshOne struct {
	ID       string
	Revision int
	Channel  string
	Base     RefreshBase
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	instanceKey string
}

// InstanceKey returns the underlying instance key.
func (c refreshOne) InstanceKey() string {
	return c.instanceKey
}

func (c refreshOne) String() string {
	return fmt.Sprintf("Refresh one (instanceKey: %s): using ID %s revision %+v, with channel %s and base %v",
		c.instanceKey, c.ID, c.Revision, c.Channel, c.Base.String())
}

// RefreshOne creates a request config for requesting only one charm.
func RefreshOne(id string, revision int, channel string, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return refreshOne{
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    revision,
		Channel:     channel,
		Base:        base,
	}, nil
}

// Build a refresh request that can be past to the API.
func (c refreshOne) Build() (transport.RefreshRequest, Headers, error) {
	base, err := constructRefreshBase(c.Base)
	if err != nil {
		return transport.RefreshRequest{}, nil, errors.Trace(err)
	}

	return transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey:     c.instanceKey,
			ID:              c.ID,
			Revision:        c.Revision,
			Base:            base,
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
	}, constructMetadataHeaders(c.Base), nil
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
	Base     RefreshBase
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	action      Action
	instanceKey string
}

// InstanceKey returns the underlying instance key.
func (c executeOne) InstanceKey() string {
	return c.instanceKey
}

// InstallOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func InstallOneFromRevision(name string, revision int, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      InstallAction,
		instanceKey: uuid.String(),
		Name:        name,
		Revision:    &revision,
		Base:        base,
	}, nil
}

// InstallOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func InstallOneFromChannel(name string, channel string, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      InstallAction,
		instanceKey: uuid.String(),
		Name:        name,
		Channel:     &channel,
		Base:        base,
	}, nil
}

// DownloadOne creates a request config for requesting only one charm.
func DownloadOne(id string, revision int, channel string, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
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
		Base:        base,
	}, nil
}

// DownloadOneFromRevision creates a request config using the revision and not
// the channel for requesting only one charm.
func DownloadOneFromRevision(id string, revision int, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      DownloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Revision:    &revision,
		Base:        base,
	}, nil
}

// DownloadOneFromChannel creates a request config using the channel and not the
// revision for requesting only one charm.
func DownloadOneFromChannel(id string, channel string, base RefreshBase) (RefreshConfig, error) {
	if err := validateBase(base); err != nil {
		return nil, errors.Trace(err)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return executeOne{
		action:      DownloadAction,
		instanceKey: uuid.String(),
		ID:          id,
		Channel:     &channel,
		Base:        base,
	}, nil
}

// Build a refresh request that can be past to the API.
func (c executeOne) Build() (transport.RefreshRequest, Headers, error) {
	base, err := constructRefreshBase(c.Base)
	if err != nil {
		return transport.RefreshRequest{}, nil, errors.Trace(err)
	}

	var id *string
	if c.ID != "" {
		id = &c.ID
	}
	var name *string
	if c.Name != "" {
		name = &c.Name
	}

	return transport.RefreshRequest{
		// Context is required here, even if it looks optional.
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(c.action),
			InstanceKey: c.instanceKey,
			ID:          id,
			Name:        name,
			Revision:    c.Revision,
			Channel:     c.Channel,
			Base:        &base,
		}},
	}, constructMetadataHeaders(c.Base), nil
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
	var using string
	if c.ID != "" {
		using = fmt.Sprintf("ID %s", c.ID)
	} else {
		using = fmt.Sprintf("Name %s", c.Name)
	}
	var revision string
	if c.Revision != nil {
		revision = fmt.Sprintf(" with revision: %+v", c.Revision)
	}
	return fmt.Sprintf("Execute One (action: %s, instanceKey: %s): using %s%s channel %v and base %s",
		c.action, c.instanceKey, using, revision, channel, c.Base)
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
func (c refreshMany) Build() (transport.RefreshRequest, Headers, error) {
	var composedHeaders Headers
	// Not all configs built here have a context, start out with an empty
	// slice, so we do not call Refresh with a nil context.
	// See executeOne.Build().
	result := transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
	}
	for _, config := range c.Configs {
		req, headers, err := config.Build()
		if err != nil {
			return transport.RefreshRequest{}, nil, errors.Trace(err)
		}
		result.Context = append(result.Context, req.Context...)
		result.Actions = append(result.Actions, req.Actions...)
		composedHeaders = composeMetadataHeaders(composedHeaders, headers)
	}
	return result, composedHeaders, nil
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

// constructRefreshBase creates a refresh request base that allows for
// partial base queries.
func constructRefreshBase(base RefreshBase) (transport.Base, error) {
	if base.Architecture == "" {
		return transport.Base{}, errors.NotValidf("refresh arch")
	}

	name := base.Name
	if name == "" {
		name = NotAvailable
	}

	var channel string
	var err error
	switch base.Channel {
	case "":
		channel = NotAvailable
	case "kubernetes":
		// Kubernetes is not a valid channel for a base.  Instead use the latest
		// LTS version of ubuntu.
		channel, err = coreseries.SeriesVersion(coreseries.LatestLts())
		if err != nil {
			return transport.Base{}, errors.NotValidf("invalid latest version")
		}
	default:
		// If we have a series, we need to convert it to a stable version. If
		// we have a version, then just pass that through.
		potential, err := coreseries.SeriesVersion(base.Channel)
		if err == nil {
			channel = potential
		} else {
			channel = base.Channel
		}
	}

	return transport.Base{
		Architecture: base.Architecture,
		Name:         name,
		Channel:      channel,
	}, nil
}

// constructHeaders adds X-Juju-Metadata headers for the charms' unique channel
// and architecture values, for example:
//
// X-Juju-Metadata: channel=bionic
// X-Juju-Metadata: arch=amd64
// X-Juju-Metadata: channel=focal
func constructMetadataHeaders(base RefreshBase) map[string][]string {
	headers := make(map[string][]string)
	if base.Architecture != "" {
		headers["arch"] = []string{base.Architecture}
	}
	if base.Name != "" {
		headers["name"] = []string{base.Name}
	}
	if base.Channel != "" {
		headers["channel"] = []string{base.Channel}
	}
	return headers
}

func composeMetadataHeaders(a, b Headers) Headers {
	result := make(map[string][]string)
	for k, v := range a {
		result[k] = append(result[k], v...)
	}
	for k, v := range b {
		result[k] = append(result[k], v...)
	}
	for k, v := range result {
		result[k] = set.NewStrings(v...).SortedValues()
	}
	return result
}

// validateBase ensures that we do not pass "all" as part of base.
// This function is to help find programming related failures.
func validateBase(rp RefreshBase) error {
	var msg []string
	if rp.Architecture == "all" {
		msg = append(msg, fmt.Sprintf("Architecture %q", rp.Architecture))
	}
	if rp.Name == "all" {
		msg = append(msg, fmt.Sprintf("Name %q", rp.Name))
	}
	if rp.Channel == "all" {
		msg = append(msg, fmt.Sprintf("Channel %q", rp.Channel))
	}
	if len(msg) > 0 {
		err := errors.Trace(errors.NotValidf(strings.Join(msg, ", ")))
		// Log the error here, trace on this side gets lost when the error
		// goes thru to the client.
		logger := loggo.GetLogger("juju.charmhub.validatebase")
		logger.Errorf(fmt.Sprintf("%s", err))
		return err
	}
	return nil
}

type instanceKey interface {
	InstanceKey() string
}

// ExtractConfigInstanceKey is used to get the instance key from a refresh
// config.
func ExtractConfigInstanceKey(cfg RefreshConfig) string {
	key, ok := cfg.(instanceKey)
	if ok {
		return key.InstanceKey()
	}
	return ""
}
