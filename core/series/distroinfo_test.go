// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

const distroInfoContents = `version,codename,series,created,release,eol,eol-server
10.04,Firefox,firefox,2009-10-13,2010-04-26,2016-04-26
12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26
99.04,Star Trek,spock,2019-04-25,2019-10-17,2365-07-17
`

type DistroInfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DistroInfoSuite{})

func (s *DistroInfoSuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	mockFileSystem := NewMockFileSystem(ctrl)
	mockFileSystem.EXPECT().Open(UbuntuDistroInfo).Return(tmpFile, nil)

	info := NewDistroInfo(UbuntuDistroInfo)
	info.fileSystem = mockFileSystem

	err := info.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	now := clock.WallClock.Now().UTC()

	// We don't expect to see wolf
	_, ok := info.SeriesInfo("firefox")
	c.Assert(ok, jc.IsFalse)

	// We expect to see precise
	precise, ok := info.SeriesInfo("precise")
	c.Assert(ok, jc.IsTrue)
	c.Assert(precise.LTS(), jc.IsTrue)
	c.Assert(precise.Supported(now), jc.IsFalse)

	// We expect to see spock
	spock, ok := info.SeriesInfo("spock")
	c.Assert(ok, jc.IsTrue)
	c.Assert(spock.LTS(), jc.IsFalse)
	c.Assert(spock.Supported(now), jc.IsTrue)

	// We don't expect to see wolf
	_, ok = info.SeriesInfo("wolf")
	c.Assert(ok, jc.IsFalse)
}

func (s *DistroInfoSuite) TestDistroInfoSerieSupported(c *gc.C) {
	now := clock.WallClock.Now()

	tests := []struct {
		Name     string
		Released time.Time
		EOL      time.Time
		Now      time.Time
		Expected bool
	}{
		{
			Name:     "within date range",
			Released: now.AddDate(0, 0, -1),
			EOL:      now.AddDate(0, 0, 1),
			Now:      now,
			Expected: true,
		},
		{
			Name:     "before date range",
			Released: now.AddDate(0, 0, -1),
			EOL:      now.AddDate(0, 0, 1),
			Now:      now.AddDate(0, 0, -2),
			Expected: false,
		},
		{
			Name:     "after date range",
			Released: now.AddDate(0, 0, -1),
			EOL:      now.AddDate(0, 0, 1),
			Now:      now.AddDate(0, 0, 2),
			Expected: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d %s", i, test.Name)

		serie := &DistroInfoSerie{
			Released: test.Released,
			EOL:      test.EOL,
		}

		supported := serie.Supported(test.Now.UTC())
		c.Assert(supported, gc.Equals, test.Expected)
	}
}

func (s *DistroInfoSuite) TestDistroInfoSerieLTS(c *gc.C) {
	tests := []struct {
		Name     string
		Version  string
		Expected bool
	}{
		{
			Name:     "is lts",
			Version:  "88.42 LTS",
			Expected: true,
		},
		{
			Name:     "before version",
			Version:  "LTS 88.42",
			Expected: false,
		},
		{
			Name:     "no keyword",
			Version:  "88.42",
			Expected: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d %s", i, test.Name)

		serie := &DistroInfoSerie{
			Version: test.Version,
		}

		lts := serie.LTS()
		c.Assert(lts, gc.Equals, test.Expected)
	}
}

func makeTempFile(c *gc.C, content string) (*os.File, func()) {
	tmpfile, err := ioutil.TempFile("", "distroinfo")
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
