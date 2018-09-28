// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	cloudfile "github.com/juju/juju/cloud"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
)

type addSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&addSuite{})

func newFakeCloudMetadataStore() *fakeCloudMetadataStore {
	var logger loggo.Logger
	return &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}
}

type fakeCloudMetadataStore struct {
	*jujutesting.CallMocker
}

func (f *fakeCloudMetadataStore) ParseCloudMetadataFile(path string) (map[string]cloudfile.Cloud, error) {
	results := f.MethodCall(f, "ParseCloudMetadataFile", path)
	return results[0].(map[string]cloudfile.Cloud), jujutesting.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) PublicCloudMetadata(searchPaths ...string) (result map[string]cloudfile.Cloud, fallbackUsed bool, _ error) {
	results := f.MethodCall(f, "PublicCloudMetadata", searchPaths)
	return results[0].(map[string]cloudfile.Cloud), results[1].(bool), jujutesting.TypeAssertError(results[2])
}

func (f *fakeCloudMetadataStore) PersonalCloudMetadata() (map[string]cloudfile.Cloud, error) {
	results := f.MethodCall(f, "PersonalCloudMetadata")
	return results[0].(map[string]cloudfile.Cloud), jujutesting.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) WritePersonalCloudMetadata(cloudsMap map[string]cloudfile.Cloud) error {
	results := f.MethodCall(f, "WritePersonalCloudMetadata", cloudsMap)
	return jujutesting.TypeAssertError(results[0])
}

func (f *fakeCloudMetadataStore) ParseOneCloud(data []byte) (cloudfile.Cloud, error) {
	results := f.MethodCall(f, "ParseOneCloud", data)
	if len(results) != 2 {
		fmt.Printf("ParseOneCloud()\n(%s)\n", string(data))
		return cloudfile.Cloud{}, errors.New("ParseOneCloud failed, not enough results")
	}
	return results[0].(cloudfile.Cloud), jujutesting.TypeAssertError(results[1])
}

func (s *addSuite) TestAddBadArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(nil), "cloud", "cloud.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

var (
	homeStackYamlFile = `
        clouds:
          homestack:
            type: openstack
            auth-types: [access-key]
            endpoint: "http://homestack"
            regions:
              london:
                endpoint: "http://london/1.0"`

	homestackCloud = cloudfile.Cloud{
		Name:      "homestack",
		Type:      "openstack",
		AuthTypes: []cloudfile.AuthType{"userpass", "access-key"},
		Endpoint:  "http://homestack",
		Regions: []cloudfile.Region{
			{
				Name:     "london",
				Endpoint: "http://london/1.0",
			},
		},
	}

	localhostYamlFile = `
        clouds:
          localhost:
            type: lxd`

	awsYamlFile = `
        clouds:
          aws:
            type: ec2
            auth-types: [access-key]
            regions:
              us-east-1:
                endpoint: "https://us-east-1.aws.amazon.com/v1.2/"`

	garageMaasYamlFile = `
        clouds:
          garage-maas:
            type: maas
            auth-types: [oauth1]
            endpoint: "http://garagemaas"`

	garageMAASCloud = cloudfile.Cloud{
		Name:      "garage-maas",
		Type:      "maas",
		AuthTypes: []cloudfile.AuthType{"oauth1"},
		Endpoint:  "http://garagemaas",
	}

	manualCloud = cloudfile.Cloud{
		Name:      "manual",
		Type:      "manual",
		AuthTypes: []cloudfile.AuthType{"manual"},
		Endpoint:  "192.168.1.6",
	}
)

func homestackMetadata() map[string]cloudfile.Cloud {
	return map[string]cloudfile.Cloud{"homestack": homestackCloud}
}

func (*addSuite) TestAddBadFilename(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	badFileErr := errors.New("")
	fake.Call("ParseCloudMetadataFile", "somefile.yaml").Returns(map[string]cloudfile.Cloud{}, badFileErr)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "somefile.yaml")
	c.Check(err, gc.Equals, badFileErr)
}

func (*addSuite) TestAddBadCloudName(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "testFile").Returns(map[string]cloudfile.Cloud{}, nil)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "testFile")
	c.Assert(err, gc.ErrorMatches, `cloud "cloud" not found in file .*`)
}

