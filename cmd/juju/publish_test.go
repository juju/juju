// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/bzr"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

// Sadly, this is a very slow test suite, heavily dominated by calls to bzr.

type PublishSuite struct {
	testbase.LoggingSuite
	testing.HTTPSuite

	home       *testing.FakeHome
	dir        string
	oldBaseURL string
	branch     *bzr.Branch
}

var _ = gc.Suite(&PublishSuite{})

func touch(c *gc.C, filename string) {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	c.Assert(err, gc.IsNil)
	f.Close()
}

func addMeta(c *gc.C, branch *bzr.Branch, meta string) {
	if meta == "" {
		meta = "name: wordpress\nsummary: Some summary\ndescription: Some description.\n"
	}
	f, err := os.Create(branch.Join("metadata.yaml"))
	c.Assert(err, gc.IsNil)
	_, err = f.Write([]byte(meta))
	f.Close()
	c.Assert(err, gc.IsNil)
	err = branch.Add("metadata.yaml")
	c.Assert(err, gc.IsNil)
	err = branch.Commit("Added metadata.yaml.")
	c.Assert(err, gc.IsNil)
}

func (s *PublishSuite) runPublish(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommandInDir(c, &PublishCommand{}, args, s.dir)
}

const pollDelay = testing.ShortWait

func (s *PublishSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)

	s.oldBaseURL = charm.Store.BaseURL
	charm.Store.BaseURL = s.URL("")
}

func (s *PublishSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)

	charm.Store.BaseURL = s.oldBaseURL
}

func (s *PublishSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeWithFiles(c, []testing.TestFile{
		{
			Name: ".bazaar/bazaar.conf",
			Data: "[DEFAULT]\nemail = Test <testing@testing.invalid>\n",
		},
	})

	s.dir = c.MkDir()
	s.branch = bzr.New(s.dir)
	err := s.branch.Init()
	c.Assert(err, gc.IsNil)
}

func (s *PublishSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.HTTPSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *PublishSuite) TestNoBranch(c *gc.C) {
	dir := c.MkDir()
	_, err := testing.RunCommandInDir(c, &PublishCommand{}, []string{"cs:precise/wordpress"}, dir)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("not a charm branch: %s", dir))
}

func (s *PublishSuite) TestEmpty(c *gc.C) {
	_, err := s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.ErrorMatches, `cannot obtain local digest: branch has no content`)
}

func (s *PublishSuite) TestFrom(c *gc.C) {
	_, err := testing.RunCommandInDir(c, &PublishCommand{}, []string{"--from", s.dir, "cs:precise/wordpress"}, c.MkDir())
	c.Assert(err, gc.ErrorMatches, `cannot obtain local digest: branch has no content`)
}

func (s *PublishSuite) TestMissingSeries(c *gc.C) {
	_, err := s.runPublish(c, "cs:wordpress")
	c.Assert(err, gc.ErrorMatches, `cannot infer charm URL for "cs:wordpress": no series provided`)
}

func (s *PublishSuite) TestNotClean(c *gc.C) {
	touch(c, s.branch.Join("file"))
	_, err := s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.ErrorMatches, `branch is not clean \(bzr status\)`)
}

func (s *PublishSuite) TestNoPushLocation(c *gc.C) {
	addMeta(c, s.branch, "")
	_, err := s.runPublish(c)
	c.Assert(err, gc.ErrorMatches, `no charm URL provided and cannot infer from current directory \(no push location\)`)
}

func (s *PublishSuite) TestUnknownPushLocation(c *gc.C) {
	addMeta(c, s.branch, "")
	err := s.branch.Push(&bzr.PushAttr{Location: c.MkDir() + "/foo", Remember: true})
	c.Assert(err, gc.IsNil)
	_, err = s.runPublish(c)
	c.Assert(err, gc.ErrorMatches, `cannot infer charm URL from branch location: ".*/foo"`)
}

func (s *PublishSuite) TestWrongRepository(c *gc.C) {
	addMeta(c, s.branch, "")
	_, err := s.runPublish(c, "local:precise/wordpress")
	c.Assert(err, gc.ErrorMatches, "charm URL must reference the juju charm store")
}

func (s *PublishSuite) TestInferURL(c *gc.C) {
	addMeta(c, s.branch, "")

	cmd := &PublishCommand{}
	cmd.ChangePushLocation(func(location string) string {
		c.Assert(location, gc.Equals, "lp:charms/precise/wordpress")
		c.SucceedNow()
		panic("unreachable")
	})

	_, err := testing.RunCommandInDir(c, cmd, []string{"precise/wordpress"}, s.dir)
	c.Assert(err, gc.IsNil)
	c.Fatal("shouldn't get here; location closure didn't run?")
}

