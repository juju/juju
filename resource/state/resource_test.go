// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
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
	path := "service-a-service/resources/spam"
	hash := res.Fingerprint.String()
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetResource("a-service", res, file)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "StageResource", "PutAndCheckHash", "SetResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 1, "PutAndCheckHash", path, file, res.Size, hash)
	s.stub.CheckCall(c, 2, "SetResource", res.Name, "a-service", res)
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
	s.stub.CheckCallNames(c, "StageResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
}

func (s *ResourceSuite) TestSetResourcePutFailureBasic(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	path := "service-a-service/resources/spam"
	hash := res.Fingerprint.String()
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, failure, nil, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "StageResource", "PutAndCheckHash", "UnstageResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 1, "PutAndCheckHash", path, file, res.Size, hash)
	s.stub.CheckCall(c, 2, "UnstageResource", res.Name, "a-service")
}

func (s *ResourceSuite) TestSetResourcePutFailureExtra(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	path := "service-a-service/resources/spam"
	hash := res.Fingerprint.String()
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	extraErr := errors.New("<just not your day>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, failure, extraErr, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "StageResource", "PutAndCheckHash", "UnstageResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 1, "PutAndCheckHash", path, file, res.Size, hash)
	s.stub.CheckCall(c, 2, "UnstageResource", res.Name, "a-service")
}

func (s *ResourceSuite) TestSetResourceSetFailureBasic(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	path := "service-a-service/resources/spam"
	hash := res.Fingerprint.String()
	file := &stubReader{stub: s.stub}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, failure, nil, nil, ignoredErr)

	err := st.SetResource("a-service", res, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "StageResource", "PutAndCheckHash", "SetResource", "Remove", "UnstageResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 1, "PutAndCheckHash", path, file, res.Size, hash)
	s.stub.CheckCall(c, 2, "SetResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 3, "Remove", path)
	s.stub.CheckCall(c, 4, "UnstageResource", res.Name, "a-service")
}

func (s *ResourceSuite) TestSetResourceSetFailureExtra(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	path := "service-a-service/resources/spam"
	hash := res.Fingerprint.String()
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
	s.stub.CheckCallNames(c, "StageResource", "PutAndCheckHash", "SetResource", "Remove", "UnstageResource")
	s.stub.CheckCall(c, 0, "StageResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 1, "PutAndCheckHash", path, file, res.Size, hash)
	s.stub.CheckCall(c, 2, "SetResource", res.Name, "a-service", res)
	s.stub.CheckCall(c, 3, "Remove", path)
	s.stub.CheckCall(c, 4, "UnstageResource", res.Name, "a-service")
}

func (s *ResourceSuite) TestOpenResourceOkay(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "some data")
	s.persist.ReturnListResources = []resource.Resource{opened.Resource}
	s.storage.ReturnGet = opened.Content()
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	info, reader, err := st.OpenResource("a-service", "spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources", "Get")
	c.Check(info, jc.DeepEquals, opened.Resource)
	c.Check(reader, gc.Equals, opened.ReadCloser)
}

func (s *ResourceSuite) TestOpenResourceNotFound(c *gc.C) {
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResource("a-service", "spam")

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourcePlaceholder(c *gc.C) {
	res := resourcetesting.NewPlaceholderResource(c, "spam")
	s.persist.ReturnListResources = []resource.Resource{res}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResource("a-service", "spam")

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourceSizeMismatch(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "some data")
	s.persist.ReturnListResources = []resource.Resource{opened.Resource}
	content := opened.Content()
	content.Size += 1
	s.storage.ReturnGet = content
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResource("a-service", "spam")

	s.stub.CheckCallNames(c, "ListResources", "Get")
	c.Check(err, gc.ErrorMatches, `storage returned a size \(10\) which doesn't match resource metadata \(9\)`)
}

func (s *ResourceSuite) TestSetUnitResourceOkay(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetUnitResource("a-service", "some-unit", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetUnitResource")
	s.stub.CheckCall(c, 0, "SetUnitResource", "a-service", "some-unit", res)
}

func (s *ResourceSuite) TestSetUnitResourceBadResource(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	res.Timestamp = time.Time{}
	st := state.NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetUnitResource("a-service", "some-unit", res)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource metadata.*`)
	s.stub.CheckNoCalls(c)
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
	reader := strings.NewReader(data)
	fp, err := charmresource.GenerateFingerprint(reader)
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
		Timestamp: time.Now().UTC(),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	return res
}
