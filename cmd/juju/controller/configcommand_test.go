// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	jujucontroller "github.com/juju/juju/controller"
)

type ConfigSuite struct {
	baseControllerSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.baseControllerSuite.SetUpTest(c)
	s.createTestClientStore(c)
}

func (s *ConfigSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return s.runWithAPI(c, &fakeControllerAPI{}, args...)
}

func (s *ConfigSuite) runWithAPI(c *gc.C, api *fakeControllerAPI, args ...string) (*cmd.Context, error) {
	command := controller.NewConfigCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *ConfigSuite) TestInit(c *gc.C) {
	path := writeFile(c, "yamlfile", "")

	tests := []struct {
		desc string
		args []string
		err  string
	}{{
		desc: "no args",
	}, {
		desc: "get one key",
		args: []string{"one"},
	}, {
		desc: "can't get two keys",
		args: []string{"one", "two"},
		err:  "can only retrieve a single value, or all values",
	}, {
		desc: "set one key",
		args: []string{"key=value"},
	}, {
		desc: "set two keys",
		args: []string{"key1=value", "key2=value"},
	}, {
		desc: "can't mix setting and getting",
		args: []string{"key1=value", "hey2"},
		err:  "can only retrieve a single value, or all values",
	}, {
		desc: "can mix setting with files",
		args: []string{"key1=value", path},
	}}
	for i, test := range tests {
		c.Logf("%d - %s", i, test.desc)
		err := cmdtesting.InitCommand(controller.NewConfigCommandForTest(&fakeControllerAPI{}, s.store), test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ConfigSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "ca-cert")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "multi\nline")
}

func (s *ConfigSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "controller-uuid")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, `"uuid"`)
}

func (s *ConfigSuite) TestAllValues(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := `
Attribute                  Value
api-port                   1234
audit-log-exclude-methods  
- Thing1
- Thing2
ca-cert  |-
  multi
  line
controller-uuid  uuid`[1:]
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigSuite) TestOneLineExcludeMethods(c *gc.C) {
	var api fakeControllerAPI
	api.config = map[string]interface{}{
		"audit-log-exclude-methods": []string{"Actual.Size"},
	}
	context, err := s.runWithAPI(c, &api)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := `
Attribute                  Value
audit-log-exclude-methods  Actual.Size`[1:]
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := `{"api-port":1234,"audit-log-exclude-methods":["Thing1","Thing2"],"ca-cert":"multi\nline","controller-uuid":"uuid"}`
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigSuite) TestNonexistentValue(c *gc.C) {
	context, err := s.run(c, "courtney-barnett")
	c.Assert(err, gc.ErrorMatches, `key "courtney-barnett" not found in "mallards" controller`)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigSuite) TestError(c *gc.C) {
	command := controller.NewConfigCommandForTest(&fakeControllerAPI{err: errors.New("error")}, s.store)
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, "error")
}

func (s *ConfigSuite) TestSettingKey(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "key1=value")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"key1": "value"})
}

func (s *ConfigSuite) TestSettingComplexKey(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "key1=[value1,value2]")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"key1": []interface{}{"value1", "value2"},
	})
}

func (s *ConfigSuite) TestSettingFromFile(c *gc.C) {
	path := writeFile(c, "yaml", "key1: value\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, path)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"key1": "value"})
}

func (s *ConfigSuite) TestSettingFromBothNoOverlap(c *gc.C) {
	path := writeFile(c, "yaml", "key1: value\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, path, "key2=123")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"key1": "value",
		"key2": 123,
	})
}

func (s *ConfigSuite) TestSettingFromBothArgFirst(c *gc.C) {
	path := writeFile(c, "yaml", "key1: value\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "key1=123", path)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	// This is a consequence of ConfigFlag reading input files first
	// and then overlaying with values from args. It's surprising but
	// probably not worth fixing - I don't think people will try to
	// set an option from a file and then override it from an arg.
	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"key1": 123,
	})
}

func (s *ConfigSuite) TestSettingFromBothFileFirst(c *gc.C) {
	path := writeFile(c, "yaml", "key1: value\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, path, "key1=123")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"key1": 123,
	})
}

func (s *ConfigSuite) TestSettingMultipleFiles(c *gc.C) {
	path1 := writeFile(c, "yaml1", "key1: value\n")
	path2 := writeFile(c, "yaml2", "key1: 123\n")

	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, path1, path2)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"key1": 123,
	})
}

func (s *ConfigSuite) TestErrorOnSetting(c *gc.C) {
	api := fakeControllerAPI{err: errors.Errorf("kablooie")}
	context, err := s.runWithAPI(c, &api, "key=value")
	c.Assert(err, gc.ErrorMatches, "kablooie")

	c.Assert(strings.TrimSpace(cmdtesting.Stdout(context)), gc.Equals, "")
	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"key": "value"})
}

func writeFile(c *gc.C, name, content string) string {
	path := filepath.Join(c.MkDir(), name)
	err := ioutil.WriteFile(path, []byte(content), 0777)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

type fakeControllerAPI struct {
	err    error
	config map[string]interface{}
	values map[string]interface{}
}

func (f *fakeControllerAPI) Close() error {
	return nil
}

func (f *fakeControllerAPI) ControllerConfig() (jujucontroller.Config, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result map[string]interface{}
	if f.config != nil {
		result = f.config
	} else {
		result = map[string]interface{}{
			"controller-uuid":           "uuid",
			"api-port":                  1234,
			"ca-cert":                   "multi\nline",
			"audit-log-exclude-methods": []interface{}{"Thing1", "Thing2"},
		}
	}
	return result, nil
}

func (f *fakeControllerAPI) ConfigSet(values map[string]interface{}) error {
	f.values = values
	return f.err
}
