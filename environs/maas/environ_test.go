package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs/config"
)

type EnvironTest struct{}

var _ = Suite(new(EnvironTest))

func getTestConfig(name, server, oauth, secret string) *config.Config {
	ecfg, err := newConfig(map[string]interface{}{
		"name":         name,
		"maas-server":  server,
		"maas-oauth":   oauth,
		"admin-secret": secret,
	})
	if err != nil {
		panic(err)
	}
	return ecfg.Config
}

func (EnvironTest) TestSetConfigUpdatesConfig(c *C) {
	cfg := getTestConfig("test env", "maas2.example.com", "a:b:c", "secret")
	env, err := NewEnviron(cfg)
	c.Check(err, IsNil)
	c.Check(env.name, Equals, "test env")

	anotherName := "another name"
	anotherServer := "maas.example.com"
	anotherOauth := "c:d:e"
	anotherSecret := "secret2"
	cfg2 := getTestConfig(anotherName, anotherServer, anotherOauth, anotherSecret)
	errSetConfig := env.SetConfig(cfg2)
	c.Check(errSetConfig, IsNil)
	c.Check(env.name, Equals, anotherName)
	authClient, _ := gomaasapi.NewAuthenticatedClient(anotherServer, anotherOauth)
	maas := gomaasapi.NewMAAS(*authClient)
	MAASServer := env.maasClientUnlocked
	c.Check(MAASServer, DeepEquals, maas)
}

func (EnvironTest) TestNewEnvironSetsConfig(c *C) {
	name := "test env"
	cfg := getTestConfig(name, "maas.example.com", "a:b:c", "secret")

	env, err := NewEnviron(cfg)

	c.Check(err, IsNil)
	c.Check(env.name, Equals, name)
}
