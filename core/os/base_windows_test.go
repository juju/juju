// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	stdtesting "testing"

	"github.com/juju/tc"
	"golang.org/x/sys/windows/registry"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

const randomPasswordBytes = 18

type windowsBaseSuite struct {
	testhelpers.CleanupSuite
}

var versionTests = []struct {
	version string
	want    corebase.Base
}{
	{
		"Hyper-V Server 2012 R2",
		corebase.MustParseBaseFromString("win@2012hvr2"),
	},
	{
		"Hyper-V Server 2012",
		corebase.MustParseBaseFromString("win@2012hv"),
	},
	{
		"Windows Server 2008 R2",
		corebase.MustParseBaseFromString("win@2008r2"),
	},
	{
		"Windows Server 2012 R2",
		corebase.MustParseBaseFromString("win@2012r2"),
	},
	{
		"Windows Server 2012",
		corebase.MustParseBaseFromString("win@2012"),
	},
	{
		"Windows Server 2012 R2 Datacenter",
		corebase.MustParseBaseFromString("win@2012r2"),
	},
	{
		"Windows Server 2012 Standard",
		corebase.MustParseBaseFromString("win@2012"),
	},
	{
		"Windows Storage Server 2012 R2",
		corebase.MustParseBaseFromString("win@2012r2"),
	},
	{
		"Windows Storage Server 2012 Standard",
		corebase.MustParseBaseFromString("win@2012"),
	},
	{
		"Windows Storage Server 2012 R2 Standard",
		corebase.MustParseBaseFromString("win@2012r2"),
	},
	{
		"Windows 7 Home",
		corebase.MustParseBaseFromString("win@7"),
	},
	{
		"Windows 8 Pro",
		corebase.MustParseBaseFromString("win@8"),
	},
	{
		"Windows 8.1 Pro",
		corebase.MustParseBaseFromString("win@81"),
	},
}

func (s *windowsBaseSuite) SetUpTest(c *tc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.createRegKey(c, &currentVersionKey)
}

func (s *windowsBaseSuite) createRegKey(c *tc.C, key *string) {
	salt, err := RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	regKey := fmt.Sprintf(`SOFTWARE\JUJU\%s`, salt)
	s.PatchValue(key, regKey)

	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, *key, registry.ALL_ACCESS)
	c.Assert(err, tc.ErrorIsNil)

	err = k.Close()
	c.Assert(err, tc.ErrorIsNil)

	s.AddCleanup(func(*tc.C) {
		registry.DeleteKey(registry.LOCAL_MACHINE, currentVersionKey)
	})
}

func (s *windowsBaseSuite) TestReadBase(c *tc.C) {
	for _, value := range versionTests {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.ALL_ACCESS)
		c.Assert(err, tc.ErrorIsNil)

		err = k.SetStringValue("ProductName", value.version)
		c.Assert(err, tc.ErrorIsNil)

		err = k.Close()
		c.Assert(err, tc.ErrorIsNil)

		ver, err := readBase()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ver, tc.Equals, value.want)
	}
}

type windowsNanoBaseSuite struct {
	windowsBaseSuite
}

func TestWindowsNanoBaseSuite(t *stdtesting.T) {
	tc.Run(t, &windowsNanoBaseSuite{})
}

func (s *windowsNanoBaseSuite) SetUpTest(c *tc.C) {
	s.windowsBaseSuite.SetUpTest(c)
	s.createRegKey(c, &isNanoKey)

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, isNanoKey, registry.ALL_ACCESS)
	c.Assert(err, tc.ErrorIsNil)

	err = k.SetDWordValue("NanoServer", 1)
	c.Assert(err, tc.ErrorIsNil)

	err = k.Close()
	c.Assert(err, tc.ErrorIsNil)
}

var nanoVersionTests = []struct {
	version string
	want    corebase.Base
}{{
	"Windows Server 2016",
	corebase.MustParseBaseFromString("win@2016nano"),
}}

func (s *windowsNanoBaseSuite) TestReadBase(c *tc.C) {
	for _, value := range nanoVersionTests {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.ALL_ACCESS)
		c.Assert(err, tc.ErrorIsNil)

		err = k.SetStringValue("ProductName", value.version)
		c.Assert(err, tc.ErrorIsNil)

		err = k.Close()
		c.Assert(err, tc.ErrorIsNil)

		ver, err := readBase()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ver, tc.Equals, value.want)
	}
}

// RandomBytes returns n random bytes.
func RandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return nil, errors.Errorf("cannot read random bytes: %v", err)
	}
	return buf, nil
}

// RandomPassword generates a random base64-encoded password.
func RandomPassword() (string, error) {
	b, err := RandomBytes(randomPasswordBytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
