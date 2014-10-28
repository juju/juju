// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"sort"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/jujuc"
)

// ContextRelation is the implementation of jujuc.ContextRelation.
type ContextRelation struct {
	ru           *uniter.RelationUnit
	relationId   int
	endpointName string

	// members contains settings for known relation members. Nil values
	// indicate members whose settings have not yet been cached.
	members SettingsMap

	// settings allows read and write access to the relation unit settings.
	settings *uniter.Settings

	// cache is a short-term cache that enables consistent access to settings
	// for units that are not currently participating in the relation. Its
	// contents should be cleared whenever a new hook is executed.
	cache SettingsMap
}

// NewContextRelation creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewContextRelation(ru *uniter.RelationUnit, members map[string]int64) *ContextRelation {
	ctx := &ContextRelation{
		ru:           ru,
		relationId:   ru.Relation().Id(),
		endpointName: ru.Endpoint().Name,
		members:      SettingsMap{},
	}
	for unit := range members {
		ctx.members[unit] = nil
	}
	ctx.ClearCache()
	return ctx
}

// WriteSettings persists all changes made to the unit's relation settings.
func (ctx *ContextRelation) WriteSettings() (err error) {
	if ctx.settings != nil {
		err = ctx.settings.Write()
	}
	return
}

// ClearCache discards all cached settings for units that are not members
// of the relation, and all unwritten changes to the unit's relation settings.
// including any changes to Settings that have not been written.
func (ctx *ContextRelation) ClearCache() {
	ctx.settings = nil
	ctx.cache = make(SettingsMap)
}

// UpdateMembers ensures that the context is aware of every supplied
// member unit. For each supplied member, the cached settings will be
// overwritten.
func (ctx *ContextRelation) UpdateMembers(members SettingsMap) {
	for m, s := range members {
		ctx.members[m] = s
	}
}

// DeleteMember drops the membership and cache of a single remote unit, without
// perturbing settings for the remaining members.
func (ctx *ContextRelation) DeleteMember(unitName string) {
	delete(ctx.members, unitName)
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

func (ctx *ContextRelation) UnitNames() (units []string) {
	for unit := range ctx.members {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
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

func (ctx *ContextRelation) ReadSettings(unit string) (settings params.RelationSettings, err error) {
	settings, member := ctx.members[unit]
	if settings == nil {
		if settings = ctx.cache[unit]; settings == nil {
			settings, err = ctx.ru.ReadSettings(unit)
			if err != nil {
				return nil, err
			}
		}
	}
	if member {
		ctx.members[unit] = settings
	} else {
		ctx.cache[unit] = settings
	}
	return settings, nil
}
