// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"sort"

	"github.com/juju/juju/apiserver/params"
)

// SettingsFunc returns the relation settings for a unit.
type SettingsFunc func(unitName string) (params.RelationSettings, error)

// SettingsMap is a map from unit name to relation settings.
type SettingsMap map[string]params.RelationSettings

// RelationCache stores a relation's remote unit membership and settings.
// Member settings are stored until invalidated or removed by name; settings
// of other units are stored only until the cache is pruned.
type RelationCache struct {
	readSettings SettingsFunc
	members      SettingsMap
	others       SettingsMap
}

// NewRelationCache creates a new RelationCache that will use the supplied
// SettingsFunc to populate itself on demand. Initial membership is determined
// by memberNames.
func NewRelationCache(readSettings SettingsFunc, memberNames []string) *RelationCache {
	cache := &RelationCache{
		readSettings: readSettings,
		members:      SettingsMap{},
	}
	for _, memberName := range memberNames {
		cache.InvalidateMember(memberName)
	}
	cache.Prune()
	return cache
}

// MemberNames returns the names of the remote units present in the relation.
func (cache *RelationCache) MemberNames() (memberNames []string) {
	for memberName := range cache.members {
		memberNames = append(memberNames, memberName)
	}
	sort.Strings(memberNames)
	return memberNames
}

// Settings returns the settings of the named remote unit.
func (cache *RelationCache) Settings(unitName string) (params.RelationSettings, error) {
	settings, isMember := cache.members[unitName]
	if settings == nil {
		if settings = cache.others[unitName]; settings == nil {
			var err error
			settings, err = cache.readSettings(unitName)
			if err != nil {
				return nil, err
			}
		}
	}
	if isMember {
		cache.members[unitName] = settings
	} else {
		cache.others[unitName] = settings
	}
	return settings, nil
}

// InvalidateMember ensures that the named remote unit will be considered a
// member of the relation, and that the next attempt to read its settings will
// use fresh data.
func (cache *RelationCache) InvalidateMember(memberName string) {
	cache.members[memberName] = nil
}

// RemoveMember ensures that the named remote unit will not be considered a
// member of the relation,
func (cache *RelationCache) RemoveMember(memberName string) {
	delete(cache.members, memberName)
}

// Prune discards the settings of all non-member units.
func (cache *RelationCache) Prune() {
	cache.others = SettingsMap{}
}
