// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	cloudfile "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
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
	return results[0].(cloudfile.Cloud), jujutesting.TypeAssertError(results[1])
}

func (s *addSuite) TestAddBadArgs(c *gc.C) {
	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(nil), "cloud", "cloud.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

var (
	homestackCloud = cloudfile.Cloud{
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
	localhostCloud = cloudfile.Cloud{Type: "lxd"}
	awsCloud       = cloudfile.Cloud{
		Type:      "ec2",
		AuthTypes: []cloudfile.AuthType{"acccess-key"},
		Regions: []cloudfile.Region{
			{
				Name:     "us-east-1",
				Endpoint: "https://us-east-1.aws.amazon.com/v1.2/",
			},
		},
	}
	garageMAASCloud = cloudfile.Cloud{
		Type:      "maas",
		AuthTypes: []cloudfile.AuthType{"oauth"},
		Endpoint:  "http://garagemaas",
	}

	manualCloud = cloudfile.Cloud{
		Type:      "manual",
		AuthTypes: []cloudfile.AuthType{"manual"},
		Endpoint:  "192.168.1.6",
	}
)

func homestackMetadata() map[string]cloudfile.Cloud {
	return map[string]cloudfile.Cloud{"homestack": homestackCloud}
}

func localhostMetadata() map[string]cloudfile.Cloud {
	return map[string]cloudfile.Cloud{"localhost": localhostCloud}
}

func awsMetadata() map[string]cloudfile.Cloud {
	return map[string]cloudfile.Cloud{"aws": awsCloud}
}

func garageMAASMetadata() map[string]cloudfile.Cloud {
	return map[string]cloudfile.Cloud{"garage-maas": garageMAASCloud}
}

func (*addSuite) TestAddBadFilename(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	badFileErr := errors.New("")
	fake.Call("ParseCloudMetadataFile", "somefile.yaml").Returns(map[string]cloudfile.Cloud{}, badFileErr)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := testing.RunCommand(c, addCmd, "cloud", "somefile.yaml")
	c.Check(err, gc.Equals, badFileErr)
}

func (*addSuite) TestAddBadCloudName(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "testFile").Returns(map[string]cloudfile.Cloud{}, nil)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := testing.RunCommand(c, addCmd, "cloud", "testFile")
	c.Assert(err, gc.ErrorMatches, `cloud "cloud" not found in file .*`)
}

func (*addSuite) TestAddExisting(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(homestackMetadata(), nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "homestack", "fake.yaml")
	c.Assert(err, gc.ErrorMatches, `"homestack" already exists; use --replace to replace this existing cloud`)
}

func (*addSuite) TestAddExistingReplace(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(homestackMetadata(), nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", homestackMetadata()).Returns(nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "homestack", "fake.yaml", "--replace")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestAddExistingPublic(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(awsMetadata(), nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(awsMetadata(), false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "aws", "fake.yaml")
	c.Assert(err, gc.ErrorMatches, `"aws" is the name of a public cloud; use --replace to override this definition`)
}

func (*addSuite) TestAddExistingBuiltin(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(localhostMetadata(), nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "localhost", "fake.yaml")
	c.Assert(err, gc.ErrorMatches, `"localhost" is the name of a built-in cloud; use --replace to override this definition`)
}

func (*addSuite) TestAddExistingPublicReplace(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(awsMetadata(), nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(awsMetadata(), false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	writeCall := fake.Call("WritePersonalCloudMetadata", awsMetadata()).Returns(nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "aws", "fake.yaml", "--replace")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(writeCall(), gc.Equals, 1)
}

func (*addSuite) TestAddNew(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "fake.yaml").Returns(garageMAASMetadata(), nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", garageMAASMetadata()).Returns(nil)

	_, err := testing.RunCommand(c, cloud.NewAddCloudCommand(fake), "garage-maas", "fake.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractive(c *gc.C) {
	command := cloud.NewAddCloudCommand(nil)
	err := testing.InitCommand(command, nil)
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
		"  maas\n"+
		"  manual\n"+
		"  openstack\n"+
		"  vsphere\n"+
		"\n"+
		"Select cloud type: \n",
	)
}

func (*addSuite) TestInteractiveOpenstack(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	myOpenstack := cloudfile.Cloud{
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
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- userpass\n" +
		"- access-key\n" +
		"endpoint: http://myopenstack\n" +
		"regions:\n" +
		"  regionone:\n" +
		"    endpoint: http://boston/1.0\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(myOpenstack, nil)
	m1Metadata := map[string]cloudfile.Cloud{"os1": myOpenstack}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", m1Metadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			"openstack\n" +
			"os1\n" +
			"http://myopenstack\n" +
			"userpass,access-key\n" +
			"regionone\n" +
			"http://boston/1.0\n" +
			"n\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
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
	m1Metadata := map[string]cloudfile.Cloud{"m1": garageMAASCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", m1Metadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	err := testing.InitCommand(command, nil)
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
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]cloudfile.Cloud{}, nil)
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manualCloud, nil)
	manMetadata := map[string]cloudfile.Cloud{"man": manualCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", manMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	err := testing.InitCommand(command, nil)
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
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "vsphere\n" +
			/* Enter a name for the cloud: */ "mvs\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6\n" +
			/* Enter region name: */ "foo\n" +
			/* Enter another region? (Y/n): */ "y\n" +
			/* Enter region name: */ "bar\n" +
			/* Enter another region? (Y/n): */ "n\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	c.Check(numCallsToWrite(), gc.Equals, 1)
}

func (*addSuite) TestInteractiveExistingNameOverride(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloudfile.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	manMetadata := map[string]cloudfile.Cloud{"homestack": manualCloud}
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manualCloud, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", manMetadata).Returns(nil)

	command := cloud.NewAddCloudCommand(fake)
	err := testing.InitCommand(command, nil)
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
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "homestack" + "\n" +
			/* Do you want to replace that definition? (y/N): */ "n" + "\n" +
			/* Enter a name for the cloud: */ "homestack2" + "\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6" + "\n",
		),
	}

	err = command.Run(ctx)
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
	command.Run(ctx)

	c.Check(out.String(), gc.Matches, "(?s).+Enter a name for your manual cloud: .*")
}
