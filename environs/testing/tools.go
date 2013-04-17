package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"strings"
)

func uploadFakeToolsVersion(storage environs.Storage, vers version.Binary) (*state.Tools, error) {
	data := vers.String()
	name := tools.StorageName(vers)
	if err := storage.Put(name, strings.NewReader(data), int64(len(data))); err != nil {
		return nil, err
	}
	url, err := storage.URL(name)
	if err != nil {
		return nil, err
	}
	return &state.Tools{Binary: vers, URL: url}, nil
}

// UploadFakeToolsVersion puts fake tools in the supplied storage for the
// supplied version.
func UploadFakeToolsVersion(c *C, storage environs.Storage, vers version.Binary) *state.Tools {
	t, err := uploadFakeToolsVersion(storage, vers)
	c.Assert(err, IsNil)
	return t
}

// MustUploadFakeToolsVersion acts as UploadFakeToolsVersion, but panics on failure.
func MustUploadFakeToolsVersion(storage environs.Storage, vers version.Binary) *state.Tools {
	t, err := uploadFakeToolsVersion(storage, vers)
	if err != nil {
		panic(err)
	}
	return t
}

func uploadFakeTools(storage environs.Storage) error {
	toolsVersion := version.Current
	if _, err := uploadFakeToolsVersion(storage, toolsVersion); err != nil {
		return err
	}
	if toolsVersion.Series == config.DefaultSeries {
		return nil
	}
	toolsVersion.Series = config.DefaultSeries
	_, err := uploadFakeToolsVersion(storage, toolsVersion)
	return err
}

// UploadFakeTools puts fake tools into the supplied storage with a binary
// version matching version.Current; if version.Current's series is different
// to config.DefaultSeries, matching fake tools will be uploaded for that series.
// This is useful for tests that are kinda casual about specifying their
// environment.
func UploadFakeTools(c *C, storage environs.Storage) {
	c.Assert(uploadFakeTools(storage), IsNil)
}

// MustUploadFakeTools acts as UploadFakeTools, but panics on failure.
func MustUploadFakeTools(storage environs.Storage) {
	if err := uploadFakeTools(storage); err != nil {
		panic(err)
	}
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *C, storage environs.Storage) {
	names, err := storage.List("tools/juju-")
	c.Assert(err, IsNil)
	for _, name := range names {
		err = storage.Remove(name)
		c.Assert(err, IsNil)
	}
}

// RemoveAllTools deletes all tools from the supplied environment.
func RemoveAllTools(c *C, env environs.Environ) {
	RemoveTools(c, env.Storage())
	RemoveTools(c, env.PublicStorage().(environs.Storage))
}
