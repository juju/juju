// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	"strings"

	"github.com/juju/proxy"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = tc.Suite(&AptSuite{})

type AptSuite struct {
	aptCommander commands.AptPackageCommander
}

func (s *AptSuite) SetUpSuite(c *tc.C) {
	s.aptCommander = commands.NewAptPackageCommander()
}

func (s *AptSuite) TestProxyConfigContentsEmpty(c *tc.C) {
	out := s.aptCommander.ProxyConfigContents(proxy.Settings{})
	c.Assert(out, tc.Equals, "")
}

func (s *AptSuite) TestProxyConfigContentsPartial(c *tc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output := s.aptCommander.ProxyConfigContents(sets)
	c.Assert(output, tc.Equals, "Acquire::http::Proxy \"dat-proxy.zone:8080\";")
}

func (s *AptSuite) TestProxyConfigContentsFull(c *tc.C) {
	sets := proxy.Settings{
		Http:    "dat-proxy.zone:8080",
		Https:   "https://much-security.com",
		Ftp:     "gimme-files.zone",
		NoProxy: "local1,local2",
	}
	expected := `Acquire::http::Proxy "dat-proxy.zone:8080";
Acquire::https::Proxy "https://much-security.com";
Acquire::ftp::Proxy "gimme-files.zone";
Acquire::http::Proxy::"local1" "DIRECT";
Acquire::https::Proxy::"local1" "DIRECT";
Acquire::ftp::Proxy::"local1" "DIRECT";
Acquire::http::Proxy::"local2" "DIRECT";
Acquire::https::Proxy::"local2" "DIRECT";
Acquire::ftp::Proxy::"local2" "DIRECT";`

	output := s.aptCommander.ProxyConfigContents(sets)
	c.Assert(output, tc.Equals, expected)
}

func (s *AptSuite) TestSetMirrorCommands(c *tc.C) {
	expected := `
old_archive_mirror=$(apt-cache policy | grep http | awk '{ $1="" ; print }' | sed 's/^ //g'  | grep "$(lsb_release -c -s)/main" | awk '{print $1; exit}')
new_archive_mirror="http://mirror"
[ -f "/etc/apt/sources.list" ] && sed -i s,$old_archive_mirror,$new_archive_mirror, "/etc/apt/sources.list"
[ -f "/etc/apt/sources.list.d/ubuntu.sources" ] && sed -i s,$old_archive_mirror,$new_archive_mirror, "/etc/apt/sources.list.d/ubuntu.sources"
old_prefix=/var/lib/apt/lists/$(echo $old_archive_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_archive_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done
old_security_mirror=$(apt-cache policy | grep http | awk '{ $1="" ; print }' | sed 's/^ //g'  | grep "$(lsb_release -c -s)-security/main" | awk '{print $1; exit}')
new_security_mirror="http://security-mirror"
[ -f "/etc/apt/sources.list" ] && sed -i s,$old_security_mirror,$new_security_mirror, "/etc/apt/sources.list"
[ -f "/etc/apt/sources.list.d/ubuntu.sources" ] && sed -i s,$old_security_mirror,$new_security_mirror, "/etc/apt/sources.list.d/ubuntu.sources"
old_prefix=/var/lib/apt/lists/$(echo $old_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done`[1:]
	cmds := s.aptCommander.SetMirrorCommands("http://mirror", "http://security-mirror")
	output := strings.Join(cmds, "\n")
	c.Assert(output, tc.Equals, expected)
}
