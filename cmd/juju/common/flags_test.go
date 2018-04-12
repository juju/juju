// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type FlagsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&FlagsSuite{})

func (*FlagsSuite) TestConfigFlagSet(c *gc.C) {
	var f ConfigFlag
	c.Assert(f.Set("a.yaml"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml"}, nil)
	c.Assert(f.Set("b.yaml"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, nil)
	c.Assert(f.Set("k1=v1"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "v1"})
	c.Assert(f.Set("k1="), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": ""})
	c.Assert(f.Set("k1=v1"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "v1"})
	c.Assert(f.Set(`k1=""`), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": ""})
	c.Assert(f.Set("k1==v2"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2"})
	c.Assert(f.Set("k2=3"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": 3})
	c.Assert(f.Set("k3="), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": 3, "k3": ""})
	f.SetPreserveStringValue(true)
	c.Assert(f.Set("k4=4.0"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": 3, "k3": "", "k4": "4.0"})
}

func (*FlagsSuite) TestConfigFlagSetErrors(c *gc.C) {
	var f ConfigFlag
	c.Assert(f.Set(""), gc.ErrorMatches, "empty string not valid")
	c.Assert(f.Set("x=!"), gc.ErrorMatches, "yaml: did not find URI escaped octet")
}

func (*FlagsSuite) TestConfigFlagString(c *gc.C) {
	var f ConfigFlag
	c.Assert(f.String(), gc.Equals, "")
	f.files = append(f.files, "a.yaml")
	c.Assert(f.String(), gc.Equals, "a.yaml")
	f.files = append(f.files, "b.yaml")
	c.Assert(f.String(), gc.Equals, "a.yaml b.yaml")
	f.files = append(f.files, "x=y")
	c.Assert(f.String(), gc.Equals, "a.yaml b.yaml x=y")
	f.files = append(f.files, "zz=y")
	c.Assert(f.String(), gc.Equals, "a.yaml b.yaml x=y zz=y")
}

func (*FlagsSuite) TestConfigFlagReadAttrs(c *gc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")
	err := ioutil.WriteFile(configFile1, []byte(`over: "'n'out"`+"\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(configFile2, []byte(`over: "'n'under"`+"\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	var f ConfigFlag
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{})
	f.files = append(f.files, configFile1)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'out"})
	f.files = append(f.files, configFile2)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'under"})
	f.attrs = map[string]interface{}{"over": "ridden"}
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "ridden"})
}

func (*FlagsSuite) TestConfigFlagReadConfigPairs(c *gc.C) {
	ctx := cmdtesting.Context(c)
	configFile1 := filepath.Join(ctx.Dir, "config-1.yaml")
	err := ioutil.WriteFile(configFile1, []byte(`over: "'n'out"`+"\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	var f ConfigFlag
	f.files = append(f.files, configFile1)
	f.attrs = map[string]interface{}{"key": "value"}
	attrs, err := f.ReadConfigPairs(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, map[string]interface{}{"key": "value"})
}

func (*FlagsSuite) TestConfigFlagReadAttrsErrors(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")

	var f ConfigFlag
	f.files = append(f.files, configFile)
	ctx := cmdtesting.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
	c.Assert(attrs, gc.IsNil)
}

func (*FlagsSuite) TestAbsoluteFilenames(c *gc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")

	var f ConfigFlag
	f.files = append(f.files, configFile1)
	f.files = append(f.files, configFile2)
	ctx := cmdtesting.Context(c)
	files, err := f.AbsoluteFileNames(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(files, jc.DeepEquals, []string{
		configFile1, configFile2,
	})
}

func assertConfigFlag(c *gc.C, f ConfigFlag, files []string, attrs map[string]interface{}) {
	c.Assert(f.files, jc.DeepEquals, files)
	c.Assert(f.attrs, jc.DeepEquals, attrs)
}

func assertConfigFlagReadAttrs(c *gc.C, f ConfigFlag, expect map[string]interface{}) {
	ctx := cmdtesting.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, expect)
}
