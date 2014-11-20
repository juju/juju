// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	stdtesting "testing"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/replicaset"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&RestoreSuite{})

type RestoreSuite struct {
	coretesting.BaseSuite
	cwd       string
	testFiles []string
}

func (r *RestoreSuite) SetUpSuite(c *gc.C) {
	r.BaseSuite.SetUpSuite(c)
}

func (r *RestoreSuite) SetUpTest(c *gc.C) {
	r.cwd = c.MkDir()
	r.BaseSuite.SetUpTest(c)
}

func (r *RestoreSuite) createTestFiles(c *gc.C) {
	tarDirE := path.Join(r.cwd, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, jc.IsNil)

	tarDirP := path.Join(r.cwd, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, jc.IsNil)

	tarSubFile1 := path.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, jc.IsNil)
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, jc.IsNil)

	tarFile1 := path.Join(r.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, jc.IsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := path.Join(r.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, jc.IsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()
	r.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}
}

func (r *RestoreSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, jc.IsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, jc.IsNil)
	return mongo.EnsureAdminUser(mongo.EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
}

func (r *RestoreSuite) TestReplicasetIsReset(c *gc.C) {
	server := &gitjujutesting.MgoInstance{Params: []string{"--replSet", "juju"}}
	err := server.Start(coretesting.Certs)
	c.Assert(err, jc.IsNil)
	defer server.DestroyWithLog()
	mgoAddr := server.Addr()
	dialInfo := server.DialInfo()

	var cfg *replicaset.Config
	dialInfo = server.DialInfo()
	dialInfo.Addrs = []string{mgoAddr}
	err = resetReplicaSet(dialInfo, mgoAddr)

	session := server.MustDial()
	defer session.Close()
	cfg, err = replicaset.CurrentConfig(session)
	c.Assert(err, jc.IsNil)
	c.Assert(cfg.Members, gc.HasLen, 1)
	c.Assert(cfg.Members[0].Address, gc.Equals, mgoAddr)
}

type backupConfigTests struct {
	yamlFile      io.Reader
	expectedError error
	message       string
}

var yamlLines = []string{
	"# format 1.18",
	"bogus: aBogusValue",
	"tag: aTag",
	"statepassword: aStatePassword",
	"oldpassword: anOldPassword",
	"stateport: 1",
	"apiport: 2",
	"cacert: aLengthyCACert",
}

func (r *RestoreSuite) TestSetAgentAddressScript(c *gc.C) {
	testServerAddresses := []string{
		"FirstNewStateServerAddress:30303",
		"SecondNewStateServerAddress:30304",
		"ThirdNewStateServerAddress:30305",
		"FourthNewStateServerAddress:30306",
		"FiftNewStateServerAddress:30307",
		"SixtNewStateServerAddress:30308",
	}
	for _, address := range testServerAddresses {
		template := setAgentAddressScript(address)
		expectedString := fmt.Sprintf("\t\ts/- .*(:[0-9]+)/- %s\\1/\n", address)
		logger.Infof(fmt.Sprintf("Testing with address %q", address))
		c.Assert(strings.Contains(template, expectedString), gc.Equals, true)
	}
}