func (*addSuite) TestAddInvalidCloudName(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "testFile").Returns(map[string]cloudfile.Cloud{}, nil)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := cmdtesting.RunCommand(c, addCmd, "bad^cloud", "testFile")
	c.Assert(err, gc.ErrorMatches, `cloud name "bad\^cloud" not valid`)
}

func (*addSuite) TestAddExisting(c *gc.C) {
	fake := newFakeCloudMetadataStore()

	cloudFile := prepareTestCloudYaml(c, homeStackYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(mockCloud, nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "homestack", cloudFile.Name())
	c.Assert(err, gc.ErrorMatches, `"homestack" already exists; use --replace to replace this existing cloud`)
}

func (*addSuite) TestAddExistingReplace(c *gc.C) {
	fake := newFakeCloudMetadataStore()

	cloudFile := prepareTestCloudYaml(c, homeStackYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PersonalCloudMetadata").Returns(mockCloud, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "homestack", cloudFile.Name(), "--replace")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestAddExistingPublic(c *gc.C) {
	cloudFile := prepareTestCloudYaml(c, awsYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(mockCloud, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "aws", cloudFile.Name())
	c.Assert(err, gc.ErrorMatches, `"aws" is the name of a public cloud; use --replace to override this definition`)
}

func (*addSuite) TestAddExistingBuiltin(c *gc.C) {
	cloudFile := prepareTestCloudYaml(c, localhostYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "localhost", cloudFile.Name())
	c.Assert(err, gc.ErrorMatches, `"localhost" is the name of a built-in cloud; use --replace to override this definition`)
}

func (*addSuite) TestAddExistingPublicReplace(c *gc.C) {
	cloudFile := prepareTestCloudYaml(c, awsYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(mockCloud, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	writeCall := fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "aws", cloudFile.Name(), "--replace")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(writeCall(), gc.Equals, 1)
}

func (*addSuite) TestAddNew(c *gc.C) {
	cloudFile := prepareTestCloudYaml(c, garageMaasYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "garage-maas", cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestAddNewInvalidAuthType(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fakeCloudYamlFile := `
        clouds:
          fakecloud:
            type: maas
            auth-types: [oauth1, user-pass]
            endpoint: "http://garagemaas"`

	cloudFile := prepareTestCloudYaml(c, fakeCloudYamlFile)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)

	_, err = cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(fake), "fakecloud", cloudFile.Name())
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`auth type "user-pass" not supported`))
}

func (*addSuite) TestInteractive(c *gc.C) {
	command := cloud.NewAddCloudCommand(nil)
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	out := &bytes.Buffer{}

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: ioutil.Discard,
		Stdin:  &bytes.Buffer{},
	}
	err = command.Run(ctx)
	c.Check(errors.Cause(err), gc.Equals, io.EOF)

	c.Assert(out.String(), gc.Equals, ""+
		"Cloud Types\n"+
		"  lxd\n"+
		"  maas\n"+
		"  manual\n"+
		"  oci\n"+
		"  openstack\n"+
		"  oracle\n"+
		"  vsphere\n"+
		"\n"+
		"Select cloud type: \n",
	)
}

func (*addSuite) TestInteractiveMaas(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- oauth1\n" +
		"endpoint: http://mymaas\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(garageMAASCloud, nil)
	m1Cloud := garageMAASCloud
	m1Cloud.Name = "m1"
	m1Metadata := map[string]cloudfile.Cloud{"m1": m1Cloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", m1Metadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "maas\n" +
			/* Enter a name for the cloud: */ "m1\n" +
			/* Enter the controller's hostname or IP address: */ "http://mymaas\n",
		),
	}

	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractiveManual(c *gc.C) {
	manCloud := manualCloud
	manCloud.Name = "man"
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manCloud, nil)
	manMetadata := map[string]cloudfile.Cloud{"man": manCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", manMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "man\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractiveVSphere(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	vsphereCloud := cloudfile.Cloud{
		Name:      "mvs",
		Type:      "vsphere",
		AuthTypes: []cloudfile.AuthType{"userpass", "access-key"},
		Endpoint:  "192.168.1.6",
		Regions: []cloudfile.Region{
			{
				Name:     "foo",
				Endpoint: "192.168.1.6",
			},
			{
				Name:     "bar",
				Endpoint: "192.168.1.6",
			},
		},
	}
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- userpass\n" +
		"endpoint: 192.168.1.6\n" +
		"regions:\n" +
		"  bar: {}\n" +
		"  foo: {}\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(vsphereCloud, nil)
	vsphereMetadata := map[string]cloudfile.Cloud{"mvs": vsphereCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", vsphereMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	var stdout bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &stdout,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "vsphere\n" +
			/* Enter a name for the cloud: */ "mvs\n" +
			/* Enter the vCenter address or URL: */ "192.168.1.6\n" +
			/* Enter datacenter name: */ "foo\n" +
			/* Enter another datacenter? (y/N): */ "y\n" +
			/* Enter datacenter name: */ "bar\n" +
			/* Enter another datacenter? (y/N): */ "n\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
	c.Check(stdout.String(), gc.Matches, "(.|\n)*"+`
Select cloud type: 
Enter a name for your vsphere cloud: 
Enter the vCenter address or URL: 
Enter datacenter name: 
Enter another datacenter\? \(y/N\): 
Enter datacenter name: 
Enter another datacenter\? \(y/N\): 
`[1:]+"(.|\n)*")
}

func (*addSuite) TestInteractiveExistingNameOverride(c *gc.C) {
	manualCloud := manualCloud
	manualCloud.Name = "homestack"

	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	manMetadata := map[string]cloudfile.Cloud{"homestack": manualCloud}
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manualCloud, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", manMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "homestack\n" +
			/* Do you want to replace that definition? */ "y\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractiveExistingNameNoOverride(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	homestack2Cloud := cloudfile.Cloud{
		Name:     "homestack2",
		Type:     "manual",
		Endpoint: "192.168.1.6",
	}
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(homestack2Cloud, nil)
	compoundCloudMetadata := map[string]cloudfile.Cloud{
		"homestack":  homestackCloud,
		"homestack2": homestack2Cloud,
	}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", compoundCloudMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	var out bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &out,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "homestack" + "\n" +
			/* Do you want to replace that definition? (y/N): */ "n\n" +
			/* Enter a name for the cloud: */ "homestack2" + "\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6" + "\n",
		),
	}

	err = command.Run(ctx)
	c.Log(out.String())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractiveAddCloud_PromptForNameIsCorrect(c *gc.C) {
	var out bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &out,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n",
		),
	}

	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)

	command := cloud.NewAddCloudCommand(fake)
	// Running the command will return an error because we only give
	// enough input to get to the prompt we care about checking. This
	// test ignores this error.
	err := command.Run(ctx)
	c.Assert(errors.Cause(err), gc.Equals, io.EOF)

	c.Check(out.String(), gc.Matches, "(?s).+Enter a name for your manual cloud: .*")
}