func (s *PublishSuite) TestBrokenCharm(c *gc.C) {
	addMeta(c, s.branch, "name: wordpress\nsummary: Some summary\n")
	_, err := s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.ErrorMatches, "metadata: description: expected string, got nothing")
}

func (s *PublishSuite) TestWrongName(c *gc.C) {
	addMeta(c, s.branch, "")
	_, err := s.runPublish(c, "cs:precise/mysql")
	c.Assert(err, gc.ErrorMatches, `charm name in metadata must match name in URL: "wordpress" != "mysql"`)
}

func (s *PublishSuite) TestPreExistingPublished(c *gc.C) {
	addMeta(c, s.branch, "")

	// Pretend the store has seen the digest before, and it has succeeded.
	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)
	body := `{"cs:precise/wordpress": {"kind": "published", "digest": %q, "revision": 42}}`
	testing.Server.Response(200, nil, []byte(fmt.Sprintf(body, digest)))

	ctx, err := s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "cs:precise/wordpress-42\n")

	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:precise/wordpress@"+digest)
}

func (s *PublishSuite) TestPreExistingPublishedEdge(c *gc.C) {
	addMeta(c, s.branch, "")

	// If it doesn't find the right digest on the first try, it asks again for
	// any digest at all to keep the tip in mind. There's a small chance that
	// on the second request the tip has changed and matches the digest we're
	// looking for, in which case we have the answer already.
	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)
	var body string
	body = `{"cs:precise/wordpress": {"errors": ["entry not found"]}}`
	testing.Server.Response(200, nil, []byte(body))
	body = `{"cs:precise/wordpress": {"kind": "published", "digest": %q, "revision": 42}}`
	testing.Server.Response(200, nil, []byte(fmt.Sprintf(body, digest)))

	ctx, err := s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "cs:precise/wordpress-42\n")

	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:precise/wordpress@"+digest)

	req = testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:precise/wordpress")
}

func (s *PublishSuite) TestPreExistingPublishError(c *gc.C) {
	addMeta(c, s.branch, "")

	// Pretend the store has seen the digest before, and it has failed.
	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)
	body := `{"cs:precise/wordpress": {"kind": "publish-error", "digest": %q, "errors": ["an error"]}}`
	testing.Server.Response(200, nil, []byte(fmt.Sprintf(body, digest)))

	_, err = s.runPublish(c, "cs:precise/wordpress")
	c.Assert(err, gc.ErrorMatches, "charm could not be published: an error")

	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:precise/wordpress@"+digest)
}

func (s *PublishSuite) TestFullPublish(c *gc.C) {
	addMeta(c, s.branch, "")

	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)

	pushBranch := bzr.New(c.MkDir())
	err = pushBranch.Init()
	c.Assert(err, gc.IsNil)

	cmd := &PublishCommand{}
	cmd.ChangePushLocation(func(location string) string {
		c.Assert(location, gc.Equals, "lp:~user/charms/precise/wordpress/trunk")
		return pushBranch.Location()
	})
	cmd.SetPollDelay(testing.ShortWait)

	var body string

	// The local digest isn't found.
	body = `{"cs:~user/precise/wordpress": {"kind": "", "errors": ["entry not found"]}}`
	testing.Server.Response(200, nil, []byte(body))

	// But the charm exists with an arbitrary non-matching digest.
	body = `{"cs:~user/precise/wordpress": {"kind": "published", "digest": "other-digest"}}`
	testing.Server.Response(200, nil, []byte(body))

	// After the branch is pushed we fake the publishing delay.
	body = `{"cs:~user/precise/wordpress": {"kind": "published", "digest": "other-digest"}}`
	testing.Server.Response(200, nil, []byte(body))

	// And finally report success.
	body = `{"cs:~user/precise/wordpress": {"kind": "published", "digest": %q, "revision": 42}}`
	testing.Server.Response(200, nil, []byte(fmt.Sprintf(body, digest)))

	ctx, err := testing.RunCommandInDir(c, cmd, []string{"cs:~user/precise/wordpress"}, s.dir)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "cs:~user/precise/wordpress-42\n")

	// Ensure the branch was actually pushed.
	pushDigest, err := pushBranch.RevisionId()
	c.Assert(err, gc.IsNil)
	c.Assert(pushDigest, gc.Equals, digest)

	// And that all the requests were sent with the proper data.
	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress@"+digest)

	for i := 0; i < 3; i++ {
		// The second request grabs tip to see the current state, and the
		// following requests are done after pushing to see when it changes.
		req = testing.Server.WaitRequest()
		c.Assert(req.URL.Path, gc.Equals, "/charm-event")
		c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress")
	}
}

