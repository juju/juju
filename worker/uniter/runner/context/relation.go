// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type RelationInfo struct {
	RelationUnit *uniter.RelationUnit
	MemberNames  []string
}

// ContextRelation is the implementation of hooks.ContextRelation.
type ContextRelation struct {
	ru           *uniter.RelationUnit
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
func NewContextRelation(ru *uniter.RelationUnit, cache *RelationCache) *ContextRelation {
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

// WriteSettings persists all changes made to the relation settings (unit and application)
func (ctx *ContextRelation) WriteSettings() error {
	unitSettings, appSettings := ctx.FinalSettings()
	return errors.Trace(ctx.ru.UpdateRelationSettings(unitSettings, appSettings))
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
