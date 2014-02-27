// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type formatSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&formatSuite{})

// The agentParams are used by the specific formatter whitebox tests, and is
// located here for easy reuse.
var agentParams = AgentConfigParams{
	Tag:            "omg",
	Password:       "sekrit",
	CACert:         []byte("ca cert"),
	StateAddresses: []string{"localhost:1234"},
	APIAddresses:   []string{"localhost:1235"},
	Nonce:          "a nonce",
}

func (*formatSuite) TestReadFormatEmptyDir(c *gc.C) {
	// Since 1.12 is deprecated (and did have a format file),
	// if the format is missing, it should assume 1.18 or later.
	dir := c.MkDir()
	format, err := readFormat(dir)
	c.Assert(format, gc.Equals, currentFormat)
	c.Assert(err, gc.IsNil)
}

func (*formatSuite) TestReadFormat(c *gc.C) {
	dir := c.MkDir()
	// Make sure that the write adds the carriage return to show that
	// this is stripped off for the returned format.
	err := ioutil.WriteFile(filepath.Join(dir, legacyFormatFilename), []byte("some format\n"), 0644)
	c.Assert(err, gc.IsNil)
	format, err := readFormat(dir)
	c.Assert(format, gc.Equals, "some format")
	c.Assert(err, gc.IsNil)
}

func (*formatSuite) TestNewFormatter(c *gc.C) {
	formatter, err := newFormatter(currentFormat)
	c.Assert(formatter, gc.NotNil)
	c.Assert(err, gc.IsNil)

	formatter, err = newFormatter(previousFormat)
	c.Assert(formatter, gc.NotNil)
	c.Assert(err, gc.IsNil)

	formatter, err = newFormatter("other")
	c.Assert(formatter, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown agent config format "other"`)
}

func (*formatSuite) TestWriteFormat(c *gc.C) {
	dir := c.MkDir()
	testDir := filepath.Join(dir, "test")
	err := writeFormatFile(testDir, "some format")
	c.Assert(err, gc.IsNil)
	format, err := readFormat(testDir)
	c.Assert(format, gc.Equals, "some format")
	c.Assert(err, gc.IsNil)
	// Make sure the carriage return is there as it makes catting the file nicer.
	content, err := ioutil.ReadFile(filepath.Join(testDir, legacyFormatFilename))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "some format\n")
}

func (*formatSuite) TestWriteCommandsForFormat(c *gc.C) {
	dir := c.MkDir()
	testDir := filepath.Join(dir, "test")
	commands := writeCommandsForFormat(testDir, "some format")
	c.Assert(commands, gc.HasLen, 3)
	c.Assert(commands[0], gc.Matches, `mkdir -p \S+`)
	c.Assert(commands[1], gc.Matches, `install -m 644 /dev/null '\S+/format'`)
	c.Assert(commands[2], gc.Matches, `printf '%s\\n' '.*' > '\S+/format'`)
}

func (*formatSuite) TestReadPreviousFormatWritesNew(c *gc.C) {
	config := newTestConfig(c)

	err := previousFormatter.write(config)
	c.Assert(err, gc.IsNil)

	_, err = ReadConf(ConfigPath(config.DataDir(), config.Tag()))
	c.Assert(err, gc.IsNil)
	format, err := readFormat(config.Dir())
	c.Assert(err, gc.IsNil)
	c.Assert(format, gc.Equals, currentFormat)
}