func (s *PublishSuite) TestFullPublishError(c *gc.C) {
	addMeta(c, s.branch, "")

	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)

	pushBranch := bzr.New(c.MkDir())
	err = pushBranch.Init()
	c.Assert(err, gc.IsNil)

	cmd := &PublishCommand{}
	cmd.ChangePushLocation(func(location string) string {
		c.Assert(location, gc.Equals, "lp:~user/charms/precise/wordpress/trunk")
		return pushBranch.Location()
	})
	cmd.SetPollDelay(pollDelay)

	var body string

	// The local digest isn't found.
	body = `{"cs:~user/precise/wordpress": {"kind": "", "errors": ["entry not found"]}}`
	testing.Server.Response(200, nil, []byte(body))

	// And tip isn't found either, meaning the charm was never published.
	testing.Server.Response(200, nil, []byte(body))

	// After the branch is pushed we fake the publishing delay.
	testing.Server.Response(200, nil, []byte(body))

	// And finally report success.
	body = `{"cs:~user/precise/wordpress": {"kind": "published", "digest": %q, "revision": 42}}`
	testing.Server.Response(200, nil, []byte(fmt.Sprintf(body, digest)))

	ctx, err := testing.RunCommandInDir(c, cmd, []string{"cs:~user/precise/wordpress"}, s.dir)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "cs:~user/precise/wordpress-42\n")

	// Ensure the branch was actually pushed.
	pushDigest, err := pushBranch.RevisionId()
	c.Assert(err, gc.IsNil)
	c.Assert(pushDigest, gc.Equals, digest)

	// And that all the requests were sent with the proper data.
	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress@"+digest)

	for i := 0; i < 3; i++ {
		// The second request grabs tip to see the current state, and the
		// following requests are done after pushing to see when it changes.
		req = testing.Server.WaitRequest()
		c.Assert(req.URL.Path, gc.Equals, "/charm-event")
		c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress")
	}
}

func (s *PublishSuite) TestFullPublishRace(c *gc.C) {
	addMeta(c, s.branch, "")

	digest, err := s.branch.RevisionId()
	c.Assert(err, gc.IsNil)

	pushBranch := bzr.New(c.MkDir())
	err = pushBranch.Init()
	c.Assert(err, gc.IsNil)

	cmd := &PublishCommand{}
	cmd.ChangePushLocation(func(location string) string {
		c.Assert(location, gc.Equals, "lp:~user/charms/precise/wordpress/trunk")
		return pushBranch.Location()
	})
	cmd.SetPollDelay(pollDelay)

	var body string

	// The local digest isn't found.
	body = `{"cs:~user/precise/wordpress": {"kind": "", "errors": ["entry not found"]}}`
	testing.Server.Response(200, nil, []byte(body))

	// And tip isn't found either, meaning the charm was never published.
	testing.Server.Response(200, nil, []byte(body))

	// After the branch is pushed we fake the publishing delay.
	testing.Server.Response(200, nil, []byte(body))

	// But, surprisingly, the digest changed to something else entirely.
	body = `{"cs:~user/precise/wordpress": {"kind": "published", "digest": "surprising-digest", "revision": 42}}`
	testing.Server.Response(200, nil, []byte(body))

	_, err = testing.RunCommandInDir(c, cmd, []string{"cs:~user/precise/wordpress"}, s.dir)
	c.Assert(err, gc.ErrorMatches, `charm changed but not to local charm digest; publishing race\?`)

	// Ensure the branch was actually pushed.
	pushDigest, err := pushBranch.RevisionId()
	c.Assert(err, gc.IsNil)
	c.Assert(pushDigest, gc.Equals, digest)

	// And that all the requests were sent with the proper data.
	req := testing.Server.WaitRequest()
	c.Assert(req.URL.Path, gc.Equals, "/charm-event")
	c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress@"+digest)

	for i := 0; i < 3; i++ {
		// The second request grabs tip to see the current state, and the
		// following requests are done after pushing to see when it changes.
		req = testing.Server.WaitRequest()
		c.Assert(req.URL.Path, gc.Equals, "/charm-event")
		c.Assert(req.Form.Get("charms"), gc.Equals, "cs:~user/precise/wordpress")
	}
}
