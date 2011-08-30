package formula_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"path/filepath"
)

func repoConfig(name string) (path string) {
	return filepath.Join("testrepo", name, "config.yaml")
}

func (s *S) TestReadConfig(c *C) {
	config, err := formula.ReadConfig(repoConfig("dummy"))
	c.Assert(err, IsNil)
	c.Assert(config.Options["title"], Equals,
		formula.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}
