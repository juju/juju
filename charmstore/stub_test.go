// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"bytes"
	"net/url"

	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/resource"
	"github.com/juju/charmrepo/v5/csclient"
	"github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/testing"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"
)

var _ csWrapper = (*fakeWrapper)(nil)

type resourceResult struct {
	resources []params.Resource
	err       error
}

func oneResourceResult(r params.Resource) resourceResult {
	return resourceResult{
		resources: []params.Resource{r},
	}
}

// fakeWrapper is an implementation of the csWrapper interface for use with
// testing the Client.
// We store the args & returns per channel, since we're running off
// map iteration and don't know the order they'll be requested in.
// The code assumes there are only two channels used - "stable" and
// "development".
type fakeWrapper struct {
	server *url.URL

	stub       *testing.Stub
	stableStub *testing.Stub
	devStub    *testing.Stub

	ReturnLatestStable [][]params.CharmRevision
	ReturnLatestDev    [][]params.CharmRevision

	ReturnListResourcesStable []resourceResult
	ReturnListResourcesDev    []resourceResult

	ReturnGetResource csclient.ResourceData

	ReturnResourceMeta params.Resource
}

func (f *fakeWrapper) makeWrapper(bakeryClient *httpbakery.Client, server string) (csWrapper, error) {
	f.stub.AddCall("makeWrapper", bakeryClient, server)
	return f, nil
}

func (f *fakeWrapper) ServerURL() string {
	if f.server != nil {
		return f.server.String()
	}
	return csclient.ServerURL
}

// this code only returns the first value the return slices to support
func (f *fakeWrapper) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	if channel == "stable" {
		f.stableStub.AddCall("Latest", channel, ids, headers)
		ret := f.ReturnLatestStable[0]
		f.ReturnLatestStable = f.ReturnLatestStable[1:]
		return ret, nil
	}
	f.devStub.AddCall("Latest", channel, ids, headers)
	ret := f.ReturnLatestDev[0]
	f.ReturnLatestDev = f.ReturnLatestDev[1:]
	return ret, nil
}

func (f *fakeWrapper) ListResources(channel params.Channel, id *charm.URL) ([]params.Resource, error) {
	if channel == "stable" {
		f.stableStub.AddCall("ListResources", channel, id)
		ret := f.ReturnListResourcesStable[0]
		f.ReturnListResourcesStable = f.ReturnListResourcesStable[1:]
		return ret.resources, ret.err
	}
	f.devStub.AddCall("ListResources", channel, id)
	ret := f.ReturnListResourcesDev[0]
	f.ReturnListResourcesDev = f.ReturnListResourcesDev[1:]
	return ret.resources, ret.err
}

func (f *fakeWrapper) GetResource(channel params.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	f.stub.AddCall("GetResource", channel, id, name, revision)
	return f.ReturnGetResource, nil
}

func (f *fakeWrapper) ResourceMeta(channel params.Channel, id *charm.URL, name string, revision int) (params.Resource, error) {
	f.stub.AddCall("ResourceMeta", channel, id, name, revision)
	return f.ReturnResourceMeta, nil
}

func fakeParamsResource(name string, data []byte) params.Resource {
	fp, err := resource.GenerateFingerprint(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return params.Resource{
		Name:        name,
		Type:        "file",
		Path:        name + ".tgz",
		Description: "something about " + name,
		Revision:    len(name),
		Fingerprint: fp.Bytes(),
		Size:        int64(len(data)),
	}
}

type fakeMacCache struct {
	stub *testing.Stub

	ReturnGet macaroon.Slice
}

func (f *fakeMacCache) Set(u *charm.URL, m macaroon.Slice) error {
	f.stub.AddCall("Set", u, m)
	return f.stub.NextErr()
}

func (f *fakeMacCache) Get(u *charm.URL) (macaroon.Slice, error) {
	f.stub.AddCall("Get", u)
	if err := f.stub.NextErr(); err != nil {
		return nil, err
	}
	return f.ReturnGet, nil
}
