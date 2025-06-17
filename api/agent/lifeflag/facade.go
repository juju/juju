// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewWatcherFunc exists to let us test Watch properly.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// Client makes calls to the LifeFlag facade.
type Client struct {
	caller base.FacadeCaller
}

// NewClient returns a new Facade using the supplied caller.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{
		caller: base.NewFacadeCaller(caller, "AgentLifeFlag", options...),
	}
}

// ErrEntityNotFound indicates that the requested entity no longer exists.
//
// We avoid errors.NotFound, because errors.NotFound is non-specific, and
// it's our job to communicate *this specific condition*. There are many
// possible sources of errors.NotFound in the world, and it's not safe or
// sane for a client to treat a generic NotFound as specific to the entity
// in question.
//
// We're still vulnerable to apiservers returning unjustified CodeNotFound
// but at least we're safe from accidental errors.NotFound injection in
// the api client mechanism.
const ErrEntityNotFound = errors.ConstError("entity not found")

// Watch returns a NotifyWatcher that sends a value whenever the
// entity's life value may have changed; or ErrEntityNotFound; or some
// other error.
func (c *Client) Watch(ctx context.Context, entity names.Tag) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: entity.String()}},
	}
	var results params.NotifyWatchResults
	err := c.caller.FacadeCall(ctx, "Watch", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("expected 1 Watch result, got %d", count)
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		if params.IsCodeNotFound(err) {
			return nil, ErrEntityNotFound
		}
		return nil, errors.Trace(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.caller.RawAPICaller(), result)
	return w, nil
}

// Life returns the entity's life value; or ErrEntityNotFound; or some
// other error.
func (c *Client) Life(ctx context.Context, entity names.Tag) (life.Value, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: entity.String()}},
	}
	var results params.LifeResults
	err := c.caller.FacadeCall(ctx, "Life", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return "", errors.Errorf("expected 1 Life result, got %d", count)
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		if params.IsCodeNotFound(err) {
			return "", ErrEntityNotFound
		}
		return "", errors.Trace(result.Error)
	}
	life := result.Life
	if err := life.Validate(); err != nil {
		return "", errors.Trace(err)
	}
	return life, nil
}
