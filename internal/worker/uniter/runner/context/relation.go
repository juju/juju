// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
)

type RelationUnit interface {
	// ApplicationSettings returns a Settings which allows access to this unit's
	// application settings within the relation.
	ApplicationSettings(context.Context) (*uniter.Settings, error)

	// Endpoint returns the relation endpoint that defines the unit's
	// participation in the relation.
	Endpoint() uniter.Endpoint

	// ReadSettings returns a map holding the settings of the unit with the
	// supplied name within this relation.
	ReadSettings(ctx context.Context, name string) (params.Settings, error)

	// Relation returns the relation associated with the unit.
	Relation() api.Relation

	// Settings returns a Settings which allows access to the unit's settings
	// within the relation.
	Settings(context.Context) (*uniter.Settings, error)
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
	broken       bool

	// settings allows read and write access to the relation unit settings.
	settings *uniter.Settings

	// applicationSettings allows read and write access to the relation application settings.
	applicationSettings *uniter.Settings

	// cache holds remote unit membership and settings.
	cache *RelationCache
}

// NewContextRelation creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewContextRelation(ru RelationUnit, cache *RelationCache, broken bool) *ContextRelation {
	return &ContextRelation{
		ru:           ru,
		relationId:   ru.Relation().Id(),
		endpointName: ru.Endpoint().Name,
		cache:        cache,
		broken:       broken,
	}
}

func (c *ContextRelation) Id() int {
	return c.relationId
}

func (c *ContextRelation) Name() string {
	return c.endpointName
}

func (c *ContextRelation) RelationTag() names.RelationTag {
	return c.ru.Relation().Tag()
}

func (c *ContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", c.endpointName, c.relationId)
}

func (c *ContextRelation) UnitNames() []string {
	return c.cache.MemberNames()
}

func (c *ContextRelation) ReadSettings(ctx context.Context, unit string) (settings params.Settings, err error) {
	return c.cache.Settings(ctx, unit)
}

func (c *ContextRelation) ReadApplicationSettings(ctx context.Context, app string) (settings params.Settings, err error) {
	return c.cache.ApplicationSettings(ctx, app)
}

func (c *ContextRelation) Settings(ctx context.Context) (jujuc.Settings, error) {
	if c.settings == nil {
		node, err := c.ru.Settings(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.settings = node
	}
	return c.settings, nil
}

func (c *ContextRelation) ApplicationSettings(ctx context.Context) (jujuc.Settings, error) {
	if c.applicationSettings == nil {
		settings, err := c.ru.ApplicationSettings(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.applicationSettings = settings
	}
	return c.applicationSettings, nil
}

// FinalSettings returns the changes made to the relation settings (unit and application)
func (c *ContextRelation) FinalSettings() (unitSettings, appSettings params.Settings) {
	if c.applicationSettings != nil && c.applicationSettings.IsDirty() {
		appSettings = c.applicationSettings.FinalResult()
	}
	if c.settings != nil {
		unitSettings = c.settings.FinalResult()
	}
	return unitSettings, appSettings
}

// Suspended returns true if the relation is suspended.
func (c *ContextRelation) Suspended() bool {
	return c.ru.Relation().Suspended()
}

// SetStatus sets the relation's status.
func (c *ContextRelation) SetStatus(ctx context.Context, status relation.Status) error {
	return errors.Trace(c.ru.Relation().SetStatus(ctx, status))
}

// RemoteApplicationName returns the application on the other end of this
// relation from the perspective of this unit.
func (c *ContextRelation) RemoteApplicationName() string {
	return c.ru.Relation().OtherApplication()
}

// RemoteModelUUID returns the UUID of the model hosting the
// application on the other end of the relation.
func (ctx *ContextRelation) RemoteModelUUID() string {
	return ctx.ru.Relation().OtherModelUUID()
}

// Life returns the relation's current life state.
func (c *ContextRelation) Life() life.Value {
	return c.ru.Relation().Life()
}
