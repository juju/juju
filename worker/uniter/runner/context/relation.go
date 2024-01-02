// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type Relation interface {
	// Id returns the integer internal relation key.
	Id() int

	// Refresh refreshes the contents of the relation from the underlying
	// state.
	Refresh() error

	// Suspended returns the relation's current suspended status.
	Suspended() bool

	// SetStatus updates the status of the relation.
	SetStatus(status relation.Status) error

	// Tag returns the relation tag.
	Tag() names.RelationTag

	// OtherApplication returns the name of the application on the other
	// end of the relation (from this unit's perspective).
	OtherApplication() string

	// Life returns the relation's current life state.
	Life() life.Value
}

type RelationUnit interface {
	// ApplicationSettings returns a Settings which allows access to this unit's
	// application settings within the relation.
	ApplicationSettings() (*uniter.Settings, error)

	// Endpoint returns the relation endpoint that defines the unit's
	// participation in the relation.
	Endpoint() uniter.Endpoint

	// ReadSettings returns a map holding the settings of the unit with the
	// supplied name within this relation.
	ReadSettings(name string) (params.Settings, error)

	// Relation returns the relation associated with the unit.
	Relation() Relation

	// Settings returns a Settings which allows access to the unit's settings
	// within the relation.
	Settings() (*uniter.Settings, error)
}

type RelationUnitShim struct {
	*uniter.RelationUnit
}

func (r *RelationUnitShim) Relation() Relation {
	return r.RelationUnit.Relation()
}

type RelationInfo struct {
	RelationUnit RelationUnit
	MemberNames  []string
}

// ContextRelation is the implementation of hooks.ContextRelation.
type ContextRelation struct {
	ru           RelationUnit
	relationId   int
	endpointName string

	// settings allows read and write access to the relation unit settings.
	settings *uniter.Settings

	// applicationSettings allows read and write access to the relation application settings.
	applicationSettings *uniter.Settings

	// cache holds remote unit membership and settings.
	cache *RelationCache
}

// NewContextRelation creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewContextRelation(ru RelationUnit, cache *RelationCache) *ContextRelation {
	return &ContextRelation{
		ru:           ru,
		relationId:   ru.Relation().Id(),
		endpointName: ru.Endpoint().Name,
		cache:        cache,
	}
}

func (ctx *ContextRelation) Id() int {
	return ctx.relationId
}

func (ctx *ContextRelation) Name() string {
	return ctx.endpointName
}

func (ctx *ContextRelation) RelationTag() names.RelationTag {
	return ctx.ru.Relation().Tag()
}

func (ctx *ContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", ctx.endpointName, ctx.relationId)
}

func (ctx *ContextRelation) UnitNames() []string {
	return ctx.cache.MemberNames()
}

func (ctx *ContextRelation) ReadSettings(unit string) (settings params.Settings, err error) {
	return ctx.cache.Settings(unit)
}

func (ctx *ContextRelation) ReadApplicationSettings(app string) (settings params.Settings, err error) {
	return ctx.cache.ApplicationSettings(app)
}

func (ctx *ContextRelation) Settings() (jujuc.Settings, error) {
	if ctx.settings == nil {
		node, err := ctx.ru.Settings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

func (ctx *ContextRelation) ApplicationSettings() (jujuc.Settings, error) {
	if ctx.applicationSettings == nil {
		settings, err := ctx.ru.ApplicationSettings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ctx.applicationSettings = settings
	}
	return ctx.applicationSettings, nil
}

// FinalSettings returns the changes made to the relation settings (unit and application)
func (ctx *ContextRelation) FinalSettings() (unitSettings, appSettings params.Settings) {
	if ctx.applicationSettings != nil && ctx.applicationSettings.IsDirty() {
		appSettings = ctx.applicationSettings.FinalResult()
	}
	if ctx.settings != nil {
		unitSettings = ctx.settings.FinalResult()
	}
	return unitSettings, appSettings
}

// Suspended returns true if the relation is suspended.
func (ctx *ContextRelation) Suspended() bool {
	return ctx.ru.Relation().Suspended()
}

// SetStatus sets the relation's status.
func (ctx *ContextRelation) SetStatus(status relation.Status) error {
	return errors.Trace(ctx.ru.Relation().SetStatus(status))
}

// RemoteApplicationName returns the application on the other end of this
// relation from the perspective of this unit.
func (ctx *ContextRelation) RemoteApplicationName() string {
	return ctx.ru.Relation().OtherApplication()
}

// Life returns the relation's current life state.
func (ctx *ContextRelation) Life() life.Value {
	return ctx.ru.Relation().Life()
}
