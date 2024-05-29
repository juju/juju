// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"os"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

const distroInfoContents = `version,codename,series,created,release,eol,eol-server
10.04,Firefox,firefox,2009-10-13,2010-04-26,2016-04-26
12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26
99.04,Focal,focal,2020-04-25,2020-10-17,2365-07-17
`

type SupportedSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSeriesSuite{})

func (s *SupportedSeriesSuite) TestSupportedInfoForType(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := supportedInfoForType(tmpFile.Name(), now, Base{}, "")
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()
	c.Assert(ctrlBases, jc.DeepEquals, transform.Slice([]string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04"}, MustParseBaseFromString))

	workloadBases := info.workloadBases(false)
	c.Assert(workloadBases, jc.DeepEquals, transform.Slice([]string{
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}

func (s *SupportedSeriesSuite) TestSupportedInfoForTypeUsingImageStream(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := supportedInfoForType(tmpFile.Name(), now, MustParseBaseFromString("ubuntu@20.04"), "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()
	c.Assert(ctrlBases, jc.DeepEquals, transform.Slice([]string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04"}, MustParseBaseFromString))

	workloadBases := info.workloadBases(false)
	c.Assert(workloadBases, jc.DeepEquals, transform.Slice([]string{
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}

func (s *SupportedSeriesSuite) TestSupportedInfoForTypeUsingInvalidImageStream(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := supportedInfoForType(tmpFile.Name(), now, MustParseBaseFromString("ubuntu@20.04"), "turtle")
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()
	c.Assert(ctrlBases, jc.DeepEquals, transform.Slice([]string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04"}, MustParseBaseFromString))

	workloadBases := info.workloadBases(false)
	c.Assert(workloadBases, jc.DeepEquals, transform.Slice([]string{
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}

func (s *SupportedSeriesSuite) TestSupportedInfoForTypeUsingInvalidSeries(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := supportedInfoForType(tmpFile.Name(), now, MustParseBaseFromString("ubuntu@10.04"), "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()
	c.Assert(ctrlBases, jc.DeepEquals, transform.Slice([]string{"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04"}, MustParseBaseFromString))

	workloadBases := info.workloadBases(false)
	c.Assert(workloadBases, jc.DeepEquals, transform.Slice([]string{
		"ubuntu@20.04", "ubuntu@22.04", "ubuntu@24.04",
	}, MustParseBaseFromString))
}

func makeTempFile(c *gc.C, content string) (*os.File, func()) {
	tmpfile, err := os.CreateTemp("", "distroinfo")
	if err != nil {
		c.Assert(err, jc.ErrorIsNil)
	}

	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, jc.ErrorIsNil)

	// Reset the file for reading.
	_, err = tmpfile.Seek(0, 0)
	c.Assert(err, jc.ErrorIsNil)

	return tmpfile, func() {
		err := os.Remove(tmpfile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}
}