func (*addSuite) TestSpecifyingCloudFileThroughFlag_CorrectlySetsMemberVar(c *gc.C) {
	command := cloud.NewAddCloudCommand(nil)
	runCmd := func() {
		cmdtesting.RunCommand(c, command, "garage-maas", "-f", "fake.yaml")
	}
	c.Assert(runCmd, gc.PanicMatches, "runtime error: invalid memory address or nil pointer dereference")
	c.Check(command.CloudFile, gc.Equals, "fake.yaml")
}

func (*addSuite) TestSpecifyingCloudFileThroughFlagAndArgument_Errors(c *gc.C) {
	command := cloud.NewAddCloudCommand(nil)
	_, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", "fake.yaml", "foo.yaml")
	c.Check(err, gc.ErrorMatches, "cannot specify cloud file with flag and argument")
}

func (*addSuite) TestValidateGoodCloudFile(c *gc.C) {
	data := `
clouds:
  foundations:
    type: maas
    auth-types: [oauth1]
    endpoint: "http://10.245.31.100/MAAS"`

	cloudFile := prepareTestCloudYaml(c, data)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	var logWriter loggo.TestWriter
	writerName := "add_cloud_tests_writer"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)

	_, err = cmdtesting.RunCommand(c, command, "foundations", cloudFile.Name())
	c.Check(err, jc.ErrorIsNil)

	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{})
}

