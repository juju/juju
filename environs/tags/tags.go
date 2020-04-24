// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tags

import "github.com/juju/names/v4"

const (
	// JujuTagPrefix is the prefix for Juju-managed tags.
	JujuTagPrefix = "juju-"

	// JujuModel is the tag name used for identifying the
	// Juju model a resource is part of.
	JujuModel = JujuTagPrefix + "model-uuid"

	// JujuController is the tag name used for identifying the
	// Juju controller that manages a resource.
	JujuController = JujuTagPrefix + "controller-uuid"

	// JujuIsController is the tag name used for determining
	// whether a machine instance is a controller or not.
	JujuIsController = JujuTagPrefix + "is-controller"

	// JujuUnitsDeployed is the tag name used for identifying
	// the units deployed to a machine instance. The value is
	// a space-separated list of the unit names.
	JujuUnitsDeployed = JujuTagPrefix + "units-deployed"

	// JujuStorageInstance is the tag name used for identifying
	// the Juju storage instance that an IaaS storage resource
	// is assigned to.
	JujuStorageInstance = JujuTagPrefix + "storage-instance"

	// JujuStorageOwner is the tag name used for identifying
	// the application or unit that owns the Juju storage instance
	// that an IaaS storage resource is assigned to.
	JujuStorageOwner = JujuTagPrefix + "storage-owner"

	// JujuMachine is the tag name used for identifying
	// the model and machine id corresponding to the
	// provisioned machine instance.
	JujuMachine = JujuTagPrefix + "machine-id"
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
func ResourceTags(modelTag names.ModelTag, controllerTag names.ControllerTag, taggers ...ResourceTagger) map[string]string {
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
	allTags[JujuModel] = modelTag.Id()
	allTags[JujuController] = controllerTag.Id()
	return allTags
}
