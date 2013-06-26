// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strconv"

	"launchpad.net/juju-core/state/apiserver/common"
)

// FakeResourceRegistry implements the common.ResourceRegistry interface.
type FakeResourceRegistry map[string]common.Resource

func (registry FakeResourceRegistry) Register(resource common.Resource) string {
	id := strconv.Itoa(len(registry))
	registry[id] = resource
	return id
}

var _ (common.ResourceRegistry) = (*FakeResourceRegistry)(nil)
