// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
)

// This file contains shims which are used to adapt the concrete uniter
// api structs to the domain interfaces used by the uniter worker.

type UniterClientShim struct {
	*uniter.Client
}

func (s UniterClientShim) Charm(curl string) (Charm, error) {
	return s.Client.Charm(curl)
}

func (s UniterClientShim) Unit(ctx context.Context, tag names.UnitTag) (Unit, error) {
	u, err := s.Client.Unit(ctx, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return UnitShim{u}, nil
}

func (s UniterClientShim) Relation(ctx context.Context, tag names.RelationTag) (Relation, error) {
	r, err := s.Client.Relation(ctx, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r}, nil
}

func (s UniterClientShim) RelationById(ctx context.Context, id int) (Relation, error) {
	r, err := s.Client.RelationById(ctx, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r}, nil
}

func (s UniterClientShim) Application(ctx context.Context, tag names.ApplicationTag) (Application, error) {
	return s.Client.Application(ctx, tag)
}

type UnitShim struct {
	*uniter.Unit
}

func (s UnitShim) Application(ctx context.Context) (Application, error) {
	return s.Unit.Application(ctx)
}

type relationShim struct {
	*uniter.Relation
}

func (s relationShim) Unit(ctx context.Context, tag names.UnitTag) (RelationUnit, error) {
	ru, err := s.Relation.Unit(ctx, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return RelationUnitShim{ru}, nil
}

type RelationUnitShim struct {
	*uniter.RelationUnit
}

func (s RelationUnitShim) Relation() Relation {
	return relationShim{s.RelationUnit.Relation()}
}
