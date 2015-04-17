// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"runtime"
	"testing"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	httptesting "github.com/juju/juju/api/http/testing"
	apiserverbackups "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	stbackups "github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	// TODO(bogdanteleaga): Fix these tests on windows
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: Skipping this on windows for now")
	}
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
	httptesting.APIHTTPClientSuite
}

func (s *httpSuite) SetUpSuite(c *gc.C) {
	s.baseSuite.SetUpSuite(c)
	s.APIHTTPClientSuite.SetUpSuite(c)
}

func (s *httpSuite) TearDownSuite(c *gc.C) {
	s.APIHTTPClientSuite.TearDownSuite(c)
	s.baseSuite.TearDownSuite(c)
}

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.APIHTTPClientSuite.SetUpTest(c)
}

func (s *httpSuite) TearDownTest(c *gc.C) {
	s.APIHTTPClientSuite.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

func (s *httpSuite) setResponse(c *gc.C, status int, data []byte, ctype string) {
	s.APIHTTPClientSuite.SetResponse(c, status, data, ctype)
	backups.SetHTTP(s.client, &s.FakeClient)
}

func (s *httpSuite) setJSONSuccess(c *gc.C, result interface{}) {
	s.APIHTTPClientSuite.SetJSONSuccess(c, result)
	backups.SetHTTP(s.client, &s.FakeClient)
}

func (s *httpSuite) setFailure(c *gc.C, msg string, status int) {
	s.APIHTTPClientSuite.SetFailure(c, msg, status)
	backups.SetHTTP(s.client, &s.FakeClient)
}

func (s *httpSuite) setError(c *gc.C, msg string, status int) {
	s.APIHTTPClientSuite.SetError(c, msg, status)
	backups.SetHTTP(s.client, &s.FakeClient)
}
