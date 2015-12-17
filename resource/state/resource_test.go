// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
)

var _ = gc.Suite(&ResourceSuite{})

type ResourceSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	raw     *stubRawState
	persist *stubPersistence
	storage *stubStorage
}

func (s *ResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.raw = &stubRawState{stub: s.stub}
	s.persist = &stubPersistence{stub: s.stub}
	s.storage = &stubStorage{stub: s.stub}
	s.raw.ReturnPersistence = s.persist
	s.raw.ReturnStorage = s.storage
}

func (s *ResourceSuite) TestListResourcesOkay(c *gc.C) {
	expected := newUploadResources(c, "spam", "eggs")
	s.persist.ReturnListResources = expected
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	resources, err := st.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources, jc.DeepEquals, expected)
	s.stub.CheckCallNames(c, "ListResources")
	s.stub.CheckCall(c, 0, "ListResources", "a-service")
}

func (s *ResourceSuite) TestListResourcesEmpty(c *gc.C) {
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	resources, err := st.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *ResourceSuite) TestListResourcesError(c *gc.C) {
	expected := newUploadResources(c, "spam", "eggs")
	s.persist.ReturnListResources = expected
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	_, err := st.ListResources("a-service")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *ResourceSuite) TestSetResourceOkay(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	hash := string(res.Fingerprint.Bytes())
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetResource("a-service", res, file)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStagedResource", "Put", "SetResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
	s.stub.CheckCall(c, 1, "Put", hash, file, res.Size)
	s.stub.CheckCall(c, 2, "SetResource", "a-service", res)
}

func (s *ResourceSuite) TestSetResourceBadResource(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	res.Timestamp = time.Time{}
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetResource("a-service", res, file)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource metadata.*`)
	s.stub.CheckNoCalls(c)
}

func (s *ResourceSuite) TestSetResourceStagingFailure(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(failure, nil, nil, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "SetStagedResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
}

func (s *ResourceSuite) TestSetResourcePutFailureBasic(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	hash := string(res.Fingerprint.Bytes())
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, failure, nil, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "SetStagedResource", "Put", "UnstageResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
	s.stub.CheckCall(c, 1, "Put", hash, file, res.Size)
	s.stub.CheckCall(c, 2, "UnstageResource", "a-service", res.Name)
}

func (s *ResourceSuite) TestSetResourcePutFailureExtra(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	hash := string(res.Fingerprint.Bytes())
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	extraErr := errors.New("<just not your day>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, failure, extraErr, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "SetStagedResource", "Put", "UnstageResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
	s.stub.CheckCall(c, 1, "Put", hash, file, res.Size)
	s.stub.CheckCall(c, 2, "UnstageResource", "a-service", res.Name)
}

func (s *ResourceSuite) TestSetResourceSetFailureBasic(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	hash := string(res.Fingerprint.Bytes())
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, failure, nil, nil, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "SetStagedResource", "Put", "SetResource", "Delete", "UnstageResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
	s.stub.CheckCall(c, 1, "Put", hash, file, res.Size)
	s.stub.CheckCall(c, 2, "SetResource", "a-service", res)
	s.stub.CheckCall(c, 3, "Delete", hash)
	s.stub.CheckCall(c, 4, "UnstageResource", "a-service", res.Name)
}

func (s *ResourceSuite) TestSetResourceSetFailureExtra(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	hash := string(res.Fingerprint.Bytes())
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	extraErr1 := errors.New("<just not your day>")
	extraErr2 := errors.New("<wow...just wow>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, failure, extraErr1, extraErr2, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "SetStagedResource", "Put", "SetResource", "Delete", "UnstageResource")
	s.stub.CheckCall(c, 0, "SetStagedResource", "a-service", res)
	s.stub.CheckCall(c, 1, "Put", hash, file, res.Size)
	s.stub.CheckCall(c, 2, "SetResource", "a-service", res)
	s.stub.CheckCall(c, 3, "Delete", hash)
	s.stub.CheckCall(c, 4, "UnstageResource", "a-service", res.Name)
}

func newUploadResources(c *gc.C, names ...string) []resource.Resource {
	var resources []resource.Resource
	for _, name := range names {
		res := newUploadResource(c, name, name)
		resources = append(resources, res)
	}
	return resources
}

func newUploadResource(c *gc.C, name, data string) resource.Resource {
	fp, err := charmresource.GenerateFingerprint([]byte(data))
	c.Assert(err, jc.ErrorIsNil)

	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: name,
				Type: charmresource.TypeFile,
				Path: name + ".tgz",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        int64(len(data)),
		},
		Username:  "a-user",
		Timestamp: time.Now(),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	return res
}
