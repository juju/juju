// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package network_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type UtilsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UtilsSuite{})

func (*UtilsSuite) TestParseResolvConfEmptyOrMissingPath(c *gc.C) {
	emptyPath := ""
	missingPath := filepath.Join(c.MkDir(), "missing")

	for _, path := range []string{emptyPath, missingPath} {
		result, err := network.ParseResolvConf(path)
		c.Check(err, jc.ErrorIsNil)
		c.Check(result, gc.IsNil)
	}
}

func (*UtilsSuite) TestParseResolvConfNotReadablePath(c *gc.C) {
	unreadableConf := makeResolvConf(c, "#empty", 0000)
	result, err := network.ParseResolvConf(unreadableConf)
	expected := fmt.Sprintf("open %s: permission denied", unreadableConf)
	c.Check(err, gc.ErrorMatches, expected)
	c.Check(result, gc.IsNil)
}

func makeResolvConf(c *gc.C, content string, perms os.FileMode) string {
	fakeConfPath := filepath.Join(c.MkDir(), "fake")
	err := ioutil.WriteFile(fakeConfPath, []byte(content), perms)
	c.Check(err, jc.ErrorIsNil)
	return fakeConfPath
}

func (*UtilsSuite) TestParseResolvConfEmptyFile(c *gc.C) {
	emptyConf := makeResolvConf(c, "", 0644)
	result, err := network.ParseResolvConf(emptyConf)
	c.Check(err, jc.ErrorIsNil)
	// Expected non-nil, but empty result.
	c.Check(result, jc.DeepEquals, &network.DNSConfig{})
}

func (*UtilsSuite) TestParseResolvConfCommentsAndWhitespaceHandling(c *gc.C) {
	const exampleConf = `
  ;; comment
# also comment
;# ditto
  #nameserver ;still comment

  search    foo example.com       bar.     ;comment, leading/trailing ignored
nameserver 8.8.8.8 #comment #still the same comment
`
	fakeConf := makeResolvConf(c, exampleConf, 0644)
	result, err := network.ParseResolvConf(fakeConf)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, &network.DNSConfig{
		Nameservers:   network.NewAddresses("8.8.8.8"),
		SearchDomains: []string{"foo", "example.com", "bar."},
	})
}

func (*UtilsSuite) TestParseResolvConfSearchWithoutValue(c *gc.C) {
	badConf := makeResolvConf(c, "search # no value\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "search": required value\(s\) missing`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfNameserverWithoutValue(c *gc.C) {
	badConf := makeResolvConf(c, "nameserver", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "nameserver": required value\(s\) missing`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfValueFollowedByCommentWithoutWhitespace(c *gc.C) {
	badConf := makeResolvConf(c, "search foo bar#bad rest;is#ignored: still part of the comment", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "search": invalid value "bar#bad"`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfNameserverWithMultipleValues(c *gc.C) {
	badConf := makeResolvConf(c, "nameserver one two 42 ;;; comment still-inside-comment\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: one value expected for "nameserver", got 3`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfLastSearchWins(c *gc.C) {
	const multiSearchConf = `
search zero five
search one
# this below overrides all of the above
search two three #comment ;also-comment still-comment
`
	fakeConf := makeResolvConf(c, multiSearchConf, 0644)
	result, err := network.ParseResolvConf(fakeConf)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, &network.DNSConfig{
		SearchDomains: []string{"two", "three"},
	})
}

func (s *UtilsSuite) TestSupportsIPv6Error(c *gc.C) {
	s.PatchValue(network.NetListen, func(netFamily, bindAddress string) (net.Listener, error) {
		c.Check(netFamily, gc.Equals, "tcp6")
		c.Check(bindAddress, gc.Equals, "[::1]:0")
		return nil, errors.New("boom!")
	})
	c.Check(network.SupportsIPv6(), jc.IsFalse)
}

type mockListener struct {
	net.Listener
}

func (*mockListener) Close() error {
	return nil
}

func (s *UtilsSuite) TestSupportsIPv6OK(c *gc.C) {
	s.PatchValue(network.NetListen, func(_, _ string) (net.Listener, error) {
		return &mockListener{}, nil
	})
	c.Check(network.SupportsIPv6(), jc.IsTrue)
}
