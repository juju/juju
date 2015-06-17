// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tags

import "github.com/juju/names"

const (
	// JujuTagPrefix is the prefix for Juju-managed tags.
	JujuTagPrefix = "juju-"

	// JujuEnv is the tag name used for identifying the
	// Juju environment a resource is part of.
	JujuEnv = JujuTagPrefix + "env-uuid"

	// JujuStateServer is the tag name used for determining
	// whether a machine instance is a state server or not.
	JujuStateServer = JujuTagPrefix + "is-state"

	// JujuUnitsDeployed is the tag name used for identifying
	// the units deployed to a machine instance.
	JujuUnitsDeployed = JujuTagPrefix + "units-deployed"

	// JujuStorageInstance is the tag name used for identifying
	// the Juju storage instance that an IaaS storage resource
	// is assigned to.
	JujuStorageInstance = JujuTagPrefix + "storage-instance"

	// JujuStorageOwner is the tag name used for identifying
	// the service or unit that owns the Juju storage instance
	// that an IaaS storage resource is assigned to.
	JujuStorageOwner = JujuTagPrefix + "storage-owner"
)

// ResourceTagger is an interface that can provide resource tags.
type ResourceTagger interface {
	// ResourceTags returns a set of resource tags, and a
	// flag indicating whether or not any resource tags are
	// available.
	ResourceTags() (map[string]string, bool)
}

// ResourceTags returns tags to set on an infrastructure resource
// for the specified Juju environment.
func ResourceTags(e names.EnvironTag, taggers ...ResourceTagger) map[string]string {
	allTags := make(map[string]string)
	for _, tagger := range taggers {
		tags, ok := tagger.ResourceTags()
		if !ok {
			continue
		}
		for k, v := range tags {
			allTags[k] = v
		}
	}
	allTags[JujuEnv] = e.Id()
	return allTags
}
