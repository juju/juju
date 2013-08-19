// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
)

// EnvironConfig returns a default environment configuration suitable for
// testing.
func EnvironConfig(c *C) *config.Config {
	cfg, err := config.New(map[string]interface{}{
		"type":            "test",
		"name":            "test-name",
		"default-series":  "test-series",
		"authorized-keys": "test-keys",
		"agent-version":   "9.9.9.9",
		"ca-cert":         CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	return cfg
}

const (
	SampleEnvName = "erewhemos"
	EnvDefault = "default:\n  " + SampleEnvName + "\n"
)

// Environment names below are explicit as it makes them more readable.
const SingleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: conn-from-name-secret
`

const SingleEnvConfig = EnvDefault + SingleEnvConfigNoDefault

const MultipleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: conn-from-name-secret
    erewhemos-2:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: conn-from-name-secret
`

const MultipleEnvConfig = EnvDefault + MultipleEnvConfigNoDefault

const SampleCertName = "erewhemos"

type TestFile struct {
	Name, Data string
}

type FakeHome struct {
	oldHomeEnv     string
	oldJujuEnv     string
	oldJujuHomeEnv string
	oldJujuHome    string
	files          []TestFile
}

// MakeFakeHomeNoEnvironments creates a new temporary directory through the
// test checker, and overrides the HOME environment variable to point to this
// new temporary directory.
//
// No ~/.juju/environments.yaml exists, but CAKeys are written for each of the
// 'certNames' specified, and the id_rsa.pub file is written to to the .ssh
// dir.
func MakeFakeHomeNoEnvironments(c *C, certNames ...string) *FakeHome {
	fake := MakeEmptyFakeHome(c)

	for _, name := range certNames {
		err := ioutil.WriteFile(config.JujuHomePath(name+"-cert.pem"), []byte(CACert), 0600)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(config.JujuHomePath(name+"-private-key.pem"), []byte(CAKey), 0600)
		c.Assert(err, IsNil)
	}

	err := os.Mkdir(HomePath(".ssh"), 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(HomePath(".ssh", "id_rsa.pub"), []byte("auth key\n"), 0666)
	c.Assert(err, IsNil)

	return fake
}

// MakeFakeHome creates a new temporary directory through the test checker,
// and overrides the HOME environment variable to point to this new temporary
// directory.
//
// A new ~/.juju/environments.yaml file is created with the content of the
// `envConfig` parameter, and CAKeys are written for each of the 'certNames'
// specified.
func MakeFakeHome(c *C, envConfig string, certNames ...string) *FakeHome {
	fake := MakeFakeHomeNoEnvironments(c, certNames...)

	envs := config.JujuHomePath("environments.yaml")
	err := ioutil.WriteFile(envs, []byte(envConfig), 0644)
	c.Assert(err, IsNil)

	return fake
}

func MakeEmptyFakeHome(c *C) *FakeHome {
	fake := MakeEmptyFakeHomeWithoutJuju(c)
	err := os.Mkdir(config.JujuHome(), 0700)
	c.Assert(err, IsNil)
	return fake
}

func MakeEmptyFakeHomeWithoutJuju(c *C) *FakeHome {
	oldHomeEnv := os.Getenv("HOME")
	oldJujuHomeEnv := os.Getenv("JUJU_HOME")
	oldJujuEnv := os.Getenv("JUJU_ENV")
	fakeHome := c.MkDir()
	os.Setenv("HOME", fakeHome)
	os.Setenv("JUJU_HOME", "")
	os.Setenv("JUJU_ENV", "")
	jujuHome := filepath.Join(fakeHome, ".juju")
	oldJujuHome := config.SetJujuHome(jujuHome)
	return &FakeHome{
		oldHomeEnv:     oldHomeEnv,
		oldJujuEnv:     oldJujuEnv,
		oldJujuHomeEnv: oldJujuHomeEnv,
		oldJujuHome:    oldJujuHome,
		files:          []TestFile{},
	}
}

func HomePath(names ...string) string {
	all := append([]string{os.Getenv("HOME")}, names...)
	return filepath.Join(all...)
}

func (h *FakeHome) Restore() {
	config.SetJujuHome(h.oldJujuHome)
	os.Setenv("JUJU_ENV", h.oldJujuEnv)
	os.Setenv("JUJU_HOME", h.oldJujuHomeEnv)
	os.Setenv("HOME", h.oldHomeEnv)
}

func (h *FakeHome) AddFiles(c *C, files []TestFile) {
	for _, f := range files {
		path := HomePath(f.Name)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(path, []byte(f.Data), 0666)
		c.Assert(err, IsNil)
		h.files = append(h.files, f)
	}
}

// FileContents returns the test file contents for the
// given specified path (which may be relative, so
// we compare with the base filename only).
func (h *FakeHome) FileContents(c *C, path string) string {
	for _, f := range h.files {
		if filepath.Base(f.Name) == filepath.Base(path) {
			return f.Data
		}
	}
	c.Fatalf("path attribute holds unknown test file: %q", path)
	panic("unreachable")
}

// FileExists returns if the given relative file path exists
// in the fake home.
func (h *FakeHome) FileExists(path string) bool {
	for _, f := range h.files {
		if f.Name == path {
			return true
		}
	}
	return false
}

func MakeFakeHomeWithFiles(c *C, files []TestFile) *FakeHome {
	fake := MakeEmptyFakeHome(c)
	fake.AddFiles(c, files)
	return fake
}

func MakeSampleHome(c *C) *FakeHome {
	return MakeFakeHome(c, SingleEnvConfig, SampleCertName)
}

func MakeMultipleEnvHome(c *C) *FakeHome {
	return MakeFakeHome(c, MultipleEnvConfig, SampleCertName, "erewhemos-2")
}

// PatchEnvironment provides a test a simple way to override a single
// environment variable. A function is returned that will return the
// environment to what it was before.
func PatchEnvironment(name, value string) func() {
	oldValue := os.Getenv(name)
	os.Setenv(name, value)
	return func() { os.Setenv(name, oldValue) }
}
