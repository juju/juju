// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/worker/caasoperator/commands"
)

type RelationInfo struct {
	RelationUnitAPI relationUnitAPI
	MemberNames     []string
}

// ContextRelation is the implementation of hooks.ContextRelation.
type ContextRelation struct {
	relationUnitAPI relationUnitAPI
	relationId      int
	endpointName    string

	// settings allows read and write access to the relation unit settings.
	settings commands.Settings

	// cache holds remote unit membership and settings.
	cache *RelationCache
}

// NewContextRelation creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewContextRelation(relationAPI relationUnitAPI, cache *RelationCache) *ContextRelation {
	return &ContextRelation{
		relationUnitAPI: relationAPI,
		relationId:      relationAPI.Id(),
		endpointName:    relationAPI.Endpoint(),
		cache:           cache,
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

func (ctx *ContextRelation) RemoteSettings(unit string) (commands.Settings, error) {
	return ctx.cache.Settings(unit)
}

func (ctx *ContextRelation) LocalSettings() (commands.Settings, error) {
	if ctx.settings == nil {
		node, err := ctx.relationUnitAPI.LocalSettings()
		if err != nil {
			return nil, err
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

// WriteSettings persists all changes made to the unit's relation settings.
func (ctx *ContextRelation) WriteSettings() (err error) {
	if ctx.settings != nil {
		err = ctx.relationUnitAPI.WriteSettings(ctx.settings)
	}
	return
}

// Suspended returns true if the relation is suspended.
func (ctx *ContextRelation) Suspended() bool {
	return ctx.relationUnitAPI.Suspended()
}

// SetStatus sets the relation's status.
func (ctx *ContextRelation) SetStatus(status relation.Status) error {
	return ctx.relationUnitAPI.SetStatus(status)
}
