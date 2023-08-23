// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner ControllerConfigGetter

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

const (
	dontWait = time.Duration(0)
)

type byMachineAndEntity []params.MachineStorageId

func (b byMachineAndEntity) Len() int {
	return len(b)
}

func (b byMachineAndEntity) Less(i, j int) bool {
	if b[i].MachineTag == b[j].MachineTag {
		return b[i].AttachmentTag < b[j].AttachmentTag
	}
	return b[i].MachineTag < b[j].MachineTag
}

func (b byMachineAndEntity) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
