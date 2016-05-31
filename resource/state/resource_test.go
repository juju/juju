// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&ResourceSuite{})

type ResourceSuite struct {
	testing.IsolationSuite

	stub      *testing.Stub
	raw       *stubRawState
	persist   *stubPersistence
	storage   *stubStorage
	timestamp time.Time
	pendingID string
}

func (s *ResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.raw = &stubRawState{stub: s.stub}
	s.persist = &stubPersistence{stub: s.stub}
	s.persist.ReturnStageResource = &stubStagedResource{stub: s.stub}
	s.storage = &stubStorage{stub: s.stub}
	s.raw.ReturnPersistence = s.persist
	s.raw.ReturnStorage = s.storage
	s.timestamp = time.Now().UTC()
	s.pendingID = ""
}

func (s *ResourceSuite) now() time.Time {
	s.stub.AddCall("currentTimestamp")
	s.stub.NextErr() // Pop one off.

	return s.timestamp
}

func (s *ResourceSuite) newPendingID() (string, error) {
	s.stub.AddCall("newPendingID")
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.pendingID, nil
}

func (s *ResourceSuite) TestListResourcesOkay(c *gc.C) {
	expected := newUploadResources(c, "spam", "eggs")
	s.persist.ReturnListResources = resource.ServiceResources{Resources: expected}
	tag := names.NewUnitTag("a-application/0")
	s.raw.ReturnUnits = []names.UnitTag{tag}
	st := NewState(s.raw)
	s.stub.ResetCalls()

	resources, err := st.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, jc.DeepEquals, expected)
	c.Check(resources.UnitResources, jc.DeepEquals, []resource.UnitResources{{
		Tag: tag,
	}})
	s.stub.CheckCallNames(c, "ListResources", "Units")
	s.stub.CheckCall(c, 0, "ListResources", "a-application")
}

func (s *ResourceSuite) TestListResourcesNoUnits(c *gc.C) {
	expected := newUploadResources(c, "spam", "eggs")
	s.persist.ReturnListResources = resource.ServiceResources{Resources: expected}
	st := NewState(s.raw)
	s.stub.ResetCalls()

	resources, err := st.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, jc.DeepEquals, expected)
	c.Check(resources.UnitResources, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "ListResources", "Units")
	s.stub.CheckCall(c, 0, "ListResources", "a-application")
}

func (s *ResourceSuite) TestListResourcesEmpty(c *gc.C) {
	s.raw.ReturnUnits = []names.UnitTag{
		names.NewUnitTag("a-application/0"),
	}
	st := NewState(s.raw)
	s.stub.ResetCalls()

	resources, err := st.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, gc.HasLen, 0)
	c.Check(resources.UnitResources, gc.HasLen, 1)
	s.stub.CheckCallNames(c, "ListResources", "Units")
}

func (s *ResourceSuite) TestListResourcesError(c *gc.C) {
	expected := newUploadResources(c, "spam", "eggs")
	s.persist.ReturnListResources = resource.ServiceResources{Resources: expected}
	st := NewState(s.raw)
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	_, err := st.ListResources("a-application")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "ListResources", "VerifyService")
}

func (s *ResourceSuite) TestGetPendingResource(c *gc.C) {
	resources := newUploadResources(c, "spam", "eggs")
	resources[0].PendingID = "some-unique-id"
	resources[1].PendingID = "other-unique-id"
	s.persist.ReturnListPendingResources = resources
	st := NewState(s.raw)
	s.stub.ResetCalls()

	res, err := st.GetPendingResource("a-application", "eggs", "other-unique-id")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListPendingResources")
	s.stub.CheckCall(c, 0, "ListPendingResources", "a-application")
	c.Check(res, jc.DeepEquals, resources[1])
}

func (s *ResourceSuite) TestSetResourceOkay(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	chRes := expected.Resource
	hash := chRes.Fingerprint.String()
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()

	res, err := st.SetResource("a-application", "a-user", chRes, file)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Activate",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, res.Size, hash)
	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource:      chRes,
		ID:            "a-application/" + res.Name,
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     s.timestamp,
	})
}

func (s *ResourceSuite) TestSetResourceInfoOnly(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = time.Time{}
	expected.Username = ""
	chRes := expected.Resource
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()

	res, err := st.SetResource("a-application", "a-user", chRes, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"SetResource",
	)
	s.stub.CheckCall(c, 0, "SetResource", expected)
	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource:      chRes,
		ID:            "a-application/" + res.Name,
		ApplicationID: "a-application",
	})
}

