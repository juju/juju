// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type dnsSuite struct {
	testhelpers.IsolationSuite
}

func TestDnsSuite(t *stdtesting.T) { tc.Run(t, &dnsSuite{}) }
func (*dnsSuite) TestParseResolvConfEmptyOrMissingPath(c *tc.C) {
	emptyPath := ""
	missingPath := filepath.Join(c.MkDir(), "missing")

	for _, path := range []string{emptyPath, missingPath} {
		result, err := network.ParseResolvConf(path)
		c.Check(err, tc.ErrorIsNil)
		c.Check(result, tc.IsNil)
	}
}

func (*dnsSuite) TestParseResolvConfNotReadablePath(c *tc.C) {
	unreadableConf := makeResolvConf(c, "#empty", 0000)
	result, err := network.ParseResolvConf(unreadableConf)
	expected := fmt.Sprintf("open %s: permission denied", unreadableConf)
	c.Check(err, tc.ErrorMatches, expected)
	c.Check(result, tc.IsNil)
}

func makeResolvConf(c *tc.C, content string, perms os.FileMode) string {
	fakeConfPath := filepath.Join(c.MkDir(), "fake")
	err := os.WriteFile(fakeConfPath, []byte(content), perms)
	c.Check(err, tc.ErrorIsNil)
	return fakeConfPath
}

func (*dnsSuite) TestParseResolvConfEmptyFile(c *tc.C) {
	emptyConf := makeResolvConf(c, "", 0644)
	result, err := network.ParseResolvConf(emptyConf)
	c.Check(err, tc.ErrorIsNil)
	// Expected non-nil, but empty result.
	c.Check(result, tc.DeepEquals, &network.DNSConfig{})
}

func (*dnsSuite) TestParseResolvConfCommentsAndWhitespaceHandling(c *tc.C) {
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, &network.DNSConfig{
		Nameservers:   []string{"8.8.8.8"},
		SearchDomains: []string{"foo", "example.com", "bar."},
	})
}

func (*dnsSuite) TestParseResolvConfSearchWithoutValue(c *tc.C) {
	badConf := makeResolvConf(c, "search # no value\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, tc.ErrorMatches, `parsing ".*", line 1: "search": required value\(s\) missing`)
	c.Check(result, tc.IsNil)
}

func (*dnsSuite) TestParseResolvConfNameserverWithoutValue(c *tc.C) {
	badConf := makeResolvConf(c, "nameserver", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, tc.ErrorMatches, `parsing ".*", line 1: "nameserver": required value\(s\) missing`)
	c.Check(result, tc.IsNil)
}

func (*dnsSuite) TestParseResolvConfValueFollowedByCommentWithoutWhitespace(c *tc.C) {
	badConf := makeResolvConf(c, "search foo bar#bad rest;is#ignored: still part of the comment", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, tc.ErrorMatches, `parsing ".*", line 1: "search": invalid value "bar#bad"`)
	c.Check(result, tc.IsNil)
}

func (*dnsSuite) TestParseResolvConfNameserverWithMultipleValues(c *tc.C) {
	badConf := makeResolvConf(c, "nameserver one two 42 ;;; comment still-inside-comment\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, tc.ErrorMatches, `parsing ".*", line 1: one value expected for "nameserver", got 3`)
	c.Check(result, tc.IsNil)
}

func (*dnsSuite) TestParseResolvConfLastSearchWins(c *tc.C) {
	const multiSearchConf = `
search zero five
search one
# this below overrides all of the above
search two three #comment ;also-comment still-comment
`
	fakeConf := makeResolvConf(c, multiSearchConf, 0644)
	result, err := network.ParseResolvConf(fakeConf)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, &network.DNSConfig{
		SearchDomains: []string{"two", "three"},
	})
}
