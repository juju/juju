// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/charmhub/transport"
)

// RefreshConfig defines a type for building refresh requests.
type RefreshConfig interface {
	// Build a refresh request for sending to the API.
	Build(ctx context.Context) (transport.RefreshRequest, error)

	// Ensure that the request back contains the information we requested.
	Ensure([]transport.RefreshResponse) error

	// String describes the underlying refresh config.
	String() string
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
	metrics     transport.ContextMetrics
	fields      []string
}

// InstanceKey returns the underlying instance key.
func (c refreshOne) InstanceKey() string {
	return c.instanceKey
}

func (c refreshOne) String() string {
	return fmt.Sprintf("Refresh one (instanceKey: %s): using ID %s revision %+v, with channel %s and base %v",
		c.instanceKey, c.ID, c.Revision, c.Channel, c.Base.String())
}

// Build a refresh request that can be past to the API.
func (c refreshOne) Build(ctx context.Context) (transport.RefreshRequest, error) {
	base, err := constructRefreshBase(ctx, c.Base)
	if err != nil {
		return transport.RefreshRequest{}, errors.Trace(err)
	}

	return transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey:     c.instanceKey,
			ID:              c.ID,
			Revision:        c.Revision,
			Base:            base,
			TrackingChannel: c.Channel,
			Metrics:         c.metrics,
			// TODO (stickupkid): We need to model the refreshed date. It's
			// currently optional, but will be required at some point. This
			// is the installed date of the charm on the system.
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      string(refreshAction),
			InstanceKey: c.instanceKey,
			ID:          &c.ID,
		}},
		Fields: c.fields,
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
	Base     RefreshBase
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	action      action
	instanceKey string
	fields      []string
}

// InstanceKey returns the underlying instance key.
func (c executeOne) InstanceKey() string {
	return c.instanceKey
}

// Build a refresh request that can be past to the API.
func (c executeOne) Build(ctx context.Context) (transport.RefreshRequest, error) {
	base, err := constructRefreshBase(ctx, c.Base)
	if err != nil {
		return transport.RefreshRequest{}, errors.Trace(err)
	}

	var id *string
	if c.ID != "" {
		id = &c.ID
	}
	var name *string
	if c.Name != "" {
		name = &c.Name
	}

	req := transport.RefreshRequest{
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
		Fields: c.fields,
	}
	return req, nil
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

type executeOneByRevision struct {
	Name     string
	Revision *int
	// ID is only used for download by revision
	ID                string
	resourceRevisions []transport.RefreshResourceRevision
	// instanceKey is a private unique key that we construct for CharmHub API
	// asynchronous calls.
	instanceKey string
	action      action
	fields      []string
}

// InstanceKey returns the underlying instance key.
func (c executeOneByRevision) InstanceKey() string {
	return c.instanceKey
}

// Build a refresh request for sending to the API.
func (c executeOneByRevision) Build(ctx context.Context) (transport.RefreshRequest, error) {
	var name, id *string
	if c.Name != "" {
		name = &c.Name
	}
	if c.ID != "" {
		id = &c.ID
	}

	req := transport.RefreshRequest{
		// Context is required here, even if it looks optional.
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:            string(c.action),
			InstanceKey:       c.instanceKey,
			Name:              name,
			ID:                id,
			Revision:          c.Revision,
			ResourceRevisions: c.resourceRevisions,
		}},
		Fields: []string{"bases", "download", "id", "revision", "version", "resources", "type"},
	}

	if len(c.fields) != 0 {
		fieldSet := set.NewStrings(req.Fields...)
		for _, field := range c.fields {
			fieldSet.Add(field)
		}
		req.Fields = fieldSet.SortedValues()
	}

	return req, nil
}

// Ensure that the request back contains the information we requested.
func (c executeOneByRevision) Ensure(responses []transport.RefreshResponse) error {
	for _, resp := range responses {
		if resp.InstanceKey == c.instanceKey {
			return nil
		}
	}
	return errors.NotValidf("%v action key", string(c.action))
}

// String describes the underlying refresh config.
func (c executeOneByRevision) String() string {
	var revision string
	if c.Revision != nil {
		revision = fmt.Sprintf(" with revision: %+v", c.Revision)
	}
	return fmt.Sprintf("Install One (action: %s, instanceKey: %s): using Name %s %s",
		c.action, c.instanceKey, c.Name, revision)
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
func (c refreshMany) Build(ctx context.Context) (transport.RefreshRequest, error) {
	if len(c.Configs) == 0 {
		return transport.RefreshRequest{}, errors.NotFoundf("configs")
	}
	// Not all configs built here have a context, start out with an empty
	// slice, so we do not call Refresh with a nil context.
	// See executeOne.Build().
	result := transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
	}
	for _, config := range c.Configs {
		req, err := config.Build(ctx)
		if err != nil {
			return transport.RefreshRequest{}, errors.Trace(err)
		}
		result.Context = append(result.Context, req.Context...)
		result.Actions = append(result.Actions, req.Actions...)
		result.Fields = append(result.Fields, req.Fields...)
	}

	// Ensure that the required field list contains no duplicates
	if len(result.Fields) != 0 {
		result.Fields = set.NewStrings(result.Fields...).SortedValues()
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
