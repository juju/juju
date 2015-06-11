// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Relation holds the data for the test double.
type Relation struct {
	// Id is data for jujuc.ContextRelation.
	Id int
	// Name is data for jujuc.ContextRelation.
	Name string
	// Units is data for jujuc.ContextRelation.
	Units map[string]Settings
	// UnitName is data for jujuc.ContextRelation.
	UnitName string
}

// Reset clears the Relation's settings.
func (r *Relation) Reset() {
	r.Units = nil
}

// SetRelated adds the relation settings for the unit.
func (r *Relation) SetRelated(name string, settings Settings) {
	if r.Units == nil {
		r.Units = make(map[string]Settings)
	}
	r.Units[name] = settings
}

// ContextRelation is a test double for jujuc.ContextRelation.
type ContextRelation struct {
	contextBase
	info *Relation
}

// Id implements jujuc.ContextRelation.
func (r *ContextRelation) Id() int {
	r.stub.AddCall("Id")
	r.stub.NextErr()

	return r.info.Id
}

// Name implements jujuc.ContextRelation.
func (r *ContextRelation) Name() string {
	r.stub.AddCall("Name")
	r.stub.NextErr()

	return r.info.Name
}

// FakeId implements jujuc.ContextRelation.
func (r *ContextRelation) FakeId() string {
	r.stub.AddCall("FakeId")
	r.stub.NextErr()

	return fmt.Sprintf("%s:%d", r.info.Name, r.info.Id)
}

// Settings implements jujuc.ContextRelation.
func (r *ContextRelation) Settings() (jujuc.Settings, error) {
	r.stub.AddCall("Settings")
	if err := r.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	settings, ok := r.info.Units[r.info.UnitName]
	if !ok {
		return nil, errors.Errorf("no settings for %q", r.info.UnitName)
	}
	return settings, nil
}

// UnitNames implements jujuc.ContextRelation.
func (r *ContextRelation) UnitNames() []string {
	r.stub.AddCall("UnitNames")
	r.stub.NextErr()

	var s []string // initially nil to match the true context.
	for name := range r.info.Units {
		s = append(s, name)
	}
	sort.Strings(s)
	return s
}

// ReadSettings implements jujuc.ContextRelation.
func (r *ContextRelation) ReadSettings(name string) (params.Settings, error) {
	r.stub.AddCall("ReadSettings", name)
	if err := r.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	s, found := r.info.Units[name]
	if !found {
		return nil, fmt.Errorf("unknown unit %s", name)
	}
	return s.Map(), nil
}
