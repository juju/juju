// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/resources_mock.go github.com/juju/juju/state Resources

var _ = gc.Suite(&ResourcesSuite{})

type ResourcesSuite struct {
	ConnSuite

	ch *state.Charm
}

func (s *ResourcesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.ch = s.ConnSuite.AddTestingCharm(c, "starsay")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "starsay",
		Charm: s.ch,
	})
}

func newResource(c *gc.C, name, data string) resources.Resource {
	opened := resourcetesting.NewResource(c, nil, name, "wordpress", data)
	res := opened.Resource
	res.Timestamp = time.Unix(res.Timestamp.Unix(), 0)
	return res
}

func newResourceFromCharm(ch charm.Charm, name string) resources.Resource {
	return resources.Resource{
		Resource: charmresource.Resource{
			Meta:   ch.Meta().Resources[name],
			Origin: charmresource.OriginUpload,
		},
		ID:            "starsay/" + name,
		ApplicationID: "starsay",
	}
}

func (s *ResourcesSuite) TestListResources(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)

	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{spam},
	})
}

func (s *ResourcesSuite) TestListResourcesNoResources(c *gc.C) {
	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultRes.Resources, gc.HasLen, 0)
}

func (s *ResourcesSuite) TestListResourcesIgnorePending(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)

	ham := newResource(c, "install-resource", "install-resource")
	_, err = res.AddPendingResource("wordpress", "user", ham.Resource)
	c.Assert(err, jc.ErrorIsNil)
	csResources := []charmresource.Resource{spam.Resource}
	err = res.SetCharmStoreResources("wordpress", csResources, testing.NonZeroTime())
	c.Assert(err, jc.ErrorIsNil)

	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources:           []resources.Resource{spam},
		CharmStoreResources: csResources,
	})
}

func (s *ResourcesSuite) TestListPendingResources(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)

	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, jc.ErrorIsNil)

	resultRes, err := res.ListPendingResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	ham.PendingID = pendingID
	ham.Username = ""
	ham.Timestamp = resultRes.Resources[0].Timestamp
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{ham},
	})
}

func (s *ResourcesSuite) TestUpdatePending(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))

	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, jc.ErrorIsNil)

	data := "spamspamspam"
	ham.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	ham.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)

	r, err := res.UpdatePendingResource("wordpress", pendingID, ham.Username, ham.Resource, bytes.NewBufferString(data))
	c.Assert(err, jc.ErrorIsNil)

	ham.Timestamp = r.Timestamp
	ham.PendingID = pendingID
	c.Assert(r, jc.DeepEquals, ham)
}

func (s *ResourcesSuite) TestGetResource(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	_, err := res.GetResource("wordpress", "store-resource")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)

	r, err := res.GetResource("wordpress", "store-resource")
	c.Assert(err, jc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, jc.DeepEquals, spam)
}

func (s *ResourcesSuite) TestGetPendingResource(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	ham := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", ham.Username, ham.Resource)
	c.Assert(err, jc.ErrorIsNil)

	r, err := res.GetPendingResource("wordpress", "install-resource", pendingID)
	c.Assert(err, jc.ErrorIsNil)
	ham.PendingID = pendingID
	ham.Username = ""
	ham.Timestamp = r.Timestamp
	c.Assert(r, jc.DeepEquals, ham)
}

func (s *ResourcesSuite) TestSetResource(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	app, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.CharmModifiedVersion(), gc.Equals, 0)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))

	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	file := bytes.NewBufferString(data)

	_, err = res.AddPendingResource("wordpress", "user", spam.Resource)
	c.Assert(err, jc.ErrorIsNil)
	r, err := res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, jc.DeepEquals, spam)
	c.Assert(r.PendingID, gc.Equals, "")

	r, err = res.GetResource("wordpress", "store-resource")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.PendingID, gc.Equals, "")

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.CharmModifiedVersion(), gc.Equals, 1)
}

func (s *ResourcesSuite) TestSetCharmStoreResources(c *gc.C) {
	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	updatedRes := newResourceFromCharm(s.ch, "store-resource")
	updatedRes.Revision = 666
	csResources := []charmresource.Resource{updatedRes.Resource}
	err := res.SetCharmStoreResources("starsay", csResources, testing.NonZeroTime())
	c.Assert(err, jc.ErrorIsNil)

	resultRes, err := res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	sort.Slice(resultRes.Resources, func(i, j int) bool {
		return resultRes.Resources[i].Name < resultRes.Resources[j].Name
	})
	sort.Slice(resultRes.CharmStoreResources, func(i, j int) bool {
		return resultRes.CharmStoreResources[i].Name < resultRes.CharmStoreResources[j].Name
	})

	expected := []resources.Resource{
		newResourceFromCharm(s.ch, "install-resource"),
		newResourceFromCharm(s.ch, "store-resource"),
		newResourceFromCharm(s.ch, "upload-resource"),
	}
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources: expected,
		CharmStoreResources: []charmresource.Resource{
			expected[0].Resource,
			updatedRes.Resource,
			expected[2].Resource,
		},
	})
}

