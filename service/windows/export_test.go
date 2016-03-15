// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/testing"
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchServiceManager(patcher patcher, stub *testing.Stub) *StubSvcManager {
	manager := &StubSvcManager{Stub: stub}
	patcher.PatchValue(&NewServiceManager, func() (ServiceManager, error) { return manager, nil })
	patcher.PatchValue(&listServices, manager.ListServices)
	return manager
}
