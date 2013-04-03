package testing

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/config"
	"os"
	"path/filepath"
)

const SampleEnvName = "erewhemos"
const EnvDefault = "default:\n  " + SampleEnvName + "\n"

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

type FakeHome struct {
	oldHome     string
	oldJujuHome string
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
	err := os.Mkdir(config.JujuHome(), 0755)
	c.Assert(err, IsNil)

	for _, name := range certNames {
		err := ioutil.WriteFile(config.JujuHomePath(name+"-cert.pem"), []byte(CACert), 0600)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(config.JujuHomePath(name+"-private-key.pem"), []byte(CAKey), 0600)
		c.Assert(err, IsNil)
	}

	err = os.Mkdir(HomePath(".ssh"), 0777)
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
	oldHome := os.Getenv("HOME")
	fakeHome := c.MkDir()
	os.Setenv("HOME", fakeHome)
	oldJujuHome := config.SetJujuHome(filepath.Join(fakeHome, ".juju"))
	return &FakeHome{oldHome, oldJujuHome}
}

func HomePath(names ...string) string {
	all := append([]string{os.Getenv("HOME")}, names...)
	return filepath.Join(all...)
}

func (h *FakeHome) Restore() {
	config.SetJujuHome(h.oldJujuHome)
	os.Setenv("HOME", h.oldHome)
}

func MakeSampleHome(c *C) *FakeHome {
	return MakeFakeHome(c, SingleEnvConfig, SampleCertName)
}

func MakeMultipleEnvHome(c *C) *FakeHome {
	return MakeFakeHome(c, MultipleEnvConfig, SampleCertName, "erewhemos-2")
}

// BreakJuju forces the dummy environment to return an error when
// environMethod is called. It allows you to customize the environ
// name and whether state server is available.
// It returns the exact message to expect, as well as the environ
// config, in case you need to call SetEnvironConfig() with it.
func BreakJuju(c *C, envName, environMethod string, withStateServer bool) (string, *config.Config) {
	brokenConfig := map[string]interface{}{
		"environments": map[string]interface{}{
			envName: map[string]interface{}{
				"type":            "dummy",
				"state-server":    withStateServer,
				"authorized-keys": "i-am-a-key",
				"broken":          environMethod,
			},
		},
	}
	data, err := goyaml.Marshal(brokenConfig)
	err = ioutil.WriteFile(config.JujuHomePath("environments.yaml"), data, 0666)
	c.Assert(err, IsNil)

	// Now get the config only and return an environ config from it.
	ecfg := brokenConfig["environments"].(map[string]interface{})[envName].(map[string]interface{})
	ecfg["name"] = envName
	cfg, err := config.New(ecfg)
	c.Assert(err, IsNil)

	return fmt.Sprintf("dummy.%s is broken", environMethod), cfg
}