func (*addSuite) TestValidateBadCloudFile(c *gc.C) {
	data := `
clouds:
  foundations:
    type: maas
    auth-typs: [oauth1]
    endpoint: "http://10.245.31.100/MAAS"`

	cloudFile := prepareTestCloudYaml(c, data)
	defer cloudFile.Close()
	defer os.Remove(cloudFile.Name())

	mockCloud, err := cloudfile.ParseCloudMetadataFile(cloudFile.Name())
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudFile.Name()).Returns(mockCloud, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)

	var logWriter loggo.TestWriter
	writerName := "add_cloud_tests_writer"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	_, err = cmdtesting.RunCommand(c, command, "foundations", cloudFile.Name())
	c.Check(err, jc.ErrorIsNil)

	c.Check(logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{
			Level:   loggo.WARNING,
			Message: `property "auth-typs" is invalid. Perhaps you mean "auth-types".`,
		},
	})
}

func prepareTestCloudYaml(c *gc.C, data string) *os.File {
	cloudFile, err := ioutil.TempFile("", "cloudFile")
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(cloudFile.Name(), []byte(data), 0644)
	if err != nil {
		cloudFile.Close()
		os.Remove(cloudFile.Name())
		c.Fatal(err.Error())
	}

	return cloudFile
}

func (s *addSuite) TestInvalidCredentialMessage(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- oauth1\n" +
		"endpoint: http://mymaas\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(garageMAASCloud, nil)
	m1Cloud := garageMAASCloud
	m1Cloud.Name = "m1"
	m1Metadata := map[string]cloudfile.Cloud{"m1": m1Cloud}
	fake.Call("WritePersonalCloudMetadata", m1Metadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return command.CloudCallCtx.InvalidateCredential("running test")
	}

	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("" +
		/* Select cloud type: */ "maas\n" +
		/* Enter a name for the cloud: */ "m1\n" +
		/* Enter the controller's hostname or IP address: */ "http://mymaas\n",
	)

	err := command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "Cloud credential is not accepted by cloud provider: running test")
}

func (*addSuite) TestInteractiveOpenstackNoCloudCert(c *gc.C) {
	myOpenstack := cloudfile.Cloud{
		Name:      "os1",
		Type:      "openstack",
		AuthTypes: []cloudfile.AuthType{"userpass", "access-key"},
		Endpoint:  "http://myopenstack",
		Regions: []cloudfile.Region{
			{
				Name:     "regionone",
				Endpoint: "http://boston/1.0",
			},
		},
	}

	var expectedYAMLarg = "" +
		"auth-types:\n" +
		"- userpass\n" +
		"- access-key\n" +
		"certfilename: \"\"\n" +
		"endpoint: http://myopenstack\n" +
		"regions:\n" +
		"  regionone:\n" +
		"    endpoint: http://boston/1.0\n"

	var input = "" +
		/* Select cloud type: */ "openstack\n" +
		/* Enter a name for your openstack cloud: */ "os1\n" +
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n" +
		/* Enter ta path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "\n" +
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n" +
		/* Enter region name: */ "regionone\n" +
		/* Enter the API endpoint url for the region [use cloud api url]: */ "http://boston/1.0\n" +
		/* Enter another region? (Y/n): */ "n\n"

	testInteractiveOpenstack(c, myOpenstack, expectedYAMLarg, input, "", "")
}

// Note: The first %s is filled with a string containing a newline
var expectedCloudYAMLarg = `
auth-types:
- userpass
- access-key
%scertfilename: %s
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: ""
`[1:]

func (*addSuite) TestInteractiveOpenstackCloudCertFail(c *gc.C) {
	fakeCertDir := c.MkDir()
	fakeCertFilename := path.Join(fakeCertDir, "cloudcert.crt")

	invalidCertFilename := path.Join(fakeCertDir, "invalid.crt")
	ioutil.WriteFile(invalidCertFilename, []byte("testing certification validation"), 0666)

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", invalidCertFilename, fakeCertFilename)

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename),
		fmt.Sprintf("Can't validate CA Certificate %s: no certificates found", invalidCertFilename))
}

