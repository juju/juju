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

func (registry FakeResourceRegistry) Get(id string) common.Resource {
	panic("unimplemented")
}

func (registry FakeResourceRegistry) Stop(id string) error {
	panic("unimplemented")
}

var _ (common.ResourceRegistry) = (*FakeResourceRegistry)(nil)
