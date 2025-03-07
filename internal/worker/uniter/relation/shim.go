// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
)

type stateTrackerStateShim struct {
	*uniter.State
}

func (s *stateTrackerStateShim) Relation(tag names.RelationTag) (Relation, error) {
	rel, err := s.State.Relation(tag)
	if err != nil {
		return nil, err
	}
	return &relationShim{rel}, nil
}

// RelationById returns the existing relation with the given id.
func (s *stateTrackerStateShim) RelationById(id int) (Relation, error) {
	rel, err := s.State.RelationById(id)
	if err != nil {
		return nil, err
	}
	return &relationShim{rel}, nil
}

// Unit returns the existing unit with the given tag.
func (s *stateTrackerStateShim) Unit(tag names.UnitTag) (Unit, error) {
	unit, err := s.State.Unit(tag)
	if err != nil {
		return nil, err
	}
	return &unitShim{unit}, nil
}

type relationShim struct {
	*uniter.Relation
}

func (s *relationShim) Unit(uTag names.UnitTag) (RelationUnit, error) {
	u, err := s.Relation.Unit(uTag)
	if err != nil {
		return nil, err
	}
	return &RelationUnitShim{u}, nil
}

type unitShim struct {
	*uniter.Unit
}

func (s *unitShim) Application() (Application, error) {
	app, err := s.Unit.Application()
	if err != nil {
		return nil, err
	}
	return &applicationShim{app}, nil
}

type applicationShim struct {
	*uniter.Application
}

type RelationUnitShim struct {
	*uniter.RelationUnit
}

func (r *RelationUnitShim) Relation() Relation {
	return &relationShim{r.RelationUnit.Relation()}
}
