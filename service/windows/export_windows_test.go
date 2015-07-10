// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package windows

import (
	"github.com/juju/testing"
)

func PatchMgrConnect(patcher patcher, stub *testing.Stub) *StubMgr {
	conn := &StubMgr{Stub: stub}
	patcher.PatchValue(&newManager, func() (windowsManager, error) { return conn, nil })
	return conn
}

func PatchGetPassword(patcher patcher, stub *testing.Stub) *StubGetPassword {
	p := &StubGetPassword{Stub: stub}
	patcher.PatchValue(&getPassword, p.GetPassword)
	return p
}