func (s *ResourceSuite) TestSetResourceBadResource(c *gc.C) {
	res := newUploadResource(c, "spam", "spamspamspam")
	res.Fingerprint = charmresource.Fingerprint{}
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()

	_, err := st.SetResource("a-application", "a-user", res.Resource, file)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource metadata.*`)
	s.stub.CheckCallNames(c, "currentTimestamp")
}

func (s *ResourceSuite) TestSetResourceStagingFailure(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, failure, ignoredErr)

	_, err := st.SetResource("a-application", "a-user", expected.Resource, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "currentTimestamp", "StageResource")
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
}

func (s *ResourceSuite) TestSetResourcePutFailureBasic(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	hash := expected.Fingerprint.String()
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, failure, nil, ignoredErr)

	_, err := st.SetResource("a-application", "a-user", expected.Resource, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Unstage",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, expected.Size, hash)
}

func (s *ResourceSuite) TestSetResourcePutFailureExtra(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	hash := expected.Fingerprint.String()
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	extraErr := errors.New("<just not your day>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, failure, extraErr, ignoredErr)

	_, err := st.SetResource("a-application", "a-user", expected.Resource, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Unstage",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, expected.Size, hash)
}

func (s *ResourceSuite) TestSetResourceSetFailureBasic(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	hash := expected.Fingerprint.String()
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, failure, nil, nil, ignoredErr)

	_, err := st.SetResource("a-application", "a-user", expected.Resource, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Activate",
		"Remove",
		"Unstage",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, expected.Size, hash)
	s.stub.CheckCall(c, 4, "Remove", path)
}

func (s *ResourceSuite) TestSetResourceSetFailureExtra(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.Timestamp = s.timestamp
	hash := expected.Fingerprint.String()
	path := "application-a-application/resources/spam"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()
	failure := errors.New("<failure>")
	extraErr1 := errors.New("<just not your day>")
	extraErr2 := errors.New("<wow...just wow>")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, failure, extraErr1, extraErr2, ignoredErr)

	_, err := st.SetResource("a-application", "a-user", expected.Resource, file)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Activate",
		"Remove",
		"Unstage",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, expected.Size, hash)
	s.stub.CheckCall(c, 4, "Remove", path)
}

func (s *ResourceSuite) TestUpdatePendingResourceOkay(c *gc.C) {
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.PendingID = "some-unique-id"
	expected.Timestamp = s.timestamp
	chRes := expected.Resource
	hash := chRes.Fingerprint.String()
	path := "application-a-application/resources/spam-some-unique-id"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	s.stub.ResetCalls()

	res, err := st.UpdatePendingResource("a-application", "some-unique-id", "a-user", chRes, file)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Activate",
	)
	s.stub.CheckCall(c, 1, "StageResource", expected, path)
	s.stub.CheckCall(c, 2, "PutAndCheckHash", path, file, res.Size, hash)
	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource:      chRes,
		ID:            "a-application/" + res.Name,
		ApplicationID: "a-application",
		PendingID:     "some-unique-id",
		Username:      "a-user",
		Timestamp:     s.timestamp,
	})
}

func (s *ResourceSuite) TestAddPendingResourceOkay(c *gc.C) {
	s.pendingID = "some-unique-ID-001"
	expected := newUploadResource(c, "spam", "spamspamspam")
	expected.PendingID = s.pendingID
	expected.Timestamp = s.timestamp
	chRes := expected.Resource
	hash := chRes.Fingerprint.String()
	path := "application-a-application/resources/spam-some-unique-ID-001"
	file := &stubReader{stub: s.stub}
	st := NewState(s.raw)
	st.currentTimestamp = s.now
	st.newPendingID = s.newPendingID
	s.stub.ResetCalls()

	pendingID, err := st.AddPendingResource("a-application", "a-user", chRes, file)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"newPendingID",
		"currentTimestamp",
		"StageResource",
		"PutAndCheckHash",
		"Activate",
	)
	s.stub.CheckCall(c, 2, "StageResource", expected, path)
	s.stub.CheckCall(c, 3, "PutAndCheckHash", path, file, expected.Size, hash)
	c.Check(pendingID, gc.Equals, s.pendingID)
}

func (s *ResourceSuite) TestOpenResourceOkay(c *gc.C) {
	data := "some data"
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", data)
	s.persist.ReturnGetResource = opened.Resource
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	s.storage.ReturnGet = opened.Content()
	st := NewState(s.raw)
	s.stub.ResetCalls()

	info, reader, err := st.OpenResource("a-application", "spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetResource", "Get")
	s.stub.CheckCall(c, 1, "Get", "application-a-application/resources/spam")
	c.Check(info, jc.DeepEquals, opened.Resource)
	c.Check(reader, gc.Equals, opened.ReadCloser)
}

func (s *ResourceSuite) TestOpenResourceNotFound(c *gc.C) {
	st := NewState(s.raw)
	s.stub.ResetCalls()
	errNotFound := errors.NotFoundf("resource")
	s.stub.SetErrors(errNotFound)

	_, _, err := st.OpenResource("a-application", "spam")

	s.stub.CheckCallNames(c, "GetResource", "VerifyService")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourcePlaceholder(c *gc.C) {
	res := resourcetesting.NewPlaceholderResource(c, "spam", "a-application")
	s.persist.ReturnGetResource = res
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	st := NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResource("a-application", "spam")

	s.stub.CheckCallNames(c, "GetResource")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourceSizeMismatch(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	s.persist.ReturnGetResource = opened.Resource
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	content := opened.Content()
	content.Size += 1
	s.storage.ReturnGet = content
	st := NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResource("a-application", "spam")

	s.stub.CheckCallNames(c, "GetResource", "Get")
	c.Check(err, gc.ErrorMatches, `storage returned a size \(10\) which doesn't match resource metadata \(9\)`)
}

