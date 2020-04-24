// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource/api/client"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	BaseSuite
}

func (s *UploadSuite) TestOkay(c *gc.C) {
	data := "<data>"
	reader := &stubFile{stub: s.stub}
	reader.returnRead = strings.NewReader(data)
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	_, s.response.Resource = newResource(c, "spam", "a-user", data)

	err := cl.Upload("a-application", "spam", "foo.zip", reader)
	c.Assert(err, jc.ErrorIsNil)

	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", reader)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.Header.Set("Content-Disposition", "form-data; filename=foo.zip")
	req.ContentLength = int64(len(data))

	s.stub.CheckCallNames(c, "Read", "Read", "Seek", "Do")
	s.stub.CheckCall(c, 3, "Do", req, s.response)
}

func (s *UploadSuite) TestBadService(c *gc.C) {
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	err := cl.Upload("???", "spam", "file.zip", nil)

	c.Check(err, gc.ErrorMatches, `.*invalid application.*`)
	s.stub.CheckNoCalls(c)
}

func (s *UploadSuite) TestBadRequest(c *gc.C) {
	reader := &stubFile{stub: s.stub}
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	err := cl.Upload("a-application", "spam", "file.zip", reader)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "Read")
}

func (s *UploadSuite) TestRequestFailed(c *gc.C) {
	reader := &stubFile{stub: s.stub}
	reader.returnRead = strings.NewReader("<data>")
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, nil, failure)

	err := cl.Upload("a-application", "spam", "file.zip", reader)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "Read", "Read", "Seek", "Do")
}

func (s *UploadSuite) TestPendingResources(c *gc.C) {
	res, apiResult := newResourceResult(c, "a-application", "spam")
	resources := []charmresource.Resource{res[0].Resource}
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{uuid.String()}
	s.response.Resource = apiResult.Resources[0]
	s.facade.pendingIDs = expected
	cURL := charm.MustParseURL("cs:~a-user/trusty/spam-5")
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	pendingIDs, err := cl.AddPendingResources(client.AddPendingResourcesArgs{
		ApplicationID: "a-application",
		CharmID: charmstore.CharmID{
			URL: cURL,
		},
		Resources: resources,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "FacadeCall")
	//s.stub.CheckCall(c, 0, "FacadeCall", "AddPendingResources", args, result)
	c.Check(pendingIDs, jc.DeepEquals, expected)
}

func (s *UploadSuite) TestPendingResourceOkay(c *gc.C) {
	res, apiResult := newResourceResult(c, "a-application", "spam")
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	s.response.Resource = apiResult.Resources[0]
	data := "<data>"
	reader := &stubFile{stub: s.stub}
	reader.returnRead = strings.NewReader(data)
	s.facade.pendingIDs = []string{expected}
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	uploadID, err := cl.UploadPendingResource("a-application", res[0].Resource, "file.zip", reader)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"FacadeCall",
		"Read",
		"Read",
		"Seek",
		"Do",
	)

	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", reader)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))
	req.URL.RawQuery = "pendingid=" + expected
	req.Header.Set("Content-Disposition", "form-data; filename=file.zip")

	s.stub.CheckCall(c, 4, "Do", req, s.response)
	c.Check(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestPendingResourceNoFile(c *gc.C) {
	res, apiResult := newResourceResult(c, "a-application", "spam")
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	s.response.Resource = apiResult.Resources[0]
	s.facade.pendingIDs = []string{expected}
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	uploadID, err := cl.UploadPendingResource("a-application", res[0].Resource, "file.zip", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"FacadeCall",
	)
	c.Check(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestPendingResourceBadService(c *gc.C) {
	res, _ := newResourceResult(c, "a-application", "spam")
	s.facade.FacadeCallFn = nil
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)

	_, err := cl.UploadPendingResource("???", res[0].Resource, "file.zip", nil)

	c.Check(err, gc.ErrorMatches, `.*invalid application.*`)
	s.stub.CheckNoCalls(c)
}

func (s *UploadSuite) TestPendingResourceBadRequest(c *gc.C) {
	res, _ := newResource(c, "spam", "", "")
	chRes := res.Resource
	reader := &stubFile{stub: s.stub}
	s.facade.pendingIDs = []string{"some-unique-id"}
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, failure)

	_, err := cl.UploadPendingResource("a-application", chRes, "file.zip", reader)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "FacadeCall", "Read")
}

func (s *UploadSuite) TestPendingResourceRequestFailed(c *gc.C) {
	res, _ := newResourceResult(c, "a-application", "spam")
	reader := &stubFile{stub: s.stub}
	reader.returnRead = strings.NewReader("<data>")
	s.facade.pendingIDs = []string{"some-unique-id"}
	cl := client.NewClient(context.Background(), s.facade, s, s.facade)
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, nil, nil, failure)

	_, err := cl.UploadPendingResource("a-application", res[0].Resource, "file.zip", reader)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c,
		"FacadeCall",
		"Read",
		"Read",
		"Seek",
		"Do",
	)
}

type stubFile struct {
	stub *testing.Stub

	returnRead io.Reader
	returnSeek int64
}

func (s *stubFile) Read(buf []byte) (int, error) {
	s.stub.AddCall("Read", buf)
	if err := s.stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return s.returnRead.Read(buf)
}

func (s *stubFile) Seek(offset int64, whence int) (int64, error) {
	s.stub.AddCall("Seek", offset, whence)
	if err := s.stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return s.returnSeek, nil
}

func (s *stubFile) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
