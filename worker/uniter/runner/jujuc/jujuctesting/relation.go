// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/rpc/params"
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
	// RemoteApplicationSettings is data for jujuc.ContextRelation
	RemoteApplicationSettings Settings
	// LocalApplicationSettings is data for jujuc.ContextRelation
	LocalApplicationSettings Settings
	// RemoteApplicationName is data for jujuc.ContextRelation
	RemoteApplicationName string
	// The current life value.
	Life life.Value
}

// Reset clears the Relation's settings.
func (r *Relation) Reset() {
	r.Units = nil
	r.RemoteApplicationSettings = nil
	r.LocalApplicationSettings = nil
}

// SetRelated adds the relation settings for the unit.
func (r *Relation) SetRelated(name string, settings Settings) {
	if r.Units == nil {
		r.Units = make(map[string]Settings)
	}
	r.Units[name] = settings
}

// SetRemoteApplicationSettings sets the settings for the remote application.
func (r *Relation) SetRemoteApplicationSettings(settings Settings) {
	r.RemoteApplicationSettings = settings
}

// SetLocalApplicationSettings sets the settings for the local application.
func (r *Relation) SetLocalApplicationSettings(settings Settings) {
	r.LocalApplicationSettings = settings
}

// ContextRelation is a test double for jujuc.ContextRelation.
type ContextRelation struct {
	contextBase
	info *Relation
}

// Id implements jujuc.ContextRelation.
func (r *ContextRelation) Id() int {
	r.stub.AddCall("Id")
	_ = r.stub.NextErr()

	return r.info.Id
}

// Name implements jujuc.ContextRelation.
func (r *ContextRelation) Name() string {
	r.stub.AddCall("Name")
	_ = r.stub.NextErr()

	return r.info.Name
}

// RelationTag implements jujuc.ContextRelation.
func (r *ContextRelation) RelationTag() names.RelationTag {
	r.stub.AddCall("RelationTag")
	_ = r.stub.NextErr()

	return names.NewRelationTag("wordpress:db mediawiki:db")
}

// FakeId implements jujuc.ContextRelation.
func (r *ContextRelation) FakeId() string {
	r.stub.AddCall("FakeId")
	_ = r.stub.NextErr()

	return fmt.Sprintf("%s:%d", r.info.Name, r.info.Id)
}

// Life implements jujuc.ContextRelation.
func (r *ContextRelation) Life() life.Value {
	r.stub.AddCall("Life")
	_ = r.stub.NextErr()

	return r.info.Life
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

// ApplicationSettings implements jujuc.ContextRelation.
func (r *ContextRelation) ApplicationSettings() (jujuc.Settings, error) {
	r.stub.AddCall("ApplicationSettings")
	if err := r.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return r.info.LocalApplicationSettings, nil
}

// UnitNames implements jujuc.ContextRelation.
func (r *ContextRelation) UnitNames() []string {
	r.stub.AddCall("UnitNames")
	_ = r.stub.NextErr()

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

// ReadApplicationSettings implements jujuc.ContextRelation.
func (r *ContextRelation) ReadApplicationSettings(name string) (params.Settings, error) {
	r.stub.AddCall("ReadApplicationSettings", name)
	if err := r.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return r.info.RemoteApplicationSettings.Map(), nil
}

// Suspended implements jujuc.ContextRelation.
func (r *ContextRelation) Suspended() bool {
	return true
}

// SetStatus implements jujuc.ContextRelation.
func (r *ContextRelation) SetStatus(status relation.Status) error {
	return nil
}

// RemoteApplicationName implements jujuc.ContextRelation.
func (r *ContextRelation) RemoteApplicationName() string {
	r.stub.AddCall("RemoteApplicationName")
	return r.info.RemoteApplicationName
}
