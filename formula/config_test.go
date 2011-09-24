package formula_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"path/filepath"
)

var sampleConfig = `
options:
  title:
    default: My Title
    description: A descriptive title used for the service.
    type: string
  outlook:
    description: No default outlook.
    type: string
  username:
    default: admin001
    description: The name of the initial account (given admin permissions).
    type: string
  skill-level:
    description: A number indicating skill.
    type: int
`

func repoConfig(name string) (path string) {
	return filepath.Join("testrepo", name, "config.yaml")
}

func assertDummyConfig(c *C, config *formula.Config) {
	c.Assert(config.Options["title"], Equals,
		formula.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s *S) TestReadConfig(c *C) {
	config, err := formula.ReadConfig(repoConfig("dummy"))
	c.Assert(err, IsNil)
	assertDummyConfig(c, config)
}

func (s *S) TestParseConfig(c *C) {
	data, err := ioutil.ReadFile(repoConfig("dummy"))
	c.Assert(err, IsNil)

	config, err := formula.ParseConfig(data)
	c.Assert(err, IsNil)
	assertDummyConfig(c, config)
}

func (s *S) TestConfigErrorWithPath(c *C) {
	path := filepath.Join(c.MkDir(), "mymeta.yaml")

	_, err := formula.ReadConfig(path)
	c.Assert(err, Matches, `.*/.*/mymeta\.yaml.*no such file.*`)

	data := `options: {t: {type: foo}}`
	err = ioutil.WriteFile(path, []byte(data), 0644)
	c.Assert(err, IsNil)

	_, err = formula.ReadConfig(path)
	c.Assert(err, Matches, `/.*/mymeta\.yaml: options.t.type: unsupported value`)
}

func (s *S) TestParseSample(c *C) {
	config, err := formula.ParseConfig([]byte(sampleConfig))
	c.Assert(err, IsNil)

	opt := config.Options
	c.Assert(opt["title"], Equals,
		formula.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
	c.Assert(opt["outlook"], Equals,
		formula.Option{
			Description: "No default outlook.",
			Type:        "string",
		},
	)
	c.Assert(opt["username"], Equals,
		formula.Option{
			Default:     "admin001",
			Description: "The name of the initial account (given admin permissions).",
			Type:        "string",
		},
	)
	c.Assert(opt["skill-level"], Equals,
		formula.Option{
			Description: "A number indicating skill.",
			Type:        "int",
		},
	)
}