func (s *ResourcesSuite) TestUnitResource(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))
	data := "spamspamspam"
	spam := newResource(c, "store-resource", data)
	_, err := res.SetUnitResource("wordpress/0", spam.Username, spam.Resource)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	file := bytes.NewBufferString(data)
	_, err = res.SetResource("wordpress", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)

	r, err := res.SetUnitResource("wordpress/0", spam.Username, spam.Resource)
	c.Assert(err, jc.ErrorIsNil)
	spam.Timestamp = r.Timestamp
	c.Assert(r, jc.DeepEquals, spam)
	resultRes, err := res.ListResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	spam.Timestamp = resultRes.Resources[0].Timestamp
	resultRes.UnitResources[0].Resources[0].Timestamp = spam.Timestamp
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources: []resources.Resource{spam},
		UnitResources: []resources.UnitResources{{
			Tag:       names.NewUnitTag("wordpress/0"),
			Resources: []resources.Resource{spam},
		}},
	})
}

func (s *ResourcesSuite) TestOpenResource(c *gc.C) {
	app, err := s.State.Application("starsay")
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))

	_, _, err = res.OpenResource("starsay", "install-resource")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	spam := newResourceFromCharm(s.ch, "install-resource")
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("starsay", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)
	_, err = res.SetUnitResource("starsay/0", spam.Username, spam.Resource)
	c.Assert(err, jc.ErrorIsNil)

	r, rdr, err := res.OpenResource("starsay", "install-resource")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	spam.Timestamp = r.Timestamp
	c.Assert(r, jc.DeepEquals, spam)

	resData, err := io.ReadAll(rdr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(resData), gc.Equals, data)

	resultRes, err := res.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultRes.Resources, gc.HasLen, 3)

	sort.Slice(resultRes.Resources, func(i, j int) bool {
		return resultRes.Resources[i].Name < resultRes.Resources[j].Name
	})
	sort.Slice(resultRes.CharmStoreResources, func(i, j int) bool {
		return resultRes.CharmStoreResources[i].Name < resultRes.CharmStoreResources[j].Name
	})

	expected := []resources.Resource{
		newResourceFromCharm(s.ch, "install-resource"),
		newResourceFromCharm(s.ch, "store-resource"),
		newResourceFromCharm(s.ch, "upload-resource"),
	}
	chRes := []charmresource.Resource{
		expected[0].Resource,
		expected[1].Resource,
		expected[2].Resource,
	}
	expected[0].Resource = spam.Resource
	expected[0].Timestamp = resultRes.Resources[0].Timestamp

	resultRes.UnitResources[0].Resources[0].Timestamp = spam.Timestamp

	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		Resources:           expected,
		CharmStoreResources: chRes,
		UnitResources: []resources.UnitResources{{
			Tag:       names.NewUnitTag("starsay/0"),
			Resources: []resources.Resource{spam},
		}},
	})
}

func (s *ResourcesSuite) TestOpenResourceForUniter(c *gc.C) {
	app, err := s.State.Application("starsay")
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))

	spam := newResourceFromCharm(s.ch, "install-resource")
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)
	file := bytes.NewBufferString(data)
	_, err = res.SetResource("starsay", spam.Username, spam.Resource, file, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)
	_, err = res.SetUnitResource("starsay/0", spam.Username, spam.Resource)
	c.Assert(err, jc.ErrorIsNil)

	unitRes, rdr, err := res.OpenResourceForUniter("starsay/0", "install-resource")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	buf := make([]byte, 2)
	_, err = rdr.Read(buf)
	c.Assert(err, jc.ErrorIsNil)

	resultRes, err := res.ListPendingResources("starsay")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resultRes.UnitResources, gc.HasLen, 1)
	c.Assert(resultRes.UnitResources[0].Resources, gc.HasLen, 1)
	resultRes.UnitResources[0].Resources[0].PendingID = ""
	c.Assert(resultRes, jc.DeepEquals, resources.ApplicationResources{
		UnitResources: []resources.UnitResources{{
			Tag:              names.NewUnitTag("starsay/0"),
			Resources:        []resources.Resource{unitRes},
			DownloadProgress: map[string]int64{"install-resource": 2},
		}},
	})
}

func (s *ResourcesSuite) TestRemovePendingAppResources(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

	res := s.State.Resources(state.NewObjectStore(c, s.State.ModelUUID()))

	spam := newResource(c, "install-resource", "install-resource")
	pendingID, err := res.AddPendingResource("wordpress", spam.Username, spam.Resource)
	c.Assert(err, jc.ErrorIsNil)

	// Add some data so we force a cleanup.
	data := "spamspamspam"
	spam.Size = int64(len(data))
	sha384hash := sha512.New384()
	sha384hash.Write([]byte(data))
	fp := fmt.Sprintf("%x", sha384hash.Sum(nil))
	spam.Fingerprint, err = charmresource.ParseFingerprint(fp)
	c.Assert(err, jc.ErrorIsNil)

	_, err = res.UpdatePendingResource("wordpress", pendingID, spam.Username, spam.Resource, bytes.NewBufferString(data))
	c.Assert(err, jc.ErrorIsNil)

	err = res.RemovePendingAppResources("wordpress", map[string]string{"install-resource": pendingID})
	c.Assert(err, jc.ErrorIsNil)

	resources, err := res.ListPendingResources("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resources.Resources, gc.HasLen, 0)

	state.AssertCleanupsWithKind(c, s.State, "resourceBlob")
}