func (s *ResourceSuite) TestOpenResourceForUniterOkay(c *gc.C) {
	data := "some data"
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", data)
	s.persist.ReturnGetResource = opened.Resource
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	s.storage.ReturnGet = opened.Content()
	unit := newUnit(s.stub, "a-application/0")
	st := NewState(s.raw)
	s.stub.ResetCalls()

	info, reader, err := st.OpenResourceForUniter(unit, "spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ApplicationName", "GetResource", "Get", "Name", "SetUnitResourceProgress")
	s.stub.CheckCall(c, 2, "Get", "application-a-application/resources/spam")
	c.Check(info, jc.DeepEquals, opened.Resource)

	b, err := ioutil.ReadAll(reader)
	// note ioutil.ReadAll converts EOF to nil
	c.Check(err, jc.ErrorIsNil)
	c.Check(b, gc.DeepEquals, []byte(data))
}

func (s *ResourceSuite) TestOpenResourceForUniterNotFound(c *gc.C) {
	unit := newUnit(s.stub, "a-application/0")
	st := NewState(s.raw)
	s.stub.ResetCalls()
	errNotFound := errors.NotFoundf("resource")
	s.stub.SetErrors(nil, errNotFound)

	_, _, err := st.OpenResourceForUniter(unit, "spam")

	s.stub.CheckCallNames(c, "ApplicationName", "GetResource", "VerifyService")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourceForUniterPlaceholder(c *gc.C) {
	res := resourcetesting.NewPlaceholderResource(c, "spam", "a-application")
	s.persist.ReturnGetResource = res
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	unit := newUnit(s.stub, "a-application/0")
	st := NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResourceForUniter(unit, "spam")

	s.stub.CheckCallNames(c, "ApplicationName", "GetResource")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestOpenResourceForUniterSizeMismatch(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	s.persist.ReturnGetResource = opened.Resource
	s.persist.ReturnGetResourcePath = "application-a-application/resources/spam"
	content := opened.Content()
	content.Size += 1
	s.storage.ReturnGet = content
	unit := newUnit(s.stub, "a-application/0")
	st := NewState(s.raw)
	s.stub.ResetCalls()

	_, _, err := st.OpenResourceForUniter(unit, "spam")

	s.stub.CheckCallNames(c, "ApplicationName", "GetResource", "Get")
	c.Check(err, gc.ErrorMatches, `storage returned a size \(10\) which doesn't match resource metadata \(9\)`)
}

func (s *ResourceSuite) TestSetCharmStoreResources(c *gc.C) {
	lastPolled := time.Now().UTC()
	resources := newStoreResources(c, "spam", "eggs")
	var info []charmresource.Resource
	for _, res := range resources {
		chRes := res.Resource
		info = append(info, chRes)
	}
	st := NewState(s.raw)
	s.stub.ResetCalls()

	err := st.SetCharmStoreResources("a-application", info, lastPolled)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"SetCharmStoreResource",
		"SetCharmStoreResource",
	)
	s.stub.CheckCall(c, 0, "SetCharmStoreResource", "a-application/spam", "a-application", info[0], lastPolled)
	s.stub.CheckCall(c, 1, "SetCharmStoreResource", "a-application/eggs", "a-application", info[1], lastPolled)
}

func (s *ResourceSuite) TestNewResourcePendingResourcesOps(c *gc.C) {
	doc1 := map[string]string{"a": "1"}
	doc2 := map[string]string{"b": "2"}
	expected := []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#pending-some-unique-ID-001",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &doc1,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam#pending-some-unique-ID-001",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &doc2,
	}}
	s.persist.ReturnNewResolvePendingResourceOps = [][]txn.Op{
		expected[:2],
		expected[2:],
	}
	applicationID := "a-application"
	st := NewState(s.raw)
	s.stub.ResetCalls()
	pendingIDs := map[string]string{
		"spam": "some-unique-id",
		"eggs": "other-unique-id",
	}

	ops, err := st.NewResolvePendingResourcesOps(applicationID, pendingIDs)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewResolvePendingResourceOps",
		"NewResolvePendingResourceOps",
	)
	c.Check(s.persist.CallsForNewResolvePendingResourceOps, jc.DeepEquals, map[string]string{
		"a-application/spam": "some-unique-id",
		"a-application/eggs": "other-unique-id",
	})
	c.Check(ops, jc.DeepEquals, expected)
}

