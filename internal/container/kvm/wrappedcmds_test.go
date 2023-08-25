// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	. "github.com/juju/juju/internal/container/kvm"
	coretesting "github.com/juju/juju/testing"
)

type LibVertSuite struct {
	coretesting.BaseSuite
	ContainerDir string
	RemovedDir   string
}

var _ = gc.Suite(&LibVertSuite{})

func (s *LibVertSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

type testSyncParams struct {
	arch, series string
	srcFunc      func() simplestreams.DataSource
	onevalErr    error
	success      bool
}

func (p testSyncParams) One() (*imagedownloads.Metadata, error) {
	if p.success {
		return &imagedownloads.Metadata{
			Arch:    p.arch,
			Release: p.series,
		}, nil
	}
	return nil, p.onevalErr
}

func (p testSyncParams) sourceURL() (string, error) {
	return p.srcFunc().URL("")
}

// Test that the call to SyncImages utilizes the defined source
func (s *LibVertSuite) TestSyncImagesUtilizesSimpleStreamsSource(c *gc.C) {

	const (
		series = "mocked-series"
		arch   = "mocked-arch"
		source = "mocked-url"
	)
	p := testSyncParams{
		arch:   arch,
		series: series,
		srcFunc: func() simplestreams.DataSource {
			ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
			return imagedownloads.NewDataSource(ss, source)
		},
		success: true,
	}
	err := Sync(p, fakeFetcher{}, source, nil)
	c.Assert(err, jc.ErrorIsNil)

	url, err := p.sourceURL()
	c.Check(err, jc.ErrorIsNil)
	c.Check(url, jc.DeepEquals, source+"/")

	res, err := p.One()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.Arch, jc.DeepEquals, arch)
	c.Check(res.Release, jc.DeepEquals, series)
}

// gocheck boilerplate.
type commandWrapperSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&commandWrapperSuite{})

func (commandWrapperSuite) TestCreateNoHostname(c *gc.C) {
	stub := NewRunStub("exit before this", nil)
	p := CreateMachineParams{}
	err := CreateMachine(p)
	c.Assert(len(stub.Calls()) == 0, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "hostname is required")
}

func (s *commandWrapperSuite) TestCreateMachineSuccessOnFocal(c *gc.C) {
	tmpDir, err := os.MkdirTemp("", "juju-libvirtSuite-")
	c.Check(err, jc.ErrorIsNil)

	want := []string{
		tmpDir + ` genisoimage -output \/tmp\/juju-libvirtSuite-\d+\/kvm\/guests\/host00-ds\.iso -volid cidata -joliet -rock user-data meta-data network-config`,
		// On focal, the backing image format must be explicitly specified
		// hence the '-F raw'
		` qemu-img create -b \/tmp/juju-libvirtSuite-\d+\/kvm\/guests\/20.04-arm64-backing-file.qcow -F raw -f qcow2 \/tmp\/juju-libvirtSuite-\d+\/kvm\/guests\/host00.qcow 8G`,
		` virsh define \/tmp\/juju-libvirtSuite-\d+\/host00.xml`,
		" virsh start host00",
	}

	assertCreateMachineSuccess(c, tmpDir, want)
}
func assertCreateMachineSuccess(c *gc.C, tmpDir string, expCommands []string) {
	stub := NewRunStub("success", nil)

	err := os.MkdirAll(filepath.Join(tmpDir, "kvm", "guests"), 0755)
	c.Check(err, jc.ErrorIsNil)
	cloudInitPath := filepath.Join(tmpDir, "cloud-init")
	userDataPath := filepath.Join(tmpDir, "user-data")
	networkConfigPath := filepath.Join(tmpDir, "network-config")
	err = os.WriteFile(cloudInitPath, []byte("#cloud-init\nEOF\n"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed removing %q in test %s", tmpDir, err)
		}
	}()
	pathfinder := func(_ paths.OS) string {
		return tmpDir
	}

	hostname := "host00"
	params := CreateMachineParams{
		Hostname:          hostname,
		Version:           "20.04",
		UserDataFile:      cloudInitPath,
		NetworkConfigData: "this-is-network-config",
		CpuCores:          1,
		RootDisk:          8,
	}

	MakeCreateMachineParamsTestable(&params, pathfinder, stub.Run, "arm64")
	err = CreateMachine(params)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(cloudInitPath)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	b, err := os.ReadFile(userDataPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), jc.Contains, "#cloud-init")

	b, err = os.ReadFile(networkConfigPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, "this-is-network-config")

	c.Check(len(stub.Calls()), gc.Equals, len(expCommands))
	for i, cmd := range stub.Calls() {
		c.Check(cmd, gc.Matches, expCommands[i])
	}
}

