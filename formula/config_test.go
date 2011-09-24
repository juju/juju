package formula_test

import (
	"bytes"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"os"
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
  agility-ratio:
    description: A number from 0 to 1 indicating agility.
    type: float
`

func repoConfig(name string) io.Reader {
	file, err := os.Open(filepath.Join("testrepo", name, "config.yaml"))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(data)
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

func (s *S) TestConfigError(c *C) {
	_, err := formula.ReadConfig(bytes.NewBuffer([]byte(`options: {t: {type: foo}}`)))
	c.Assert(err, Matches, `config: options.t.type: unsupported value`)
}

func (s *S) TestParseSample(c *C) {
	config, err := formula.ReadConfig(bytes.NewBuffer([]byte(sampleConfig)))
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

func (s *S) TestValidate(c *C) {
	config, err := formula.ReadConfig(bytes.NewBuffer([]byte(sampleConfig)))
	c.Assert(err, IsNil)

	input := map[string]string{
		"title":   "Helpful Title",
		"outlook": "Peachy",
	}

	// This should include an overridden value, a default and a new value.
	expected := map[string]interface{}{
		"title":    "Helpful Title",
		"outlook":  "Peachy",
		"username": "admin001",
	}

	output, err := config.Validate(input)
	c.Assert(err, IsNil)
	c.Assert(output, Equals, expected)

	// Check whether float conversion is working.
	input["agility-ratio"] = "0.5"
	input["skill-level"] = "7"
	expected["agility-ratio"] = 0.5
	expected["skill-level"] = int64(7)
	output, err = config.Validate(input)
	c.Assert(err, IsNil)
	c.Assert(output, Equals, expected)

	// Check whether float errors are caught.
	input["agility-ratio"] = "foo"
	output, err = config.Validate(input)
	c.Assert(err, Matches, `Value for "agility-ratio" is not a float: "foo"`)
	input["agility-ratio"] = "0.5"

	// Check whether int errors are caught.
	input["skill-level"] = "foo"
	output, err = config.Validate(input)
	c.Assert(err, Matches, `Value for "skill-level" is not an int: "foo"`)
	input["skill-level"] = "7"

	// Now try to set a value outside the expected.
	input["bad"] = "value"
	output, err = config.Validate(input)
	c.Assert(output, IsNil)
	c.Assert(err, Matches, `Unknown configuration option: "bad"`)
}
