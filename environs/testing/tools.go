package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"strings"
)

func uploadFakeToolsVersion(s storage.ReadWriter, vers version.Binary) (*state.Tools, error) {
	data := vers.String()
	name := tools.StorageName(vers)
	if err := s.Put(name, strings.NewReader(data), int64(len(data))); err != nil {
		return nil, err
	}
	url, err := s.URL(name)
	if err != nil {
		return nil, err
	}
	return &state.Tools{Binary: vers, URL: url}, nil
}

// UploadFakeToolsVersion puts fake tools in the supplied storage for the
// supplied version.
func UploadFakeToolsVersion(c *C, s storage.ReadWriter, vers version.Binary) *state.Tools {
	t, err := uploadFakeToolsVersion(s, vers)
	c.Assert(err, IsNil)
	return t
}

// MustUploadFakeToolsVersion acts as UploadFakeToolsVersion, but panics on failure.
func MustUploadFakeToolsVersion(s storage.ReadWriter, vers version.Binary) *state.Tools {
	t, err := uploadFakeToolsVersion(s, vers)
	if err != nil {
		panic(err)
	}
	return t
}

func uploadFakeTools(s storage.ReadWriter) error {
	toolsVersion := version.Current
	if _, err := uploadFakeToolsVersion(s, toolsVersion); err != nil {
		return err
	}
	if toolsVersion.Series == config.DefaultSeries {
		return nil
	}
	toolsVersion.Series = config.DefaultSeries
	_, err := uploadFakeToolsVersion(s, toolsVersion)
	return err
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching version.Current; if version.Current's series is different
// to config.DefaultSeries, matching fake tools will be uploaded for that series.
// This is useful for tests that are kinda casual about specifying their
// environment.
func UploadFakeTools(c *C, s storage.ReadWriter) {
	c.Assert(uploadFakeTools(s), IsNil)
}

// MustUploadFakeTools acts as UploadFakeTools, but panics on failure.
func MustUploadFakeTools(s storage.ReadWriter) {
	if err := uploadFakeTools(s); err != nil {
		panic(err)
	}
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *C, s storage.ReadWriter) {
	names, err := s.List("tools/juju-")
	c.Assert(err, IsNil)
	for _, name := range names {
		err = s.Remove(name)
		c.Assert(err, IsNil)
	}
}