func (commandWrapperSuite) TestDestroyMachineSuccess(c *gc.C) {
	tmpDir, err := os.MkdirTemp("", "juju-libvirtSuite-")
	c.Check(err, jc.ErrorIsNil)
	guestBase := filepath.Join(tmpDir, "kvm", "guests")
	err = os.MkdirAll(guestBase, 0700)
	c.Check(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(guestBase, "aname.qcow"), []byte("diskcontents"), 0700)
	c.Check(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(guestBase, "aname-ds.iso"), []byte("diskcontents"), 0700)
	c.Check(err, jc.ErrorIsNil)

	pathfinder := func(_ paths.OS) string {
		return tmpDir
	}

	stub := NewRunStub("success", nil)
	container := NewTestContainer("aname", stub.Run, pathfinder)
	err = DestroyMachine(container)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		" virsh destroy aname",
		" virsh undefine --nvram aname",
	})
}

func (commandWrapperSuite) TestDestroyMachineFails(c *gc.C) {
	stub := NewRunStub("", errors.New("Boom"))
	container := NewTestContainer("aname", stub.Run, nil)
	err := DestroyMachine(container)
	c.Check(stub.Calls(), jc.DeepEquals, []string{
		" virsh destroy aname",
		" virsh undefine --nvram aname",
	})
	log := c.GetTestLog()
	c.Check(log, jc.Contains, "`virsh destroy aname` failed")
	c.Check(log, jc.Contains, "`virsh undefine --nvram aname` failed")
	c.Assert(err, jc.ErrorIsNil)

}

func (commandWrapperSuite) TestAutostartMachineSuccess(c *gc.C) {
	stub := NewRunStub("success", nil)
	container := NewTestContainer("aname", stub.Run, nil)
	err := AutostartMachine(container)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{" virsh autostart aname"})
	c.Assert(err, jc.ErrorIsNil)
}

func (commandWrapperSuite) TestAutostartMachineFails(c *gc.C) {
	stub := NewRunStub("", errors.New("Boom"))
	container := NewTestContainer("aname", stub.Run, nil)
	err := AutostartMachine(container)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{" virsh autostart aname"})
	c.Check(err, gc.ErrorMatches, `failed to autostart domain "aname": Boom`)
}

func (commandWrapperSuite) TestListMachinesSuccess(c *gc.C) {
	output := `
 Id    Name                           State
----------------------------------------------------
 0     Domain-0                       running
 2     ubuntu                         paused
`[1:]
	stub := NewRunStub(output, nil)
	got, err := ListMachines(stub.Run)

	c.Check(err, jc.ErrorIsNil)
	c.Check(stub.Calls(), jc.DeepEquals, []string{" virsh -q list --all"})
	c.Assert(got, jc.DeepEquals, map[string]string{
		"Domain-0": "running",
		"ubuntu":   "paused",
	})

}

func (commandWrapperSuite) TestListMachinesFails(c *gc.C) {
	stub := NewRunStub("", errors.New("Boom"))
	got, err := ListMachines(stub.Run)
	c.Check(err, gc.ErrorMatches, "Boom")
	c.Check(stub.Calls(), jc.DeepEquals, []string{" virsh -q list --all"})
	c.Assert(got, gc.IsNil)
}
