// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"io/ioutil"
	"os"
	"path/filepath"

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
	var f configFlag
	c.Assert(f.Set("a.yaml"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml"}, nil)
	c.Assert(f.Set("b.yaml"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, nil)
	c.Assert(f.Set("k1=v1"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "v1"})
	c.Assert(f.Set("k1==v2"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2"})
	c.Assert(f.Set("k2=3"), jc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": 3})
}

func (*FlagsSuite) TestConfigFlagSetErrors(c *gc.C) {
	var f configFlag
	c.Assert(f.Set(""), gc.ErrorMatches, "empty string not valid")
	c.Assert(f.Set("x=!"), gc.ErrorMatches, "yaml: did not find URI escaped octet")
}

func (*FlagsSuite) TestConfigFlagString(c *gc.C) {
	var f configFlag
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

	var f configFlag
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{})
	f.files = append(f.files, configFile1)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'out"})
	f.files = append(f.files, configFile2)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'under"})
	f.attrs = map[string]interface{}{"over": "ridden"}
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "ridden"})
}

func (*FlagsSuite) TestConfigFlagReadAttrsErrors(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")

	var f configFlag
	f.files = append(f.files, configFile)
	ctx := testing.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
	c.Assert(attrs, gc.IsNil)
}

func assertConfigFlag(c *gc.C, f configFlag, files []string, attrs map[string]interface{}) {
	c.Assert(f.files, jc.DeepEquals, files)
	c.Assert(f.attrs, jc.DeepEquals, attrs)
}

func assertConfigFlagReadAttrs(c *gc.C, f configFlag, expect map[string]interface{}) {
	ctx := testing.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, expect)
}
