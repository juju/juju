// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package restore

import (
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	stdtesting "testing"

	gitjujutesting "github.com/juju/testing"
	"labix.org/v2/mgo"
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

func (r *RestoreSuite) TestFSIsPrepared(c *gc.C) {
	recreableFolder := filepath.Join(r.cwd, "recreate_me")
	os.MkdirAll(recreableFolder, os.FileMode(0755))
	recreableFolderInfo, err := os.Stat(recreableFolder)
	c.Assert(err, gc.IsNil)

	recreableFolder1 := filepath.Join(recreableFolder, "recreate_me_too")
	os.MkdirAll(recreableFolder1, os.FileMode(0755))
	recreableFolder1Info, err := os.Stat(recreableFolder1)
	c.Assert(err, gc.IsNil)

	deletableFolder := filepath.Join(recreableFolder, "dont_recreate_me")
	os.MkdirAll(deletableFolder, os.FileMode(0755))

	deletableFile := filepath.Join(recreableFolder, "delete_me")
	fh, err := os.Create(deletableFile)
	c.Assert(err, gc.IsNil)
	defer fh.Close()

	deletableFile1 := filepath.Join(recreableFolder1, "delete_me.too")
	fhr, err := os.Create(deletableFile1)
	c.Assert(err, gc.IsNil)
	defer fhr.Close()

	replaceableFiles = func() (map[string]os.FileMode, error) {
		replaceables := map[string]os.FileMode{}
		for _, replaceable := range []string{
			recreableFolder,
			recreableFolder1,
		} {
			dirStat, err := os.Stat(replaceable)
			if err != nil {
				return map[string]os.FileMode{}, err
			}
			replaceables[replaceable] = dirStat.Mode()
		}
		return replaceables, nil
	}

	err = prepareMachineForBackup()
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(deletableFolder)
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(os.IsNotExist(err), gc.Equals, true)

	recreatedFolderInfo, err := os.Stat(recreableFolder)
	c.Assert(err, gc.IsNil)
	c.Assert(recreableFolderInfo.Mode(), gc.Equals, recreatedFolderInfo.Mode())
	c.Assert(recreableFolderInfo.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolderInfo.Sys().(*syscall.Stat_t).Ino)

	recreatedFolder1Info, err := os.Stat(recreableFolder1)
	c.Assert(err, gc.IsNil)
	c.Assert(recreableFolder1Info.Mode(), gc.Equals, recreatedFolder1Info.Mode())
	c.Assert(recreableFolder1Info.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolder1Info.Sys().(*syscall.Stat_t).Ino)

}
