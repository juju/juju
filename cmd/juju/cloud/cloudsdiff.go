// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	jujucloud "github.com/juju/juju/cloud"
)

func diffCloudDetails(cloudName string, new, old jujucloud.Cloud, diff *changes) {
	sameAuthTypes := func() bool {
		if len(old.AuthTypes) != len(new.AuthTypes) {
			return false
		}
		newAuthTypes := set.NewStrings()
		for _, one := range new.AuthTypes {
			newAuthTypes.Add(string(one))
		}

		for _, anOldOne := range old.AuthTypes {
			if !newAuthTypes.Contains(string(anOldOne)) {
				return false
			}
		}
		return true
	}

	endpointChanged := new.Endpoint != old.Endpoint
	identityEndpointChanged := new.IdentityEndpoint != old.IdentityEndpoint
	storageEndpointChanged := new.StorageEndpoint != old.StorageEndpoint

	if endpointChanged || identityEndpointChanged || storageEndpointChanged || new.Type != old.Type || !sameAuthTypes() {
		diff.addChange(updateChange, attributeScope, cloudName)
	}

	formatCloudRegion := func(rName string) string {
		return fmt.Sprintf("%v/%v", cloudName, rName)
	}

	oldRegions := mapRegions(old.Regions)
	newRegions := mapRegions(new.Regions)
	// added & modified regions
	for newName, newRegion := range newRegions {
		oldRegion, ok := oldRegions[newName]
		if !ok {
			diff.addChange(addChange, regionScope, formatCloudRegion(newName))
			continue

		}
		if (oldRegion.Endpoint != newRegion.Endpoint) || (oldRegion.IdentityEndpoint != newRegion.IdentityEndpoint) || (oldRegion.StorageEndpoint != newRegion.StorageEndpoint) {
			diff.addChange(updateChange, regionScope, formatCloudRegion(newName))
		}
	}

	// deleted regions
	for oldName := range oldRegions {
		if _, ok := newRegions[oldName]; !ok {
			diff.addChange(deleteChange, regionScope, formatCloudRegion(oldName))
		}
	}
}

func mapRegions(regions []jujucloud.Region) map[string]jujucloud.Region {
	result := make(map[string]jujucloud.Region)
	for _, region := range regions {
		result[region.Name] = region
	}
	return result
}

type changeType string

const (
	addChange    changeType = "added"
	deleteChange changeType = "deleted"
	updateChange changeType = "changed"
)

type scope string

const (
	cloudScope     scope = "cloud"
	regionScope    scope = "cloud region"
	attributeScope scope = "cloud attribute"
)

type changes struct {
	all map[changeType]map[scope][]string
}

func newChanges() *changes {
	return &changes{make(map[changeType]map[scope][]string)}
}

func (c *changes) addChange(aType changeType, entity scope, details string) {
	byType, ok := c.all[aType]
	if !ok {
		byType = make(map[scope][]string)
		c.all[aType] = byType
	}
	byType[entity] = append(byType[entity], details)
}

func (c *changes) summary() string {
	if len(c.all) == 0 {
		return ""
	}

	// Sort by change types
	types := []string{}
	for one := range c.all {
		types = append(types, string(one))
	}
	sort.Strings(types)

	msgs := []string{}
	details := ""
	tabSpace := "    "
	detailsSeparator := fmt.Sprintf("\n%v%v- ", tabSpace, tabSpace)
	for _, aType := range types {
		typeGroup := c.all[changeType(aType)]
		entityMsgs := []string{}

		// Sort by change scopes
		scopes := []string{}
		for one := range typeGroup {
			scopes = append(scopes, string(one))
		}
		sort.Strings(scopes)

		for _, aScope := range scopes {
			scopeGroup := typeGroup[scope(aScope)]
			sort.Strings(scopeGroup)
			entityMsgs = append(entityMsgs, adjustPlurality(aScope, len(scopeGroup)))
			details += fmt.Sprintf("\n%v%v %v:%v%v",
				tabSpace,
				aType,
				aScope,
				detailsSeparator,
				strings.Join(scopeGroup, detailsSeparator))
		}
		typeMsg := formatSlice(entityMsgs, ", ", " and ")
		msgs = append(msgs, fmt.Sprintf("%v %v", typeMsg, aType))
	}

	result := formatSlice(msgs, "; ", " as well as ")
	return fmt.Sprintf("%v:\n%v", result, details)
}

// TODO(anastasiamac 2014-04-13) Move this to
// juju/utils (eg. Pluralize). Added tech debt card.
func adjustPlurality(entity string, count int) string {
	switch count {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%d %v", count, entity)
	default:
		return fmt.Sprintf("%d %vs", count, entity)
	}
}

func formatSlice(slice []string, itemSeparator, lastSeparator string) string {
	switch len(slice) {
	case 0:
		return ""
	case 1:
		return slice[0]
	default:
		return fmt.Sprintf("%v%v%v",
			strings.Join(slice[:len(slice)-1], itemSeparator),
			lastSeparator,
			slice[len(slice)-1])
	}
}
