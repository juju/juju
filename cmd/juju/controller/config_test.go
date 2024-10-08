// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
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
		err:  "cannot specify multiple keys to get",
	}, {
		desc: "set one key",
		args: []string{"juju-ha-space=value"},
	}, {
		desc: "set two keys",
		args: []string{"juju-ha-space=value", "juju-mgmt-space=value"},
	}, {
		desc: "can't mix setting and getting",
		args: []string{"juju-ha-space=value", "hey2"},
		err:  "cannot get value and set key=value pairs simultaneously",
	}, {
		desc: "can't reset",
		args: []string{"--reset", "key1,key2"},
		err:  "option provided but not defined: --reset",
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
	c.Assert(err, gc.ErrorMatches, `key "courtney-barnett" not found in controller "mallards"`)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigSuite) TestSetReadOnly(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "api-port=123")
	c.Assert(err, gc.ErrorMatches, `invalid or read-only controller config values cannot be updated: \[api-port\]`)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigSuite) TestSetWrongType(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "audit-log-max-backups=foo")
	c.Assert(err, gc.ErrorMatches, `audit-log-max-backups: expected number, got string\("foo"\)`)
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
	context, err := s.runWithAPI(c, &api, "juju-ha-space=value")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"juju-ha-space": "value"})
}

func (s *ConfigSuite) TestSettingDuration(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "api-port-open-delay=100ms")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"api-port-open-delay": "100ms"})
}

func (s *ConfigSuite) TestSettingFromFile(c *gc.C) {
	path := writeFile(c, "yaml", "juju-ha-space: value\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "--file", path)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"juju-ha-space": "value"})
}

func (s *ConfigSuite) TestSettingFromStdin(c *gc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("juju-ha-space: value\n")
	var api fakeControllerAPI
	code := cmd.Main(controller.NewConfigCommandForTest(&api, s.store), ctx,
		[]string{"--file", "-"})

	c.Assert(code, gc.Equals, 0)
	output := strings.TrimSpace(cmdtesting.Stdout(ctx))
	c.Assert(output, gc.Equals, "")
	stderr := strings.TrimSpace(cmdtesting.Stderr(ctx))
	c.Assert(stderr, gc.Equals, "")
	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"juju-ha-space": "value"})
}

func (s *ConfigSuite) TestOverrideFileFromArgs(c *gc.C) {
	path := writeFile(c, "yaml", "juju-ha-space: value\naudit-log-max-backups: 2\n")
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, "--file", path, "audit-log-max-backups=4")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")

	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{
		"juju-ha-space":         "value",
		"audit-log-max-backups": 4,
	})
}

func (s *ConfigSuite) TestSetReadOnlyControllerName(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, `controller-name=new-name`)
	c.Assert(err, gc.ErrorMatches, `invalid or read-only controller config values cannot be updated: \[controller-name\]`)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigSuite) TestSetReadOnlyInvalidControllerName(c *gc.C) {
	var api fakeControllerAPI
	context, err := s.runWithAPI(c, &api, `controller-name=-new-name-`)
	c.Assert(err, gc.ErrorMatches, `invalid or read-only controller config values cannot be updated: \[controller-name\]`)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigSuite) TestErrorOnSetting(c *gc.C) {
	api := fakeControllerAPI{err: errors.Errorf("kablooie")}
	context, err := s.runWithAPI(c, &api, "juju-ha-space=value")
	c.Assert(err, gc.ErrorMatches, "kablooie")

	c.Assert(strings.TrimSpace(cmdtesting.Stdout(context)), gc.Equals, "")
	c.Assert(api.values, gc.DeepEquals, map[string]interface{}{"juju-ha-space": "value"})
}

func writeFile(c *gc.C, name, content string) string {
	path := filepath.Join(c.MkDir(), name)
	err := os.WriteFile(path, []byte(content), 0777)
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

func (f *fakeControllerAPI) ControllerConfig(context.Context) (jujucontroller.Config, error) {
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

func (f *fakeControllerAPI) ConfigSet(ctx context.Context, values map[string]interface{}) error {
	if f.values == nil {
		f.values = values
	} else {
		// Append values rather than overwriting
		for key, val := range values {
			f.values[key] = val
		}
	}
	return f.err
}
