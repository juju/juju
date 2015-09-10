// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package series_test

import (
	"fmt"

	"github.com/juju/utils"

	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/juju/juju/series"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type windowsSeriesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&windowsSeriesSuite{})

var versionTests = []struct {
	version string
	want    string
}{
	{
		"Hyper-V Server 2012 R2",
		"win2012hvr2",
	},
	{
		"Hyper-V Server 2012",
		"win2012hv",
	},
	{
		"Windows Server 2012 R2",
		"win2012r2",
	},
	{
		"Windows Server 2012",
		"win2012",
	},
	{
		"Windows Server 2012 R2 Datacenter",
		"win2012r2",
	},
	{
		"Windows Server 2012 Standard",
		"win2012",
	},
	{
		"Windows Storage Server 2012 R2",
		"win2012r2",
	},
	{
		"Windows Storage Server 2012 Standard",
		"win2012",
	},
	{
		"Windows Storage Server 2012 R2 Standard",
		"win2012r2",
	},
	{
		"Windows 7 Home",
		"win7",
	},
	{
		"Windows 8 Pro",
		"win8",
	},
	{
		"Windows 8.1 Pro",
		"win81",
	},
}

func (s *windowsSeriesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	salt, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	regKey := fmt.Sprintf(`SOFTWARE\JUJU\%s`, salt)
	s.PatchValue(series.CurrentVersionKey, regKey)

	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, *series.CurrentVersionKey, registry.ALL_ACCESS)
	c.Assert(err, jc.ErrorIsNil)

	err = k.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.AddCleanup(func(*gc.C) {
		registry.DeleteKey(registry.LOCAL_MACHINE, *series.CurrentVersionKey)
	})
}

func (s *windowsSeriesSuite) TestReadSeries(c *gc.C) {
	for _, value := range versionTests {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, *series.CurrentVersionKey, registry.ALL_ACCESS)
		c.Assert(err, jc.ErrorIsNil)

		err = k.SetStringValue("ProductName", value.version)
		c.Assert(err, jc.ErrorIsNil)

		err = k.Close()
		c.Assert(err, jc.ErrorIsNil)

		ver, err := series.ReadSeries()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ver, gc.Equals, value.want)
	}
}
