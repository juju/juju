// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/testing"
	"github.com/juju/utils/set"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/gate"
)

type fakeWorker struct {
	worker.Worker
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeFacade struct {
	stub *testing.Stub

	createSpaces params.ErrorResults
	addSubnets   params.ErrorResults
	listSpaces   params.DiscoverSpacesResults
	listSubnets  params.ListSubnetsResults
}

func (f *fakeFacade) CreateSpaces(args params.CreateSpacesParams) (params.ErrorResults, error) {
	f.stub.AddCall("CreateSpaces", args)
	return f.createSpaces, f.stub.NextErr()
}

func (f *fakeFacade) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	f.stub.AddCall("AddSubnets", args)
	return f.addSubnets, f.stub.NextErr()
}

func (f *fakeFacade) ListSpaces() (params.DiscoverSpacesResults, error) {
	f.stub.AddCall("ListSpaces")
	return f.listSpaces, f.stub.NextErr()
}

func (f *fakeFacade) ListSubnets(args params.SubnetsFilters) (params.ListSubnetsResults, error) {
	f.stub.AddCall("ListSubnets", args)
	return f.listSubnets, f.stub.NextErr()
}

type fakeNoNetworkEnviron struct {
	environs.Environ
}

type fakeEnviron struct {
	environs.NetworkingEnviron

	stub           *testing.Stub
	spaceDiscovery bool
	spaces         []network.SpaceInfo
}

func (e *fakeEnviron) SupportsSpaceDiscovery() (bool, error) {
	e.stub.AddCall("SupportsSpaceDiscovery")
	return e.spaceDiscovery, e.stub.NextErr()
}

func (e *fakeEnviron) Spaces() ([]network.SpaceInfo, error) {
	e.stub.AddCall("Spaces")
	return e.spaces, e.stub.NextErr()
}

func fakeNewName(_ string, _ set.Strings) string {
	panic("fake")
}

type fakeUnlocker struct {
	gate.Unlocker
}
