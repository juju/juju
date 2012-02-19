package store_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/store"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BranchSuite struct {
	StoreSuite
	branch bzrDir
	urls []*charm.URL
}

var _ = Suite(&BranchSuite{})

func (s *BranchSuite) SetUpTest(c *C) {
	s.StoreSuite.SetUpTest(c)

	s.branch = bzrDir(c.MkDir())
	s.branch.init()

	copyCharmDir(s.branch.path(), s.charm.Path)
	s.branch.add()
	s.branch.commit("Imported charm.")

	url1 := charm.MustParseURL("cs:~joe/oneiric/dummy")
	url2 := charm.MustParseURL("cs:oneiric/dummy")
	s.urls = []*charm.URL{url1, url2}
}

func (s *BranchSuite) TestPublish(c *C) {
	err := store.PublishBazaarBranch(s.store, s.urls, s.branch.path(), "wrong-rev")
	c.Assert(err, IsNil)

	for _, url := range s.urls {
		info, rc, err := s.store.OpenCharm(url)
		c.Assert(err, IsNil)
		defer rc.Close()
		c.Assert(info.Revision(), Equals, 0)
		c.Assert(info.Meta().Name, Equals, "dummy")

		data, err := ioutil.ReadAll(rc)
		c.Assert(err, IsNil)

		bundle, err := charm.ReadBundleBytes(data)
		c.Assert(err, IsNil)
		c.Assert(bundle.Revision(), Equals, 0)
		c.Assert(bundle.Meta().Name, Equals, "dummy")
	}

	// Attempt to publish the same content again while providing the wrong
	// tip revision. It must pick the real revision from the branch and
	// note this was previously published.
	err = store.PublishBazaarBranch(s.store, s.urls, s.branch.path(), "wrong-rev")
	c.Assert(err, Equals, store.ErrRedundantUpdate)

	// Bump the content revision and lie again about the known tip revision.
	// This time, though, pretend it's the same as the real branch revision
	// previously published. It must error and not publish the new revision
	// because it will use the revision provided as a parameter to check if
	// publishing was attempted before. This is the mechanism that enables
	// stopping fast without having to download every single branch. Real
	// revision is picked in the next scan.
	digest1 := s.branch.digest()
	s.branch.change()
	err = store.PublishBazaarBranch(s.store, s.urls, s.branch.path(), digest1)
	c.Assert(err, Equals, store.ErrRedundantUpdate)

	// Now allow it to publish the new content by providing an unseen revision.
	err = store.PublishBazaarBranch(s.store, s.urls, s.branch.path(), "wrong-rev")
	c.Assert(err, IsNil)
	digest2 := s.branch.digest()

	info, err := s.store.CharmInfo(s.urls[0])
	c.Assert(err, IsNil)
	c.Assert(info.Revision(), Equals, 1)
	c.Assert(info.Meta().Name, Equals, "dummy")

	// There are two events published, for each of the successful attempts.
	// The failures are ignored given that they are artifacts of the
	// publishing mechanism rather than actual problems.
	_, err = s.store.CharmEvent(s.urls[0], "wrong-rev")
	c.Assert(err, Equals, store.ErrNotFound)
	for i, digest := range []string{digest1, digest2} {
		event, err := s.store.CharmEvent(s.urls[0], digest)
		c.Assert(err, IsNil)
		c.Assert(event.Kind, Equals, store.EventPublished)
		c.Assert(event.Revision, Equals, i)
		c.Assert(event.Errors, IsNil)
		c.Assert(event.Warnings, IsNil)
	}
}

func (s *BranchSuite) TestPublishError(c *C) {
	// Corrupt the charm.
	s.branch.remove("metadata.yaml")
	s.branch.commit("Removed metadata.yaml.")

	// Attempt to publish the erroneous content.
	err := store.PublishBazaarBranch(s.store, s.urls, s.branch.path(), "wrong-rev")
	c.Assert(err, ErrorMatches, ".*/metadata.yaml: no such file or directory")

	// The event should be logged as well, since this was an error in the charm
	// that won't go away and must be communicated to the author.
	event, err := s.store.CharmEvent(s.urls[0], s.branch.digest())
	c.Assert(err, IsNil)
	c.Assert(event.Kind, Equals, store.EventPublishError)
	c.Assert(event.Revision, Equals, 0)
	c.Assert(event.Errors, NotNil)
	c.Assert(event.Errors[0], Matches, ".*/metadata.yaml: no such file or directory")
	c.Assert(event.Warnings, IsNil)
}

type bzrDir string

func (dir bzrDir) path(args ...string) string {
	return filepath.Join(append([]string{string(dir)}, args...)...)
}

func (dir bzrDir) run(args ...string) []byte {
	cmd := exec.Command("bzr", args...)
	cmd.Dir = string(dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("command failed: bzr %s\n%s", strings.Join(args, " "), output))
	}
	return output
}

func (dir bzrDir) init() {
	dir.run("init")
}

func (dir bzrDir) add(paths ...string) {
	dir.run(append([]string{"add"}, paths...)...)
}

func (dir bzrDir) remove(paths ...string) {
	dir.run(append([]string{"rm"}, paths...)...)
}

func (dir bzrDir) commit(msg string) {
	dir.run("commit", "-m", msg)
}

func (dir bzrDir) write(path string, data string) {
	err := ioutil.WriteFile(dir.path(path), []byte(data), 0644)
	if err != nil {
		panic(err)
	}
}

func (dir bzrDir) change() {
	t := time.Now().String()
	dir.write("timestamp", t)
	dir.add("timestamp")
	dir.commit("Revision bumped at " + t)
}

func (dir bzrDir) digest() string {
	output := dir.run("revision-info")
	f := bytes.Fields(output)
	if len(f) != 2 {
		panic("revision-info returned bad output: " + string(output))
	}
	return string(f[1])
}

func copyCharmDir(dst, src string) {
	dir, err := charm.ReadDir(src)
	if err != nil {
		panic(err)
	}
	var b bytes.Buffer
	err = dir.BundleTo(&b)
	if err != nil {
		panic(err)
	}
	bundle, err := charm.ReadBundleBytes(b.Bytes())
	if err != nil {
		panic(err)
	}
	err = bundle.ExpandTo(dst)
	if err != nil {
		panic(err)
	}
}
