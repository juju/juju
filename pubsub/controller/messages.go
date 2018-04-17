// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "github.com/juju/juju/controller"

// ConfigChanged messages are published by the apiserver client controller
// facade whenever the controller config is updated.
// data: `ConfigChangedMessage`
const ConfigChanged = "controller.config-changed"

// ConfigChangedMessage contains the controller.Config as it is after
// the update. Despite the controller.Config being a map[string]interface{},
// which also happens to be the default pubsub message payload, we wrap it
// in a structure because the central hub annotates the serialised data structure
// with, at least, the origin of the message.
type ConfigChangedMessage struct {
	Config controller.Config
	// TODO(thumper): add a version int to allow out of order messages.
	// Out of order could occur if two events happen simultaneously on two
	// different machines, and the forwarding of those messages cross each other.
	// Adding a version could allow subscribers to ignore lower versioned messages.
}
