// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type FlagsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestFlagsSuite(t *stdtesting.T) {
	tc.Run(t, &FlagsSuite{})
}

func (*FlagsSuite) TestConfigFlagSet(c *tc.C) {
	var f ConfigFlag
	c.Assert(f.Set("a.yaml"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml"}, nil)
	c.Assert(f.Set("b.yaml"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, nil)
	c.Assert(f.Set("k1=v1"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "v1"})
	c.Assert(f.Set("k1="), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": ""})
	c.Assert(f.Set("k1=v1"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "v1"})
	c.Assert(f.Set("k1==v2"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2"})
	c.Assert(f.Set("k2=3"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": "3"})
	c.Assert(f.Set("k3="), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": "3", "k3": ""})
	c.Assert(f.Set("k4=4.0"), tc.ErrorIsNil)
	assertConfigFlag(c, f, []string{"a.yaml", "b.yaml"}, map[string]interface{}{"k1": "=v2", "k2": "3", "k3": "", "k4": "4.0"})
}

func (*FlagsSuite) TestConfigFlagSetErrors(c *tc.C) {
	var f ConfigFlag
	c.Assert(f.Set(""), tc.ErrorMatches, "empty string not valid")
}

func (*FlagsSuite) TestConfigFlagSetAttrsFromReader(c *tc.C) {
	yaml := `
foo: 1
bar: 2
`[1:]

	var f ConfigFlag
	c.Assert(f.SetAttrsFromReader(bytes.NewBufferString(yaml)), tc.ErrorIsNil)
	assertConfigFlag(c, f, nil, map[string]interface{}{"foo": 1, "bar": 2})

	yaml = `
foo: 3
baz: 4
`[1:]
	c.Assert(f.SetAttrsFromReader(bytes.NewBufferString(yaml)), tc.ErrorIsNil)
	assertConfigFlag(c, f, nil, map[string]interface{}{"foo": 3, "bar": 2, "baz": 4})
}

func (*FlagsSuite) TestConfigFlagSetAttrsFromReaderErrors(c *tc.C) {
	var f ConfigFlag
	c.Assert(f.SetAttrsFromReader(nil), tc.ErrorMatches, "empty reader not valid")
	c.Assert(f.SetAttrsFromReader(bytes.NewBufferString("!?@>£")), tc.ErrorMatches, "yaml: did not find expected whitespace or line break")
}

func (*FlagsSuite) TestConfigFlagString(c *tc.C) {
	var f ConfigFlag
	c.Assert(f.String(), tc.Equals, "")
	f.files = append(f.files, "a.yaml")
	c.Assert(f.String(), tc.Equals, "a.yaml")
	f.files = append(f.files, "b.yaml")
	c.Assert(f.String(), tc.Equals, "a.yaml b.yaml")
	f.files = append(f.files, "x=y")
	c.Assert(f.String(), tc.Equals, "a.yaml b.yaml x=y")
	f.files = append(f.files, "zz=y")
	c.Assert(f.String(), tc.Equals, "a.yaml b.yaml x=y zz=y")
}

func (*FlagsSuite) TestConfigFlagReadAttrs(c *tc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")
	err := os.WriteFile(configFile1, []byte(`over: "'n'out"`+"\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(configFile2, []byte(`over: "'n'under"`+"\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	var f ConfigFlag
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{})
	f.files = append(f.files, configFile1)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'out"})
	f.files = append(f.files, configFile2)
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "'n'under"})
	f.attrs = map[string]interface{}{"over": "ridden"}
	assertConfigFlagReadAttrs(c, f, map[string]interface{}{"over": "ridden"})
}

func (*FlagsSuite) TestConfigFlagReadConfigPairs(c *tc.C) {
	ctx := cmdtesting.Context(c)
	configFile1 := filepath.Join(ctx.Dir, "config-1.yaml")
	err := os.WriteFile(configFile1, []byte(`over: "'n'out"`+"\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	var f ConfigFlag
	f.files = append(f.files, configFile1)
	f.attrs = map[string]interface{}{"key": "value"}
	attrs, err := f.ReadConfigPairs(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attrs, tc.DeepEquals, map[string]interface{}{"key": "value"})
}

func (*FlagsSuite) TestConfigFlagReadAttrsErrors(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")

	var f ConfigFlag
	f.files = append(f.files, configFile)
	ctx := cmdtesting.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(errors.Cause(err), tc.Satisfies, os.IsNotExist)
	c.Assert(attrs, tc.IsNil)
}

func (*FlagsSuite) TestAbsoluteFilenames(c *tc.C) {
	tmpdir := c.MkDir()
	configFile1 := filepath.Join(tmpdir, "config-1.yaml")
	configFile2 := filepath.Join(tmpdir, "config-2.yaml")

	var f ConfigFlag
	f.files = append(f.files, configFile1)
	f.files = append(f.files, configFile2)
	ctx := cmdtesting.Context(c)
	files, err := f.AbsoluteFileNames(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(files, tc.DeepEquals, []string{
		configFile1, configFile2,
	})
}

func assertConfigFlag(c *tc.C, f ConfigFlag, files []string, attrs map[string]interface{}) {
	c.Assert(f.files, tc.DeepEquals, files)
	c.Assert(f.attrs, tc.DeepEquals, attrs)
}

func assertConfigFlagReadAttrs(c *tc.C, f ConfigFlag, expect map[string]interface{}) {
	ctx := cmdtesting.Context(c)
	attrs, err := f.ReadAttrs(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attrs, tc.DeepEquals, expect)
}

func (*FlagsSuite) TestAutoBoolValue(c *tc.C) {
	var f AutoBoolValue
	c.Assert(f.Get(), tc.IsNil)
	c.Assert(f.String(), tc.Equals, "nil")

	c.Assert(f.Set("true"), tc.ErrorIsNil)
	c.Assert(*f.Get(), tc.IsTrue)
	c.Assert(f.String(), tc.Equals, "true")

	c.Assert(f.Set("false"), tc.ErrorIsNil)
	c.Assert(*f.Get(), tc.IsFalse)
	c.Assert(f.String(), tc.Equals, "false")

	c.Assert(f.Set(""), tc.ErrorMatches, `strconv.ParseBool: parsing "": invalid syntax`)
	c.Assert(f.Set("non-bool"), tc.ErrorMatches, `strconv.ParseBool: parsing "non-bool": invalid syntax`)

	c.Assert(f.IsBoolFlag(), tc.IsTrue)
}
