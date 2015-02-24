// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/testing"

	"github.com/juju/juju/service/common"
)

type serviceInfo interface {
	Name() string
	Conf() common.Conf
}

type ServiceData struct {
	testing.Stub

	Installed []serviceInfo
	Removed   []serviceInfo

	Exists  bool
	Running bool
}

type stubService struct {
	*ServiceData

	name string
	conf common.Conf
}

func (ss *stubService) Name() string {
	return ss.name
}

func (ss *stubService) Conf() common.Conf {
	return ss.conf
}

func (ss *stubService) Install() error {
	ss.Stub.AddCall("Install")
	ss.ServiceData.Installed = append(ss.ServiceData.Installed, ss)

	return ss.NextErr()
}

func (ss *stubService) Start() error {
	ss.Stub.AddCall("Start")

	return ss.NextErr()
}

func (ss *stubService) Stop() error {
	ss.Stub.AddCall("Stop")

	return ss.NextErr()
}

func (ss *stubService) Remove() error {
	ss.Stub.AddCall("Remove")
	ss.ServiceData.Removed = append(ss.ServiceData.Removed, ss)

	return ss.NextErr()
}

func (ss *stubService) Exists() bool {
	ss.Stub.AddCall("Exists")

	ss.NextErr()
	return ss.ServiceData.Exists
}

func (ss *stubService) Running() bool {
	ss.Stub.AddCall("Running")

	ss.NextErr()
	return ss.ServiceData.Running
}
