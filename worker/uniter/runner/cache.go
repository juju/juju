// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"sort"

	"github.com/juju/juju/apiserver/params"
)

// SettingsFunc returns the relation settings for a unit.
type SettingsFunc func(unitName string) (params.Settings, error)

// SettingsMap is a map from unit name to relation settings.
type SettingsMap map[string]params.Settings

// RelationCache stores a relation's remote unit membership and settings.
// Member settings are stored until invalidated or removed by name; settings
// of non-member units are stored only until the cache is pruned.
type RelationCache struct {
	// readSettings is used to get settings data if when not already present.
	readSettings SettingsFunc
	// members' keys define the relation's membership; non-nil values hold
	// cached settings.
	members SettingsMap
	// others is a short-term cache for non-member settings.
	others SettingsMap
}

// NewRelationCache creates a new RelationCache that will use the supplied
// SettingsFunc to populate itself on demand. Initial membership is determined
// by memberNames.
func NewRelationCache(readSettings SettingsFunc, memberNames []string) *RelationCache {
	cache := &RelationCache{
		readSettings: readSettings,
	}
	cache.Prune(memberNames)
	return cache
}

// Prune resets the membership to the supplied list, and discards the settings
// of all non-member units.
func (cache *RelationCache) Prune(memberNames []string) {
	newMembers := SettingsMap{}
	for _, memberName := range memberNames {
		newMembers[memberName] = cache.members[memberName]
	}
	cache.members = newMembers
	cache.others = SettingsMap{}
}

// MemberNames returns the names of the remote units present in the relation.
func (cache *RelationCache) MemberNames() (memberNames []string) {
	for memberName := range cache.members {
		memberNames = append(memberNames, memberName)
	}
	sort.Strings(memberNames)
	return memberNames
}

// Settings returns the settings of the named remote unit. It's valid to get
// the settings of any unit that has ever been in the relation.
func (cache *RelationCache) Settings(unitName string) (params.Settings, error) {
	settings, isMember := cache.members[unitName]
	if settings == nil {
		if !isMember {
			settings = cache.others[unitName]
		}
		if settings == nil {
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
