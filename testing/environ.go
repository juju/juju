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

type FakeHome string

func MakeFakeHome(c *C, config string, certNames ...string) FakeHome {
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", c.MkDir())

	err := os.Mkdir(HomePath(".juju"), 0755)
	c.Assert(err, IsNil)

	envs := HomePath(".juju", "environments.yaml")
	err = ioutil.WriteFile(envs, []byte(config), 0644)

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
