// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type jenvSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&jenvSuite{})

var jenvInitErrorsTests = []struct {
	about string
	args  []string
	err   string
}{{
	about: "no args",
	err:   "no jenv file provided",
}, {
	about: "invalid env name",
	args:  []string{"path/to/jenv", "invalid/name"},
	err:   `invalid model name "invalid/name"`,
}, {
	about: "too many args",
	args:  []string{"path/to/jenv", "env-name", "unexpected"},
	err:   `unrecognized args: \["unexpected"\]`,
}}

func (s *jenvSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	dir := gitjujutesting.JujuXDGDataHomePath()
	err := os.MkdirAll(dir, 0600)
	c.Check(err, jc.ErrorIsNil)

}

func (*jenvSuite) TestInitErrors(c *gc.C) {
	for i, test := range jenvInitErrorsTests {
		c.Logf("test %d: %s", i, test.about)

		jenvCmd := &model.JenvCommand{}
		err := testing.InitCommand(jenvCmd, test.args)
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (*jenvSuite) TestJenvFileNotFound(c *gc.C) {
	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, "/no/such/file.jenv")
	c.Assert(err, gc.ErrorMatches, `jenv file "/no/such/file.jenv" not found`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestJenvFileDirectory(c *gc.C) {
	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, c.MkDir())

	// The error is different on some platforms
	c.Assert(err, gc.ErrorMatches, "cannot read the provided jenv file .*: (is a directory|The handle is invalid.)")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestJenvFileNotReadable(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a read-only jenv file.
	f := openJenvFile(c, nil)
	defer f.Close()
	err := f.Chmod(0)
	c.Assert(err, jc.ErrorIsNil)

	// Run the command.
	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, "cannot read the provided jenv file .* permission denied")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

var jenvFileContentErrorsTests = []struct {
	about    string
	contents []byte
	err      string
}{{
	about: "empty",
	err:   "invalid jenv file .*: missing required fields in jenv data: User, Password, ModelUUID, Controllers, CACert",
}, {
	about:    "invalid YAML",
	contents: []byte(":"),
	err:      "invalid jenv file .*: cannot unmarshal jenv data: yaml: .*",
}, {
	about:    "missing field",
	contents: makeJenvContents("myuser", "mypasswd", "model-uuid", "", "1.2.3.4:17070"),
	err:      "invalid jenv file .*: missing required fields in jenv data: CACert",
}}

func (*jenvSuite) TestJenvFileContentErrors(c *gc.C) {
	for i, test := range jenvFileContentErrorsTests {
		c.Logf("test %d: %s", i, test.about)

		// Create the jenv file with the contents provided by the test.
		f := openJenvFile(c, test.contents)
		defer f.Close()

		jenvCmd := &model.JenvCommand{}
		ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
		c.Assert(err, gc.ErrorMatches, test.err)
		c.Assert(testing.Stdout(ctx), gc.Equals, "")
	}
}

func (*jenvSuite) TestConfigStoreError(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a jenv file.
	f := openJenvFile(c, nil)
	defer f.Close()

	// Remove Juju home read permissions.
	home := gitjujutesting.JujuXDGDataHomePath()
	err := os.Chmod(home, 0)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(home, 0700)

	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, "cannot get config store: .*: permission denied")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestWriteError(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Create the environments dir without write permissions.
	envsDir := gitjujutesting.JujuXDGDataHomePath("models")
	err := os.Mkdir(envsDir, 0500)
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, "cannot write the jenv file: cannot write info: .*: permission denied")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSwitchErrorJujuEnvSet(c *gc.C) {
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Override the default Juju environment with the environment variable.
	err := os.Setenv(osenv.JujuModelEnvKey, "ec2")
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `cannot switch to the new model "testing": cannot switch when JUJU_MODEL is overriding the model \(set to "ec2"\)`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSwitchErrorCannotWriteCurrentModel(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Create the current environment file without write permissions.
	currentEnvPath := gitjujutesting.JujuXDGDataHomePath(modelcmd.CurrentModelFilename)
	currentEnvFile, err := os.Create(currentEnvPath)
	c.Assert(err, jc.ErrorIsNil)
	defer currentEnvFile.Close()
	err = currentEnvFile.Chmod(0500)
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `cannot switch to the new model "testing": unable to write to the model file: .*: permission denied`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSuccess(c *gc.C) {
	// Create a jenv file.
	contents := makeJenvContents("who", "Secret!", "model-UUID", testing.CACert, "1.2.3.4:17070")
	f := openJenvFile(c, contents)
	defer f.Close()

	// Import the newly created jenv file.
	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, jc.ErrorIsNil)

	// The jenv file has been properly imported.
	assertJenvContents(c, contents, "testing")

	// The default environment is now the newly imported one, and the output
	// reflects the change.
	currEnv, err := modelcmd.ReadCurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currEnv, gc.Equals, "testing")
	c.Assert(testing.Stdout(ctx), gc.Equals, "-> testing\n")

	// Trying to import the jenv with the same name a second time raises an
	// error.
	jenvCmd = &model.JenvCommand{}
	ctx, err = testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `an model named "testing" already exists: you can provide a second parameter to rename the model`)

	// Overriding the environment name solves the problem.
	jenvCmd = &model.JenvCommand{}
	ctx, err = testing.RunCommand(c, jenvCmd, f.Name(), "another")
	c.Assert(err, jc.ErrorIsNil)
	assertJenvContents(c, contents, "another")

	currEnv, err = modelcmd.ReadCurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currEnv, gc.Equals, "another")
	c.Assert(testing.Stdout(ctx), gc.Equals, "testing -> another\n")
}

func (*jenvSuite) TestSuccessCustomEnvironmentName(c *gc.C) {
	// Create a jenv file.
	contents := makeValidJenvContents()
	f := openJenvFile(c, contents)
	defer f.Close()

	// Import the newly created jenv file with a customized name.
	jenvCmd := &model.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name(), "my-env")
	c.Assert(err, jc.ErrorIsNil)

	// The jenv file has been properly imported.
	assertJenvContents(c, contents, "my-env")

	// The default environment is now the newly imported one, and the output
	// reflects the change.
	currEnv, err := modelcmd.ReadCurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currEnv, gc.Equals, "my-env")
	c.Assert(testing.Stdout(ctx), gc.Equals, "-> my-env\n")
}

