// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Life is a local representation of entity life. Should probably be
// in a core/life or core/entity package; and quite probably, so
// should the ConnFacade interface.
type Life string

const (
	Alive Life = "alive"
	Dying Life = "dying"
	Dead  Life = "dead"
)

// ConnFacade exposes the parts of the Agent facade needed by the bits
// that currently live in apicaller. This criterion is every bit as
// terrible as it sounds -- surely there should be a new facade at the
// apiserver level somewhere? -- but:
//  1. this feels like a convenient/transitional method grouping, not a
//     fundamental *role*; and
//
// Progress not perfection.
type ConnFacade interface {

	// Life returns Alive, Dying, Dead, ErrDenied, or some other error.
	Life(context.Context, names.Tag) (Life, error)

	// SetPassword returns nil, ErrDenied, or some other error.
	SetPassword(context.Context, names.Tag, string) error
}

// ErrDenied is returned by Life and SetPassword to indicate that the
// requested operation is impossible (and hence that the entity is
// either dead or gone, and in either case that no further meaningful
// interaction is possible).
var ErrDenied = errors.New("entity operation impossible")

// NewConnFacade returns a ConnFacade backed by the supplied APICaller.
func NewConnFacade(caller base.APICaller) (ConnFacade, error) {
	facadeCaller := base.NewFacadeCaller(caller, "Agent")
	return &connFacade{
		caller: facadeCaller,
	}, nil
}

// connFacade implements ConnFacade.
type connFacade struct {
	caller base.FacadeCaller
}

// Life is part of the ConnFacade interface.
func (facade *connFacade) Life(ctx context.Context, entity names.Tag) (Life, error) {
	var results params.AgentGetEntitiesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: entity.String()}},
	}
	err := facade.caller.FacadeCall(ctx, "GetEntities", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Entities) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Entities))
	}
	if err := results.Entities[0].Error; err != nil {
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			return "", ErrDenied
		}
		return "", errors.Trace(err)
	}
	life := Life(results.Entities[0].Life)
	switch life {
	case Alive, Dying, Dead:
		return life, nil
	}
	return "", errors.Errorf("unknown life value %q", life)
}

// SetPassword is part of the ConnFacade interface.
func (facade *connFacade) SetPassword(ctx context.Context, entity names.Tag, password string) error {
	var results params.ErrorResults
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      entity.String(),
			Password: password,
		}},
	}
	err := facade.caller.FacadeCall(ctx, "SetPasswords", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		if params.IsCodeDead(err) {
			return ErrDenied
		} else if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			return ErrDenied
		}
		return errors.Trace(err)
	}
	return nil
}
