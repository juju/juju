// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Relations holds the values for the hook context.
type Relations struct {
	Relations map[int]*ContextRelation
}

func (r *Relations) setRelation(id int, relCtx *ContextRelation) {
	if r.Relations == nil {
		r.Relations = make(map[int]*ContextRelation)
	}
	r.Relations[id] = relCtx
}

// RelationHook holds the values for the hook context.
type RelationHook struct {
	HookRelation   int
	RemoteUnitName string
}

// ContextRelations is a test double for jujuc.ContextRelations and RelationHook.
type ContextRelations struct {
	Stub      *testing.Stub
	Relations *Relations
	Hook      *RelationHook
}

func (c *ContextRelations) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Relations == nil {
		c.Relations = &Relations{}
	}
	if c.Hook == nil {
		c.Hook = &RelationHook{}
	}
}

func (c *ContextRelations) setRelation(id int, name string) *ContextRelation {
	c.init()
	if name == "" {
		name = fmt.Sprintf("relation-%d", id)
	}
	rel := &Relation{
		Id:   id,
		Name: name,
	}
	relCtx := &ContextRelation{c.Stub, rel}

	c.Relations.setRelation(id, relCtx)
	return relCtx
}

func (c *ContextRelations) setRelated(id int, unit string, settings Settings) {
	c.init()
	rels := c.Relations
	relCtx, ok := rels.Relations[id]
	if !ok {
		relCtx = c.setRelation(id, "")
	}
	relCtx.setUnit(unit, settings)
}

// HookRelation implements jujuc.ContextRelations.
func (c *ContextRelations) HookRelation() (jujuc.ContextRelation, bool) {
	c.Stub.AddCall("HookRelation")
	c.Stub.NextErr()

	c.init()
	return c.Relation(c.Hook.HookRelation)
}

// RemoteUnitName implements jujuc.ContextRelations.
func (c *ContextRelations) RemoteUnitName() (string, bool) {
	c.Stub.AddCall("RemoteUnitName")
	c.Stub.NextErr()

	c.init()
	return c.Hook.RemoteUnitName, c.Hook.RemoteUnitName != ""
}

// Relation implements jujuc.ContextRelations.
func (c *ContextRelations) Relation(id int) (jujuc.ContextRelation, bool) {
	c.Stub.AddCall("Relation", id)
	c.Stub.NextErr()

	c.init()
	r, found := c.Relations.Relations[id]
	return r, found
}

// RelationIds implements jujuc.ContextRelations.
func (c *ContextRelations) RelationIds() []int {
	c.Stub.AddCall("RelationIds")
	c.Stub.NextErr()

	c.init()
	ids := []int{}
	for id := range c.Relations.Relations {
		ids = append(ids, id)
	}
	return ids
}

// Relation holds the data for the test double.
type Relation struct {
	Id       int
	Name     string
	Units    map[string]Settings
	UnitName string
}

func (r *Relation) setUnit(name string, settings Settings) {
	if r.Units == nil {
		r.Units = make(map[string]Settings)
	}
	r.Units[name] = settings
}

// ContextRelation is a test double for jujuc.ContextRelation.
type ContextRelation struct {
	Stub *testing.Stub
	Info *Relation
}

func (c *ContextRelation) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Info == nil {
		c.Info = &Relation{}
	}
}

func (r *ContextRelation) setUnit(name string, settings Settings) {
	r.init()
	r.Info.setUnit(name, settings)
}

// Id implements jujuc.ContextRelation.
func (r *ContextRelation) Id() int {
	r.Stub.AddCall("Id")
	r.Stub.NextErr()
	r.init()
	return r.Info.Id
}

// Name implements jujuc.ContextRelation.
func (r *ContextRelation) Name() string {
	r.Stub.AddCall("Name")
	r.Stub.NextErr()
	r.init()
	return r.Info.Name
}

// FakeId implements jujuc.ContextRelation.
func (r *ContextRelation) FakeId() string {
	r.Stub.AddCall("FakeId")
	r.Stub.NextErr()
	r.init()
	return fmt.Sprintf("%s:%d", r.Info.Name, r.Info.Id)
}

// Settings implements jujuc.ContextRelation.
func (r *ContextRelation) Settings() (jujuc.Settings, error) {
	r.Stub.AddCall("Settings")
	if err := r.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	r.init()
	return r.Info.Units[r.Info.UnitName], nil
}

// UnitNames implements jujuc.ContextRelation.
func (r *ContextRelation) UnitNames() []string {
	r.Stub.AddCall("UnitNames")
	r.Stub.NextErr()
	r.init()
	var s []string // initially nil to match the true context.
	for name := range r.Info.Units {
		s = append(s, name)
	}
	sort.Strings(s)
	return s
}

// ReadSettings implements jujuc.ContextRelation.
func (r *ContextRelation) ReadSettings(name string) (params.Settings, error) {
	r.Stub.AddCall("ReadSettings", name)
	if err := r.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	r.init()
	s, found := r.Info.Units[name]
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