func (*addSuite) TestInteractiveOpenstackCloudCertReadFailRetry(c *gc.C) {
	var invalidCertFilename = "/tmp/no-such-file"
	fakeCertDir := c.MkDir()
	fakeCertFilename := path.Join(fakeCertDir, "cloudcert.crt")

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", invalidCertFilename, fakeCertFilename)

	testInteractiveOpenstackCloudCert(c,
		fakeCertFilename,
		input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename),
		fmt.Sprintf("Can't validate CA Certificate file: open %s:", invalidCertFilename),
	)
}

func (*addSuite) TestInteractiveOpenstackCloudCert(c *gc.C) {
	fakeCertFilename := path.Join(c.MkDir(), "cloudcert.crt")

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", fakeCertFilename)

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename), "")
}

type addOpenStackSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&addOpenStackSuite{})

func (s *addOpenStackSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	os.Unsetenv("OS_CACERT")
	os.Unsetenv("OS_AUTH_URL")
}

func (*addOpenStackSuite) TestInteractiveOpenstackCloudCertEnvVar(c *gc.C) {
	fakeCertFilename := path.Join(c.MkDir(), "cloudcert.crt")

	input := "" +
		/* Select cloud type: */ "openstack\n" +
		/* Enter a name for your openstack cloud: */ "os1\n" +
		/* Enter the API endpoint url for the cloud [$OS_AUTH_URL]: */ "\n" +
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [$OS_CACERT] */ "\n" +
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n" +
		/* Enter region name: */ "regionone\n" +
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n" +
		/* Enter another region? (Y/n): */ "n\n"

	os.Setenv("OS_CACERT", fakeCertFilename)
	os.Setenv("OS_AUTH_URL", "http://myopenstack")

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename), "")
}

func testInteractiveOpenstackCloudCert(c *gc.C, fakeCertFilename, input, addStdErrMsg, stdOutMsg string) {
	fakeCert := testing.CACert
	ioutil.WriteFile(fakeCertFilename, []byte(fakeCert), 0666)

	myOpenstack := cloudfile.Cloud{
		Name:      "os1",
		Type:      "openstack",
		AuthTypes: []cloudfile.AuthType{"userpass", "access-key"},
		Endpoint:  "http://myopenstack",
		Regions: []cloudfile.Region{
			{
				Name:     "regionone",
				Endpoint: "http://myopenstack",
			},
		},
		CACertificates: []string{fakeCert},
	}

	fakeCertMap := map[string]interface{}{
		"ca-certificates": []string{fakeCert},
	}
	fakeCertYaml, err := yaml.Marshal(fakeCertMap)
	c.Assert(err, gc.IsNil)

	expectedYAMLarg := fmt.Sprintf(expectedCloudYAMLarg, fakeCertYaml, fakeCertFilename)

	testInteractiveOpenstack(c, myOpenstack, expectedYAMLarg, input, addStdErrMsg, stdOutMsg)
}

func testInteractiveOpenstack(c *gc.C, myOpenstack cloudfile.Cloud, expectedYAMLarg, input, addStdErrMsg, stdOutMsg string) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)

	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(myOpenstack, nil)
	m1Metadata := map[string]cloudfile.Cloud{"os1": myOpenstack}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", m1Metadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	command.Ping = func(environs.EnvironProvider, string) error {
		return nil
	}
	err := cmdtesting.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader(input)

	err = command.Run(ctx)

	if err != nil {
		fmt.Printf("expectedYAML\n(%s)\n", expectedYAMLarg)
	}

	c.Check(err, jc.ErrorIsNil)
	var output = addStdErrMsg +
		"Cloud \"os1\" successfully added\n" +
		"\n" +
		"You will need to add credentials for this cloud (`juju add-credential os1`)\n" +
		"before creating a controller (`juju bootstrap os1`).\n"
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, output)
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, stdOutMsg)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}