// openJenvFile creates and returns a jenv file with the given contents.
// Callers are responsible of closing the file.
func openJenvFile(c *gc.C, contents []byte) *os.File {
	path := filepath.Join(c.MkDir(), "testing.jenv")
	f, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	_, err = f.Write(contents)
	c.Assert(err, jc.ErrorIsNil)
	return f
}

// makeJenvContents returns a YAML encoded environment info data.
func makeJenvContents(user, password, modelUUID, caCert string, controllers ...string) []byte {
	b, err := yaml.Marshal(configstore.EnvironInfoData{
		User:        user,
		Password:    password,
		ModelUUID:   modelUUID,
		CACert:      caCert,
		Controllers: controllers,
	})
	if err != nil {
		panic(err)
	}
	return b
}

// makeValidJenvContents returns valid jenv file YAML encoded contents.
func makeValidJenvContents() []byte {
	return makeJenvContents(
		"myuser", "mypasswd", "model-uuid", testing.CACert,
		"1.2.3.4:17070", "example.com:17070")
}

// assertJenvContents checks that the jenv file corresponding to the given
// envName is correctly present in the Juju Home and has the given contents.
func assertJenvContents(c *gc.C, contents []byte, envName string) {
	path := gitjujutesting.JujuXDGDataHomePath("models", envName+".jenv")
	// Ensure the jenv file has been created.
	c.Assert(path, jc.IsNonEmptyFile)

	// Retrieve the jenv file contents.
	b, err := ioutil.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the jenv file reflects the source contents.
	var data map[string]interface{}
	err = yaml.Unmarshal(contents, &data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), jc.YAMLEquals, data)
}
