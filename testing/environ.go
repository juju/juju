package testing

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
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

const PeckhamConfig = `
default:
    peckham
environments:
    peckham:
        type: dummy
        state-server: false
        admin-secret: arble
        authorized-keys: i-am-a-key
    walthamstow:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
    brokenenv:
        type: dummy
        broken: Bootstrap Destroy
        state-server: false
        authorized-keys: i-am-a-key
`

type FakeHome string

// MakeFakeHomeNoEnvironments creates a new temporary directory through the
// test checker, and overrides the HOME environment variable to point to this
// new temporary directory.
//
// No ~/.juju/environments.yaml exists, but CAKeys are written for each of the
// 'certNames' specified, and the id_rsa.pub file is written to to the .ssh
// dir.
func MakeFakeHomeNoEnvironments(c *C, certNames ...string) FakeHome {
	fake := MakeEmptyFakeHome(c)

	err := os.Mkdir(HomePath(".juju"), 0755)
	c.Assert(err, IsNil)

	for _, name := range certNames {
		err := ioutil.WriteFile(HomePath(".juju", name+"-cert.pem"), []byte(CACert), 0600)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(HomePath(".juju", name+"-private-key.pem"), []byte(CAKey), 0600)
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
// `config` parameter, and CAKeys are written for each of the 'certNames'
// specified.
func MakeFakeHome(c *C, config string, certNames ...string) FakeHome {
	fake := MakeFakeHomeNoEnvironments(c, certNames...)

	envs := HomePath(".juju", "environments.yaml")
	err := ioutil.WriteFile(envs, []byte(config), 0644)
	c.Assert(err, IsNil)

	return fake
}

func MakeEmptyFakeHome(c *C) FakeHome {
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", c.MkDir())

	return FakeHome(oldHome)
}

func HomePath(names ...string) string {
	all := append([]string{os.Getenv("HOME")}, names...)
	return filepath.Join(all...)
}

func (h FakeHome) Restore() {
	os.Setenv("HOME", string(h))
}

func MakeSampleHome(c *C) FakeHome {
	return MakeFakeHome(c, SingleEnvConfig, SampleCertName)
}

func MakeMultipleEnvHome(c *C) FakeHome {
	return MakeFakeHome(c, MultipleEnvConfig, SampleCertName, "erewhemos-2")
}
