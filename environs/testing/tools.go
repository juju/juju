// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
)

func uploadFakeToolsVersion(storage environs.Storage, vers version.Binary) (*tools.Tools, error) {
	data := vers.String()
	name := tools.StorageName(vers)
	log.Noticef("environs/testing: uploading FAKE tools %s", vers)
	if err := storage.Put(name, strings.NewReader(data), int64(len(data))); err != nil {
		return nil, err
	}
	url, err := storage.URL(name)
	if err != nil {
		return nil, err
	}
	return &tools.Tools{Binary: vers, URL: url}, nil
}

// UploadFakeToolsVersion puts fake tools in the supplied storage for the
// supplied version.
func UploadFakeToolsVersion(c *C, storage environs.Storage, vers version.Binary) *tools.Tools {
	t, err := uploadFakeToolsVersion(storage, vers)
	c.Assert(err, IsNil)
	return t
}

// MustUploadFakeToolsVersion acts as UploadFakeToolsVersion, but panics on failure.
func MustUploadFakeToolsVersion(storage environs.Storage, vers version.Binary) *tools.Tools {
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

// RemoveFakeTools deletes the fake tools from the supplied storage.
func RemoveFakeTools(c *C, storage environs.Storage) {
	toolsVersion := version.Current
	name := tools.StorageName(toolsVersion)
	err := storage.Remove(name)
	c.Check(err, IsNil)
	if version.Current.Series != config.DefaultSeries {
		toolsVersion.Series = config.DefaultSeries
		name := tools.StorageName(toolsVersion)
		err := storage.Remove(name)
		c.Check(err, IsNil)
	}
}

// RemoveTools deletes all tools from the supplied storage.
func RemoveTools(c *C, storage environs.Storage) {
	names, err := storage.List("tools/juju-")
	c.Assert(err, IsNil)
	c.Logf("removing files: %v", names)
	for _, name := range names {
		err = storage.Remove(name)
		c.Check(err, IsNil)
	}
}

// RemoveAllTools deletes all tools from the supplied environment.
func RemoveAllTools(c *C, env environs.Environ) {
	c.Logf("clearing private storage")
	RemoveTools(c, env.Storage())
	c.Logf("clearing public storage")
	RemoveTools(c, env.PublicStorage().(environs.Storage))
}
