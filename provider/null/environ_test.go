// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"errors"
	"io"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type environSuite struct {
	testbase.LoggingSuite
	env *nullEnviron
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	envConfig := getEnvironConfig(c, minimalConfigValues())
	s.env = &nullEnviron{cfg: envConfig}
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	err := s.env.SetConfig(minimalConfig(c))
	c.Assert(err, gc.IsNil)

	testConfig := minimalConfig(c)
	testConfig, err = testConfig.Apply(map[string]interface{}{"bootstrap-host": ""})
	c.Assert(err, gc.IsNil)
	err = s.env.SetConfig(testConfig)
	c.Assert(err, gc.ErrorMatches, "bootstrap-host must be specified")
}

func (s *environSuite) TestInstances(c *gc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 0)

	ids = append(ids, manual.BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.NotNil)

	ids = append(ids, manual.BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 2)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)

	ids = append(ids, instance.Id("invalid"))
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, gc.HasLen, 3)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)
	c.Assert(instances[2], gc.IsNil)

	ids = []instance.Id{instance.Id("invalid")}
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.IsNil)
}

func (s *environSuite) TestDestroy(c *gc.C) {
	var resultStderr string
	var resultErr error
	runSSHCommandTesting := func(host string, command []string) (string, error) {
		c.Assert(host, gc.Equals, "ubuntu@hostname")
		c.Assert(command, gc.DeepEquals, []string{"sudo", "pkill", "-6", "jujud"})
		return resultStderr, resultErr
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)
	type test struct {
		stderr string
		err    error
		match  string
	}
	tests := []test{
		{"", nil, ""},
		{"abc", nil, ""},
		{"", errors.New("oh noes"), "oh noes"},
		{"123", errors.New("abc"), "abc \\(123\\)"},
	}
	for i, t := range tests {
		c.Logf("test %d: %v", i, t)
		resultStderr, resultErr = t.stderr, t.err
		err := s.env.Destroy()
		if t.match == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.match)
		}
	}
}

func (s *environSuite) TestLocalStorageConfig(c *gc.C) {
	c.Assert(s.env.StorageDir(), gc.Equals, "/var/lib/juju/storage")
	c.Assert(s.env.cfg.storageListenAddr(), gc.Equals, ":8040")
	c.Assert(s.env.StorageAddr(), gc.Equals, s.env.cfg.storageListenAddr())
	c.Assert(s.env.SharedStorageAddr(), gc.Equals, "")
	c.Assert(s.env.SharedStorageDir(), gc.Equals, "")
}

func (s *environSuite) TestEnvironSupportsCustomSources(c *gc.C) {
	sources, err := tools.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	url, err := sources[0].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(url, "/tools"), jc.IsTrue)
}

type dummyStorage struct {
	storage.Storage
}

func (s *environSuite) TestEnvironBootstrapStorager(c *gc.C) {
	var newSSHStorageResult = struct {
		stor storage.Storage
		err  error
	}{dummyStorage{}, errors.New("failed to get SSH storage")}
	s.PatchValue(&newSSHStorage, func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return newSSHStorageResult.stor, newSSHStorageResult.err
	})

	var initUbuntuResult error
	s.PatchValue(&initUbuntuUser, func(host, user, authorizedKeys string, stdin io.Reader, stdout io.Writer) error {
		return initUbuntuResult
	})

	ctx := envtesting.NewBootstrapContext(coretesting.Context(c))
	initUbuntuResult = errors.New("failed to initialise ubuntu user")
	c.Assert(s.env.EnableBootstrapStorage(ctx), gc.Equals, initUbuntuResult)
	initUbuntuResult = nil
	c.Assert(s.env.EnableBootstrapStorage(ctx), gc.Equals, newSSHStorageResult.err)
	// after the user is initialised once successfully,
	// another attempt will not be made.
	initUbuntuResult = errors.New("failed to initialise ubuntu user")
	c.Assert(s.env.EnableBootstrapStorage(ctx), gc.Equals, newSSHStorageResult.err)

	// after the bootstrap storage is initialised once successfully,
	// another attempt will not be made.
	backup := newSSHStorageResult.err
	newSSHStorageResult.err = nil
	c.Assert(s.env.EnableBootstrapStorage(ctx), gc.IsNil)
	newSSHStorageResult.err = backup
	c.Assert(s.env.EnableBootstrapStorage(ctx), gc.IsNil)
}
