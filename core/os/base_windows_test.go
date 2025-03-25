// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/sys/windows/registry"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/errors"
)

const randomPasswordBytes = 18

type windowsBaseSuite struct {
	testing.CleanupSuite
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

func (s *windowsBaseSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.createRegKey(c, &currentVersionKey)
}

func (s *windowsBaseSuite) createRegKey(c *gc.C, key *string) {
	salt, err := RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	regKey := fmt.Sprintf(`SOFTWARE\JUJU\%s`, salt)
	s.PatchValue(key, regKey)

	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, *key, registry.ALL_ACCESS)
	c.Assert(err, jc.ErrorIsNil)

	err = k.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.AddCleanup(func(*gc.C) {
		registry.DeleteKey(registry.LOCAL_MACHINE, currentVersionKey)
	})
}

func (s *windowsBaseSuite) TestReadBase(c *gc.C) {
	for _, value := range versionTests {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.ALL_ACCESS)
		c.Assert(err, jc.ErrorIsNil)

		err = k.SetStringValue("ProductName", value.version)
		c.Assert(err, jc.ErrorIsNil)

		err = k.Close()
		c.Assert(err, jc.ErrorIsNil)

		ver, err := readBase()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ver, gc.Equals, value.want)
	}
}

type windowsNanoBaseSuite struct {
	windowsBaseSuite
}

var _ = gc.Suite(&windowsNanoBaseSuite{})

func (s *windowsNanoBaseSuite) SetUpTest(c *gc.C) {
	s.windowsBaseSuite.SetUpTest(c)
	s.createRegKey(c, &isNanoKey)

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, isNanoKey, registry.ALL_ACCESS)
	c.Assert(err, jc.ErrorIsNil)

	err = k.SetDWordValue("NanoServer", 1)
	c.Assert(err, jc.ErrorIsNil)

	err = k.Close()
	c.Assert(err, jc.ErrorIsNil)
}

var nanoVersionTests = []struct {
	version string
	want    corebase.Base
}{{
	"Windows Server 2016",
	corebase.MustParseBaseFromString("win@2016nano"),
}}

func (s *windowsNanoBaseSuite) TestReadBase(c *gc.C) {
	for _, value := range nanoVersionTests {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.ALL_ACCESS)
		c.Assert(err, jc.ErrorIsNil)

		err = k.SetStringValue("ProductName", value.version)
		c.Assert(err, jc.ErrorIsNil)

		err = k.Close()
		c.Assert(err, jc.ErrorIsNil)

		ver, err := readBase()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ver, gc.Equals, value.want)
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
