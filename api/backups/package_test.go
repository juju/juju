// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	httptesting "github.com/juju/juju/api/http/testing"
	apiserverbackups "github.com/juju/juju/apiserver/backups"
	apiserverhttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	stbackups "github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	jujutesting.JujuConnSuite
	backupstesting.BaseSuite
	client *backups.Client
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
	s.client = backups.NewClient(s.APIState)
}

func (s *baseSuite) metadataResult() *params.BackupsMetadataResult {
	result := apiserverbackups.ResultFromMetadata(s.Meta)
	return &result
}

func (s *baseSuite) checkMetadataResult(c *gc.C, result *params.BackupsMetadataResult, meta *stbackups.Metadata) {
	var finished, stored time.Time
	if meta.Finished != nil {
		finished = *meta.Finished
	}
	if meta.Stored() != nil {
		stored = *(meta.Stored())
	}

	c.Check(result.ID, gc.Equals, meta.ID())
	c.Check(result.Started, gc.Equals, meta.Started)
	c.Check(result.Finished, gc.Equals, finished)
	c.Check(result.Checksum, gc.Equals, meta.Checksum())
	c.Check(result.ChecksumFormat, gc.Equals, meta.ChecksumFormat())
	c.Check(result.Size, gc.Equals, meta.Size())
	c.Check(result.Stored, gc.Equals, stored)
	c.Check(result.Notes, gc.Equals, meta.Notes)

	c.Check(result.Environment, gc.Equals, meta.Origin.Environment)
	c.Check(result.Machine, gc.Equals, meta.Origin.Machine)
	c.Check(result.Hostname, gc.Equals, meta.Origin.Hostname)
	c.Check(result.Version, gc.Equals, meta.Origin.Version)
}

type httpSuite struct {
	baseSuite
	httpClient httptesting.FakeClient
}

func (s *httpSuite) setResponse(c *gc.C, status int, data []byte, ctype string) {
	resp := http.Response{
		StatusCode: status,
		Header:     make(http.Header),
	}

	resp.Header.Set("Content-Type", ctype)
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	s.httpClient.Response = &resp
	backups.SetHTTP(s.client, &s.httpClient)
}

func (s *httpSuite) setJSONSuccess(c *gc.C, result interface{}) {
	status := http.StatusOK
	data, err := json.Marshal(result)
	c.Assert(err, jc.ErrorIsNil)

	s.setResponse(c, status, data, apiserverhttp.CTYPE_JSON)
}

func (s *httpSuite) setFailure(c *gc.C, msg string, status int) {
	if status < 0 {
		status = http.StatusInternalServerError
	}

	failure := params.Error{
		Message: msg,
	}
	data, err := json.Marshal(&failure)
	c.Assert(err, jc.ErrorIsNil)

	s.setResponse(c, status, data, apiserverhttp.CTYPE_JSON)
}

func (s *httpSuite) setError(c *gc.C, msg string, status int) {
	if status < 0 {
		status = http.StatusInternalServerError
	}

	data := []byte(msg)
	s.setResponse(c, status, data, "application/octet-stream")
}