func (s *ResourceSuite) TestUnitSetterEOF(c *gc.C) {
	r := unitSetter{
		ReadCloser: ioutil.NopCloser(&bytes.Buffer{}),
		persist:    &stubPersistence{stub: s.stub},
		unit:       newUnit(s.stub, "some-application/0"),
		resource:   newUploadResource(c, "res", "res"),
	}
	// have to try to read non-zero data, or bytes.buffer will happily return
	// nil.
	p := make([]byte, 5)
	n, err := r.Read(p)
	c.Assert(n, gc.Equals, 0)
	c.Assert(err, gc.Equals, io.EOF)

	s.stub.CheckCallNames(c, "Name", "SetUnitResource")
	s.stub.CheckCall(c, 1, "SetUnitResource", "some-application/0", r.resource)
}

func (s *ResourceSuite) TestUnitSetterNoEOF(c *gc.C) {
	r := unitSetter{
		ReadCloser: ioutil.NopCloser(bytes.NewBufferString("foobar")),
		persist:    &stubPersistence{stub: s.stub},
		unit:       newUnit(s.stub, "some-application/0"),
		resource:   newUploadResource(c, "res", "res"),
	}
	// read less than the full buffer
	p := make([]byte, 3)
	n, err := r.Read(p)
	c.Assert(n, gc.Equals, 3)
	c.Assert(err, gc.Equals, nil)

	// Assert that we don't call SetUnitResource if we read but don't reach the
	// end of the buffer.
	s.stub.CheckCallNames(c, "Name", "SetUnitResourceProgress")
}

func (s *ResourceSuite) TestUnitSetterSetUnitErr(c *gc.C) {
	r := unitSetter{
		ReadCloser: ioutil.NopCloser(&bytes.Buffer{}),
		persist:    &stubPersistence{stub: s.stub},
		unit:       newUnit(s.stub, "some-application/0"),
		resource:   newUploadResource(c, "res", "res"),
	}

	s.stub.SetErrors(errors.Errorf("oops!"))
	// have to try to read non-zero data, or bytes.buffer will happily return
	// nil.
	p := make([]byte, 5)
	n, err := r.Read(p)
	c.Assert(n, gc.Equals, 0)

	// ensure that we return the EOF from bytes.buffer and not the error from SetUnitResource.
	c.Assert(err, gc.Equals, io.EOF)

	s.stub.CheckCallNames(c, "Name", "SetUnitResource")
	s.stub.CheckCall(c, 1, "SetUnitResource", "some-application/0", r.resource)
}

func (s *ResourceSuite) TestUnitSetterErr(c *gc.C) {
	r := unitSetter{
		ReadCloser: ioutil.NopCloser(&stubReader{stub: s.stub}),
		persist:    &stubPersistence{stub: s.stub},
		unit:       newUnit(s.stub, "some-application/0"),
		resource:   newUploadResource(c, "res", "res"),
	}
	expected := errors.Errorf("some-err")
	s.stub.SetErrors(expected)
	// have to try to read non-zero data, or bytes.buffer will happily return
	// nil.
	p := make([]byte, 5)
	n, err := r.Read(p)
	c.Assert(n, gc.Equals, 0)
	c.Assert(err, gc.Equals, expected)

	s.stub.CheckCall(c, 0, "Read", p)
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
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	return opened.Resource
}

func newStoreResources(c *gc.C, names ...string) []resource.Resource {
	var resources []resource.Resource
	for _, name := range names {
		res := newStoreResource(c, name, name)
		resources = append(resources, res)
	}
	return resources
}

func newStoreResource(c *gc.C, name, data string) resource.Resource {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Origin = charmresource.OriginStore
	res.Revision = 1
	res.Username = ""
	res.Timestamp = time.Time{}
	return res
}

func newCharmResource(c *gc.C, name, data string, rev int) charmresource.Resource {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	chRes := opened.Resource.Resource
	chRes.Origin = charmresource.OriginStore
	chRes.Revision = rev
	return chRes
}

func newUnit(stub *testing.Stub, name string) *resourcetesting.StubUnit {
	return &resourcetesting.StubUnit{
		Stub:                  stub,
		ReturnName:            name,
		ReturnApplicationName: strings.Split(name, "/")[0],
	}
}
