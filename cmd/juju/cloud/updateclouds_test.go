// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type updateCloudsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&updateCloudsSuite{})

func (s *updateCloudsSuite) SetUpTest(c *gc.C) {
	origHome := osenv.SetJujuXDGDataHome(c.MkDir())
	s.AddCleanup(func(*gc.C) { osenv.SetJujuXDGDataHome(origHome) })
}

func (s *updateCloudsSuite) setupTestServer(serverContent string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch serverContent {
		case "404":
			w.WriteHeader(http.StatusNotFound)
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
		}
		fmt.Fprintln(w, serverContent)
	}))
}

func (s *updateCloudsSuite) TestBadArgs(c *gc.C) {
	updateCmd := cloud.NewUpdateCloudsCommandForTest("")
	_, err := testing.RunCommand(c, updateCmd, "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *updateCloudsSuite) run(c *gc.C, url, errMsg string) string {
	updateCmd := cloud.NewUpdateCloudsCommandForTest(url)
	out, err := testing.RunCommand(c, updateCmd)
	if errMsg == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		errString := strings.Replace(err.Error(), "\n", "", -1)
		c.Assert(errString, gc.Matches, errMsg)
	}
	return strings.Replace(testing.Stdout(out), "\n", "", -1)
}

func (s *updateCloudsSuite) Test404(c *gc.C) {
	ts := s.setupTestServer("404")
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	c.Assert(msg, gc.Matches, ".*no new public cloud information available at this time.*")
}

func (s *updateCloudsSuite) Test401(c *gc.C) {
	ts := s.setupTestServer("401")
	defer ts.Close()

	s.run(c, ts.URL, "unauthorised access to URL .*")
}

func (s *updateCloudsSuite) TestBadDataOnServer(c *gc.C) {
	ts := s.setupTestServer("bad data")
	defer ts.Close()

	s.run(c, ts.URL, ".*invalid cloud data received when updating clouds.*")
}

var sampleUpdateCloudData = `
clouds:
  aws:
    type: ec2
    auth-types: [access-key]
    endpoint: http://region
    regions:
      region:
        endpoint: http://region/1.0
`[1:]

func (s *updateCloudsSuite) TestNoNewData(c *gc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)

	ts := s.setupTestServer(sampleUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	c.Assert(msg, gc.Matches, ".*no new public cloud information available at this time.*")
}

func (s *updateCloudsSuite) TestFirstRun(c *gc.C) {
	ts := s.setupTestServer(sampleUpdateCloudData)
	defer ts.Close()

	s.run(c, ts.URL, "")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
}

func (s *updateCloudsSuite) TestNewData(c *gc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)

	newUpdateCloudData := sampleUpdateCloudData + `
      anotherregion:
        endpoint: http://anotherregion/1.0
`[1:]
	ts := s.setupTestServer(newUpdateCloudData)
	defer ts.Close()

	s.run(c, ts.URL, "")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	clouds, err = jujucloud.ParseCloudMetadata([]byte(newUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
}
