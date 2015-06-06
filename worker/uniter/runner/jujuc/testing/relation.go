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

// Relations holds the values for the hook context.
type Relations struct {
	Relations map[int]jujuc.ContextRelation
}

// Set adds the relation to the set of known relations.
func (r *Relations) Set(id int, relCtx jujuc.ContextRelation) {
	if r.Relations == nil {
		r.Relations = make(map[int]jujuc.ContextRelation)
	}
	r.Relations[id] = relCtx
}

// ContextRelations is a test double for jujuc.ContextRelations.
type ContextRelations struct {
	contextBase
	info *Relations
}

func (c *ContextRelations) setRelation(id int, name string) *Relation {
	if name == "" {
		name = fmt.Sprintf("relation-%d", id)
	}
	rel := &Relation{
		Id:   id,
		Name: name,
	}
	relCtx := &ContextRelation{info: rel}
	relCtx.stub = c.stub

	c.info.Set(id, relCtx)
	return rel
}

// Relation implements jujuc.ContextRelations.
func (c *ContextRelations) Relation(id int) (jujuc.ContextRelation, bool) {
	c.stub.AddCall("Relation", id)
	c.stub.NextErr()

	r, found := c.info.Relations[id]
	return r, found
}

// RelationIds implements jujuc.ContextRelations.
func (c *ContextRelations) RelationIds() []int {
	c.stub.AddCall("RelationIds")
	c.stub.NextErr()

	ids := []int{}
	for id := range c.info.Relations {
		ids = append(ids, id)
	}
	return ids
}

// RelationHook holds the values for the hook context.
type RelationHook struct {
	HookRelation   jujuc.ContextRelation
	RemoteUnitName string
}

// ContextRelationHook is a test double for jujuc.RelationHookContext.
type ContextRelationHook struct {
	contextBase
	info *RelationHook
}

// HookRelation implements jujuc.RelationHookContext.
func (c *ContextRelationHook) HookRelation() (jujuc.ContextRelation, bool) {
	c.stub.AddCall("HookRelation")
	c.stub.NextErr()

	return c.info.HookRelation, c.info.HookRelation == nil
}

// RemoteUnitName implements jujuc.RelationHookContext.
func (c *ContextRelationHook) RemoteUnitName() (string, bool) {
	c.stub.AddCall("RemoteUnitName")
	c.stub.NextErr()

	return c.info.RemoteUnitName, c.info.RemoteUnitName != ""
}

// Relation holds the data for the test double.
type Relation struct {
	Id       int
	Name     string
	Units    map[string]Settings
	UnitName string
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

//func (r *ContextRelation) setUnit(name string, settings Settings) {
//	r.info.setUnit(name, settings)
//}

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

// Settings is a test double for jujuc.Settings.
type Settings params.Settings

// Get implements jujuc.Settings.
func (s Settings) Get(k string) (interface{}, bool) {
	v, f := s[k]
	return v, f
}

// Set implements jujuc.Settings.
func (s Settings) Set(k, v string) {
	s[k] = v
}

// Delete implements jujuc.Settings.
func (s Settings) Delete(k string) {
	delete(s, k)
}

// Map implements jujuc.Settings.
func (s Settings) Map() params.Settings {
	r := params.Settings{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
