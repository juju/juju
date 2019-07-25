// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

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

func (ctx *ContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", ctx.endpointName, ctx.relationId)
}

func (ctx *ContextRelation) UnitNames() []string {
	return ctx.cache.MemberNames()
}

func (ctx *ContextRelation) ReadSettings(unit string) (settings params.Settings, err error) {
	return ctx.cache.Settings(unit)
}

func (ctx *ContextRelation) Settings() (jujuc.Settings, error) {
	if ctx.settings == nil {
		node, err := ctx.ru.Settings()
		if err != nil {
			return nil, err
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

func (ctx *ContextRelation) ApplicationSettings() (jujuc.Settings, error) {
	if ctx.applicationSettings == nil {
		settings, err := ctx.ru.ApplicationSettings()
		if err != nil {
			return nil, err
		}
		ctx.applicationSettings = settings
	}
	return ctx.applicationSettings, nil
}

// WriteSettings persists all changes made to the relation settings (unit and application)
func (ctx *ContextRelation) WriteSettings() error {
	var appSettings params.Settings
	if ctx.applicationSettings != nil {
		appSettings = ctx.applicationSettings.FinalResult()
	}
	var unitSettings params.Settings
	if ctx.settings != nil {
		unitSettings = ctx.settings.FinalResult()
	}
	return ctx.ru.UpdateRelationSettings(unitSettings, appSettings)
}

// Suspended returns true if the relation is suspended.
func (ctx *ContextRelation) Suspended() bool {
	return ctx.ru.Relation().Suspended()
}

// SetStatus sets the relation's status.
func (ctx *ContextRelation) SetStatus(status relation.Status) error {
	return ctx.ru.Relation().SetStatus(status)
}
