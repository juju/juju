package charmstore

import (
	"bytes"

	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

var _ csWrapper = (*fakeWrapper)(nil)

// fakeWrapper is an implementation of the csWrapper interface for use with
// testing the Client.
// We store the args & returns per channel, since we're running off
// map iteration and don't know the order they'll be requested in.
// The code assumes there are only two channels used - "stable" and
// "development".
type fakeWrapper struct {
	stableStub *testing.Stub
	devStub    *testing.Stub

	ReturnLatestStable []charmrepo.CharmRevision
	ReturnLatestDev    []charmrepo.CharmRevision

	ReturnListResourcesStable map[string][]params.Resource
	ReturnListResourcesDev    map[string][]params.Resource

	ReturnGetResource csclient.ResourceData

	ReturnResourceInfo params.Resource
}

func (f *fakeWrapper) Latest(channel params.Channel, ids []*charm.URL, headers map[string]string) ([]charmrepo.CharmRevision, error) {
	if channel == "stable" {
		f.stableStub.AddCall("Latest", channel, ids, headers)
		return f.ReturnLatestStable, nil
	}
	f.devStub.AddCall("Latest", channel, ids, headers)
	return f.ReturnLatestDev, nil
}

func (f *fakeWrapper) ListResources(channel params.Channel, ids []*charm.URL) (map[string][]params.Resource, error) {
	if channel == "stable" {
		f.stableStub.AddCall("ListResources", channel, ids)
		return f.ReturnListResourcesStable, nil
	}
	f.devStub.AddCall("ListResources", channel, ids)
	return f.ReturnListResourcesDev, nil
}

func (f *fakeWrapper) GetResource(channel params.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	f.stableStub.AddCall("GetResource", channel, id, name, revision)
	return f.ReturnGetResource, nil
}

func (f *fakeWrapper) ResourceInfo(channel params.Channel, id *charm.URL, name string, revision int) (params.Resource, error) {
	f.stableStub.AddCall("ResourceInfo", channel, id, name, revision)
	return f.ReturnResourceInfo, nil
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
