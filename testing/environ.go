// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

// FakeAuthKeys holds the authorized key used for testing
// purposes in FakeConfig. It is valid for parsing with the utils/ssh
// authorized-key utilities.
const FakeAuthKeys = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAYQDP8fPSAMFm2PQGoVUks/FENVUMww1QTK6m++Y2qX9NGHm43kwEzxfoWR77wo6fhBhgFHsQ6ogE/cYLx77hOvjTchMEP74EVxSce0qtDjI7SwYbOpAButRId3g/Ef4STz8= joe@0.1.2.4`

func init() {
	_, err := ssh.ParseAuthorisedKey(FakeAuthKeys)
	if err != nil {
		panic("FakeAuthKeys does not hold a valid authorized key: " + err.Error())
	}
}

// FakeConfig() returns an environment configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "someprovider",
		"name":                      "testenv",
		"authorized-keys":           FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"admin-secret":              "fish",
		"ca-cert":                   CACert,
		"ca-private-key":            CAKey,
		"ssl-hostname-verification": true,
		"development":               false,
		"state-port":                19034,
		"api-port":                  17777,
		"default-series":            config.DefaultSeries,
	}
}

// EnvironConfig returns a default environment configuration suitable for
// setting in the state.
func EnvironConfig(c *gc.C) *config.Config {
	return CustomEnvironConfig(c, Attrs{})
}

// CustomEnvironConfig returns an environment configuration with
// additional specified keys added.
func CustomEnvironConfig(c *gc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "1.2.3",
	}).Merge(extra).Delete("admin-secret", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

const (
	SampleEnvName = "erewhemos"
	EnvDefault    = "default:\n  " + SampleEnvName + "\n"
)

const DefaultMongoPassword = "conn-from-name-secret"

// Environment names below are explicit as it makes them more readable.
const SingleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
`

const SingleEnvConfig = EnvDefault + SingleEnvConfigNoDefault

const MultipleEnvConfigNoDefault = `
environments:
    erewhemos:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
    erewhemos-2:
        type: dummy
        state-server: true
        authorized-keys: i-am-a-key
        admin-secret: ` + DefaultMongoPassword + `
`

const MultipleEnvConfig = EnvDefault + MultipleEnvConfigNoDefault

const SampleCertName = "erewhemos"

type TestFile struct {
	Name, Data string
}

type FakeHome struct {
	oldHomeEnv     string
	oldEnvironment map[string]string
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
func MakeFakeHomeNoEnvironments(c *gc.C, certNames ...string) *FakeHome {
	fake := MakeEmptyFakeHome(c)

	for _, name := range certNames {
		err := ioutil.WriteFile(osenv.JujuHomePath(name+"-cert.pem"), []byte(CACert), 0600)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(osenv.JujuHomePath(name+"-private-key.pem"), []byte(CAKey), 0600)
		c.Assert(err, gc.IsNil)
	}

	err := os.Mkdir(HomePath(".ssh"), 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(HomePath(".ssh", "id_rsa.pub"), []byte("auth key\n"), 0666)
	c.Assert(err, gc.IsNil)

	return fake
}

// MakeFakeHome creates a new temporary directory through the test checker,
// and overrides the HOME environment variable to point to this new temporary
// directory.
//
// A new ~/.juju/environments.yaml file is created with the content of the
// `envConfig` parameter, and CAKeys are written for each of the 'certNames'
// specified.
func MakeFakeHome(c *gc.C, envConfig string, certNames ...string) *FakeHome {
	fake := MakeFakeHomeNoEnvironments(c, certNames...)

	envs := osenv.JujuHomePath("environments.yaml")
	err := ioutil.WriteFile(envs, []byte(envConfig), 0644)
	c.Assert(err, gc.IsNil)

	return fake
}

func MakeEmptyFakeHome(c *gc.C) *FakeHome {
	fake := MakeEmptyFakeHomeWithoutJuju(c)
	err := os.Mkdir(osenv.JujuHome(), 0700)
	c.Assert(err, gc.IsNil)
	return fake
}

func MakeEmptyFakeHomeWithoutJuju(c *gc.C) *FakeHome {
	oldHomeEnv := osenv.Home()
	oldEnvironment := make(map[string]string)
	for _, name := range []string{
		osenv.JujuHomeEnvKey,
		osenv.JujuEnvEnvKey,
		osenv.JujuLoggingConfigEnvKey,
	} {
		oldEnvironment[name] = os.Getenv(name)
	}
	fakeHome := c.MkDir()
	osenv.SetHome(fakeHome)
	os.Setenv(osenv.JujuHomeEnvKey, "")
	os.Setenv(osenv.JujuEnvEnvKey, "")
	os.Setenv(osenv.JujuLoggingConfigEnvKey, "")
	jujuHome := filepath.Join(fakeHome, ".juju")
	oldJujuHome := osenv.SetJujuHome(jujuHome)
	return &FakeHome{
		oldHomeEnv:     oldHomeEnv,
		oldEnvironment: oldEnvironment,
		oldJujuHome:    oldJujuHome,
		files:          []TestFile{},
	}
}

func HomePath(names ...string) string {
	all := append([]string{osenv.Home()}, names...)
	return filepath.Join(all...)
}

func (h *FakeHome) Restore() {
	osenv.SetJujuHome(h.oldJujuHome)
	for name, value := range h.oldEnvironment {
		os.Setenv(name, value)
	}
	osenv.SetHome(h.oldHomeEnv)
}

func (h *FakeHome) AddFiles(c *gc.C, files []TestFile) {
	for _, f := range files {
		path := HomePath(f.Name)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(path, []byte(f.Data), 0666)
		c.Assert(err, gc.IsNil)
		h.files = append(h.files, f)
	}
}

// FileContents returns the test file contents for the
// given specified path (which may be relative, so
// we compare with the base filename only).
func (h *FakeHome) FileContents(c *gc.C, path string) string {
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

func MakeFakeHomeWithFiles(c *gc.C, files []TestFile) *FakeHome {
	fake := MakeEmptyFakeHome(c)
	fake.AddFiles(c, files)
	return fake
}

func MakeSampleHome(c *gc.C) *FakeHome {
	return MakeFakeHome(c, SingleEnvConfig, SampleCertName)
}

func MakeMultipleEnvHome(c *gc.C) *FakeHome {
	return MakeFakeHome(c, MultipleEnvConfig, SampleCertName, SampleCertName+"-2")
}

type FakeHomeSuite struct {
	testbase.LoggingSuite
	Home *FakeHome
}

func (s *FakeHomeSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.Home = MakeSampleHome(c)
	s.AddCleanup(func(*gc.C) { s.Home.Restore() })
}
