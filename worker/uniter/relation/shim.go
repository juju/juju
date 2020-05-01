// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/uniter"
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
