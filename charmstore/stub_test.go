package charmstore

import (
	"bytes"
	"net/url"

	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
)

var _ csWrapper = (*fakeWrapper)(nil)

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

	ReturnListResourcesStable []map[string][]params.Resource
	ReturnListResourcesDev    []map[string][]params.Resource

	ReturnGetResource csclient.ResourceData

	ReturnResourceMeta params.Resource
}

func (f *fakeWrapper) makeWrapper(bakeryClient *httpbakery.Client, server *url.URL) csWrapper {
	f.stub.AddCall("makeWrapper", bakeryClient, server)
	return f
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

func (f *fakeWrapper) ListResources(channel params.Channel, ids []*charm.URL) (map[string][]params.Resource, error) {
	if channel == "stable" {
		f.stableStub.AddCall("ListResources", channel, ids)
		ret := f.ReturnListResourcesStable[0]
		f.ReturnListResourcesStable = f.ReturnListResourcesStable[1:]
		return ret, nil
	}
	f.devStub.AddCall("ListResources", channel, ids)
	ret := f.ReturnListResourcesDev[0]
	f.ReturnListResourcesDev = f.ReturnListResourcesDev[1:]
	return ret, nil
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
		Origin:      "store",
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
