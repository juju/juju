// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type jenvSuite struct {
	testing.FakeJujuHomeSuite
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
	err:   `invalid environment name "invalid/name"`,
}, {
	about: "too many args",
	args:  []string{"path/to/jenv", "env-name", "unexpected"},
	err:   `unrecognized args: \["unexpected"\]`,
}}

func (*jenvSuite) TestInitErrors(c *gc.C) {
	for i, test := range jenvInitErrorsTests {
		c.Logf("test %d: %s", i, test.about)

		jenvCmd := &environment.JenvCommand{}
		err := testing.InitCommand(jenvCmd, test.args)
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (*jenvSuite) TestJenvFileNotFound(c *gc.C) {
	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, "/no/such/file.jenv")
	c.Assert(err, gc.ErrorMatches, `jenv file "/no/such/file.jenv" not found`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestJenvFileDirectory(c *gc.C) {
	jenvCmd := &environment.JenvCommand{}
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
	jenvCmd := &environment.JenvCommand{}
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
	err:   "invalid jenv file .*: missing required fields in jenv data: User, Password, EnvironUUID, StateServers, CACert",
}, {
	about:    "invalid YAML",
	contents: []byte(":"),
	err:      "invalid jenv file .*: cannot unmarshal jenv data: YAML error: .*",
}, {
	about:    "missing field",
	contents: makeJenvContents("myuser", "mypasswd", "env-uuid", "", "1.2.3.4:17070"),
	err:      "invalid jenv file .*: missing required fields in jenv data: CACert",
}}

func (*jenvSuite) TestJenvFileContentErrors(c *gc.C) {
	for i, test := range jenvFileContentErrorsTests {
		c.Logf("test %d: %s", i, test.about)

		// Create the jenv file with the contents provided by the test.
		f := openJenvFile(c, test.contents)
		defer f.Close()

		jenvCmd := &environment.JenvCommand{}
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
	home := gitjujutesting.HomePath(".juju")
	err := os.Chmod(home, 0)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(home, 0700)

	jenvCmd := &environment.JenvCommand{}
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
	envsDir := gitjujutesting.HomePath(".juju", "environments")
	err := os.Mkdir(envsDir, 0500)
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, "cannot write the jenv file: cannot write info: .*: permission denied")
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSwitchErrorJujuEnvSet(c *gc.C) {
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Override the default Juju environment with the environment variable.
	err := os.Setenv(osenv.JujuEnvEnvKey, "ec2")
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `cannot switch to the new environment "testing": cannot switch when JUJU_ENV is overriding the environment \(set to "ec2"\)`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSwitchErrorEnvironmentsNotReadable(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Remove write permissions to the environments.yaml file.
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Chmod(envPath, 0200)
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `cannot switch to the new environment "testing": cannot get the default environment: .*: permission denied`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSwitchErrorCannotWriteCurrentEnvironment(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot test on windows because it uses chmod")
	}
	// Create a jenv file.
	f := openJenvFile(c, makeValidJenvContents())
	defer f.Close()

	// Create the current environment file without write permissions.
	currentEnvPath := gitjujutesting.HomePath(".juju", envcmd.CurrentEnvironmentFilename)
	currentEnvFile, err := os.Create(currentEnvPath)
	c.Assert(err, jc.ErrorIsNil)
	defer currentEnvFile.Close()
	err = currentEnvFile.Chmod(0500)
	c.Assert(err, jc.ErrorIsNil)

	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `cannot switch to the new environment "testing": unable to write to the environment file: .*: permission denied`)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
}

func (*jenvSuite) TestSuccess(c *gc.C) {
	// Create a jenv file.
	contents := makeJenvContents("who", "Secret!", "env-UUID", testing.CACert, "1.2.3.4:17070")
	f := openJenvFile(c, contents)
	defer f.Close()

	// Import the newly created jenv file.
	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, jc.ErrorIsNil)

	// The jenv file has been properly imported.
	assertJenvContents(c, contents, "testing")

	// The default environment is now the newly imported one, and the output
	// reflects the change.
	currEnv, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currEnv, gc.Equals, "testing")
	c.Assert(testing.Stdout(ctx), gc.Equals, "erewhemos -> testing\n")

	// Trying to import the jenv with the same name a second time raises an
	// error.
	jenvCmd = &environment.JenvCommand{}
	ctx, err = testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, gc.ErrorMatches, `an environment named "testing" already exists: you can provide a second parameter to rename the environment`)

	// Overriding the environment name solves the problem.
	jenvCmd = &environment.JenvCommand{}
	ctx, err = testing.RunCommand(c, jenvCmd, f.Name(), "another")
	c.Assert(err, jc.ErrorIsNil)
	assertJenvContents(c, contents, "another")

	currEnv, err = envcmd.ReadCurrentEnvironment()
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
	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name(), "my-env")
	c.Assert(err, jc.ErrorIsNil)

	// The jenv file has been properly imported.
	assertJenvContents(c, contents, "my-env")

	// The default environment is now the newly imported one, and the output
	// reflects the change.
	currEnv, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currEnv, gc.Equals, "my-env")
	c.Assert(testing.Stdout(ctx), gc.Equals, "erewhemos -> my-env\n")
}

func (*jenvSuite) TestSuccessNoJujuEnvironments(c *gc.C) {
	// Create a jenv file.
	contents := makeValidJenvContents()
	f := openJenvFile(c, contents)
	defer f.Close()

	// Remove the Juju environments.yaml file.
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)

	// Importing a jenv does not require having environments already defined.
	jenvCmd := &environment.JenvCommand{}
	ctx, err := testing.RunCommand(c, jenvCmd, f.Name())
	c.Assert(err, jc.ErrorIsNil)
	assertJenvContents(c, contents, "testing")
	c.Assert(testing.Stdout(ctx), gc.Equals, "-> testing\n")
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
func makeJenvContents(user, password, environUUID, caCert string, stateServers ...string) []byte {
	b, err := yaml.Marshal(configstore.EnvironInfoData{
		User:         user,
		Password:     password,
		EnvironUUID:  environUUID,
		CACert:       caCert,
		StateServers: stateServers,
	})
	if err != nil {
		panic(err)
	}
	return b
}

// makeValidJenvContents returns valid jenv file YAML encoded contents.
func makeValidJenvContents() []byte {
	return makeJenvContents(
		"myuser", "mypasswd", "env-uuid", testing.CACert,
		"1.2.3.4:17070", "example.com:17070")
}

// assertJenvContents checks that the jenv file corresponding to the given
// envName is correctly present in the Juju Home and has the given contents.
func assertJenvContents(c *gc.C, contents []byte, envName string) {
	path := gitjujutesting.HomePath(".juju", "environments", envName+".jenv")
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
