package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/version"
	"strings"
)

// PutFakeTools sets up a bucket containing something
// that looks like a tools archive so test methods
// that start an instance can succeed even though they
// do not upload tools.
func PutFakeTools(c *C, s environs.StorageWriter) {
	toolsVersion := version.Current
	path := environs.ToolsStoragePath(toolsVersion)
	c.Logf("putting fake tools at %v", path)
	toolsContents := "tools archive, honest guv"
	err := s.Put(path, strings.NewReader(toolsContents), int64(len(toolsContents)))
	c.Assert(err, IsNil)
	if toolsVersion.Series != version.DefaultSeries() {
		toolsVersion.Series = version.DefaultSeries()
		path = environs.ToolsStoragePath(toolsVersion)
		c.Logf("putting fake tools at %v", path)
		err = s.Put(path, strings.NewReader(toolsContents), int64(len(toolsContents)))
		c.Assert(err, IsNil)
	}
}
