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

func boolPtr(b bool) *bool {
	return &b
}

func (s *SupportedSeriesSuite) TestUbuntuVersions(c *gc.C) {
	ubuntuSeries := map[SeriesName]seriesVersion{
		Precise: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "12.04",
		},
		Quantal: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "12.10",
		},
		Raring: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "13.04",
		},
		Saucy: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "13.10",
		},
		Trusty: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "14.04",
			LTS:          true,
			ESMSupported: true,
		},
		Utopic: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "14.10",
		},
		Vivid: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "15.04",
		},
		Wily: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "15.10",
		},
		Xenial: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "16.04",
			LTS:          true,
			ESMSupported: true,
		},
		Yakkety: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "16.10",
		},
		Zesty: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "17.04",
		},
		Artful: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "17.10",
		},
		Bionic: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "18.04",
			LTS:          true,
			ESMSupported: true,
		},
		Cosmic: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "18.10",
		},
		Disco: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "19.04",
		},
		Eoan: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "19.10",
		},
		Focal: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "20.04",
			LTS:          true,
			Supported:    true,
			ESMSupported: true,
		},
		Groovy: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "20.10",
		},
		Hirsute: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "21.04",
		},
		Impish: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "21.10",
		},
		Jammy: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "22.04",
			LTS:          true,
			Supported:    true,
			ESMSupported: true,
		},
		Kinetic: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "22.10",
		},
		Lunar: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "23.04",
		},
		Mantic: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "23.10",
		},
		Noble: {
			WorkloadType: ControllerWorkloadType,
			OS:           UbuntuOS,
			Version:      "24.04",
			LTS:          true,
			ESMSupported: true,
		},
	}

	result := ubuntuVersions(nil, nil, ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"artful": "17.10", "bionic": "18.04", "cosmic": "18.10", "disco": "19.04", "eoan": "19.10", "focal": "20.04", "groovy": "20.10", "hirsute": "21.04", "impish": "21.10", "jammy": "22.04", "kinetic": "22.10", "lunar": "23.04", "mantic": "23.10", "noble": "24.04", "precise": "12.04", "quantal": "12.10", "raring": "13.04", "saucy": "13.10", "trusty": "14.04", "utopic": "14.10", "vivid": "15.04", "wily": "15.10", "xenial": "16.04", "yakkety": "16.10", "zesty": "17.04"})

	result = ubuntuVersions(boolPtr(true), boolPtr(true), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"focal": "20.04", "jammy": "22.04"})

	result = ubuntuVersions(boolPtr(false), boolPtr(false), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"artful": "17.10", "cosmic": "18.10", "disco": "19.04", "eoan": "19.10", "groovy": "20.10", "hirsute": "21.04", "impish": "21.10", "kinetic": "22.10", "lunar": "23.04", "mantic": "23.10", "precise": "12.04", "quantal": "12.10", "raring": "13.04", "saucy": "13.10", "utopic": "14.10", "vivid": "15.04", "wily": "15.10", "yakkety": "16.10", "zesty": "17.04"})

	result = ubuntuVersions(boolPtr(true), boolPtr(false), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{})

	result = ubuntuVersions(boolPtr(false), boolPtr(true), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"bionic": "18.04", "noble": "24.04", "trusty": "14.04", "xenial": "16.04"})

	result = ubuntuVersions(boolPtr(true), nil, ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"focal": "20.04", "jammy": "22.04"})

	result = ubuntuVersions(boolPtr(false), nil, ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"artful": "17.10", "bionic": "18.04", "cosmic": "18.10", "disco": "19.04", "eoan": "19.10", "groovy": "20.10", "hirsute": "21.04", "impish": "21.10", "kinetic": "22.10", "lunar": "23.04", "mantic": "23.10", "noble": "24.04", "precise": "12.04", "quantal": "12.10", "raring": "13.04", "saucy": "13.10", "trusty": "14.04", "utopic": "14.10", "vivid": "15.04", "wily": "15.10", "xenial": "16.04", "yakkety": "16.10", "zesty": "17.04"})

	result = ubuntuVersions(nil, boolPtr(true), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"bionic": "18.04", "focal": "20.04", "jammy": "22.04", "noble": "24.04", "trusty": "14.04", "xenial": "16.04"})

	result = ubuntuVersions(nil, boolPtr(false), ubuntuSeries)
	c.Check(result, gc.DeepEquals, map[string]string{"artful": "17.10", "cosmic": "18.10", "disco": "19.04", "eoan": "19.10", "groovy": "20.10", "hirsute": "21.04", "impish": "21.10", "kinetic": "22.10", "lunar": "23.04", "mantic": "23.10", "precise": "12.04", "quantal": "12.10", "raring": "13.04", "saucy": "13.10", "utopic": "14.10", "vivid": "15.04", "wily": "15.10", "yakkety": "16.10", "zesty": "17.04"})
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
