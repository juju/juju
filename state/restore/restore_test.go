// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package restore

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	stdtesting "testing"

	gitjujutesting "github.com/juju/testing"
	"gopkg.in/mgo.v2"
	gc "launchpad.net/gocheck"

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

func (b *RestoreSuite) SetUpSuite(c *gc.C) {
	b.BaseSuite.SetUpSuite(c)
}

func (b *RestoreSuite) SetUpTest(c *gc.C) {
	b.cwd = c.MkDir()
	b.BaseSuite.SetUpTest(c)
}

func (b *RestoreSuite) createTestFiles(c *gc.C) {
	tarDirE := path.Join(b.cwd, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarDirP := path.Join(b.cwd, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarSubFile1 := path.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, gc.IsNil)
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarFile1 := path.Join(b.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := path.Join(b.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()
	b.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}
}

func (r *RestoreSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, gc.IsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Members, gc.HasLen, 1)
	c.Assert(cfg.Members[0].Address, gc.Equals, mgoAddr)
}

func (r *RestoreSuite) TestDirectoriesCleaned(c *gc.C) {
	recreatableFolder := filepath.Join(r.cwd, "recreate_me")
	os.MkdirAll(recreatableFolder, os.FileMode(0755))
	recreatableFolderInfo, err := os.Stat(recreatableFolder)
	c.Assert(err, gc.IsNil)

	recreatableFolder1 := filepath.Join(recreatableFolder, "recreate_me_too")
	os.MkdirAll(recreatableFolder1, os.FileMode(0755))
	recreatableFolder1Info, err := os.Stat(recreatableFolder1)
	c.Assert(err, gc.IsNil)

	deletableFolder := filepath.Join(recreatableFolder, "dont_recreate_me")
	os.MkdirAll(deletableFolder, os.FileMode(0755))

	deletableFile := filepath.Join(recreatableFolder, "delete_me")
	fh, err := os.Create(deletableFile)
	c.Assert(err, gc.IsNil)
	defer fh.Close()

	deletableFile1 := filepath.Join(recreatableFolder1, "delete_me.too")
	fhr, err := os.Create(deletableFile1)
	c.Assert(err, gc.IsNil)
	defer fhr.Close()

	r.PatchValue(&replaceableFiles, func() (map[string]os.FileMode, error) {
		replaceables := map[string]os.FileMode{}
		for _, replaceable := range []string{
			recreatableFolder,
			recreatableFolder1,
		} {
			dirStat, err := os.Stat(replaceable)
			if err != nil {
				return map[string]os.FileMode{}, err
			}
			replaceables[replaceable] = dirStat.Mode()
		}
		return replaceables, nil
	})

	err = prepareMachineForRestore()
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(deletableFolder)
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(os.IsNotExist(err), gc.Equals, true)

	recreatedFolderInfo, err := os.Stat(recreatableFolder)
	c.Assert(err, gc.IsNil)
	c.Assert(recreatableFolderInfo.Mode(), gc.Equals, recreatedFolderInfo.Mode())
	c.Assert(recreatableFolderInfo.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolderInfo.Sys().(*syscall.Stat_t).Ino)

	recreatedFolder1Info, err := os.Stat(recreatableFolder1)
	c.Assert(err, gc.IsNil)
	c.Assert(recreatableFolder1Info.Mode(), gc.Equals, recreatedFolder1Info.Mode())
	c.Assert(recreatableFolder1Info.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolder1Info.Sys().(*syscall.Stat_t).Ino)

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

func (r *RestoreSuite) TestFetchConfigFromBackupFailures(c *gc.C) {
	testCases := []backupConfigTests{{
		yamlFile:      bytes.NewBuffer([]byte{}),
		expectedError: fmt.Errorf("config file unmarshalled to %T not %T", nil, map[interface{}]interface{}{}),
		message:       "Fails on emtpy/invalid yaml.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:2], "\n"))),
		expectedError: fmt.Errorf("tag not found in configuration"),
		message:       "Fails when tag key is not present.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:3], "\n"))),
		expectedError: fmt.Errorf("agent tag user password not found in configuration"),
		message:       "Fails when state password key is not present.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:4], "\n"))),
		expectedError: fmt.Errorf("agent admin password not found in configuration"),
		message:       "Fails when oldpassword key is not pressent.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:5], "\n"))),
		expectedError: fmt.Errorf("state port not found in configuration"),
		message:       "Fails when stateport key is not present.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:6], "\n"))),
		expectedError: fmt.Errorf("api port not found in configuration"),
		message:       "Fails when apiport key is not present.",
	}, {
		yamlFile:      bytes.NewBuffer([]byte(strings.Join(yamlLines[:7], "\n"))),
		expectedError: fmt.Errorf("CACert not found in configuration"),
		message:       "Fails when cacert key is not present.",
	},
	}
	for _, tCase := range testCases {
		_, err := fetchAgentConfigFromBackup(tCase.yamlFile)
		logger.Infof(tCase.message)
		c.Assert(err, gc.DeepEquals, tCase.expectedError)
	}

}

func (r *RestoreSuite) TestFetchConfigFromBackupSuccess(c *gc.C) {
	yamlFile := bytes.NewBuffer([]byte(strings.Join(yamlLines, "\n")))
	aConf, err := fetchAgentConfigFromBackup(yamlFile)
	c.Assert(err, gc.IsNil)
	expectedConf := agentConfig{
		credentials: credentials{
			tag:           "aTag",
			tagPassword:   "aStatePassword",
			adminUsername: "admin",
			adminPassword: "anOldPassword",
		},
		apiPort:   "2",
		statePort: "1",
		cACert:    "aLengthyCACert",
	}
	c.Assert(aConf, gc.DeepEquals, expectedConf)
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
