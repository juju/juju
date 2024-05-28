// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	"strings"

	"github.com/juju/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = gc.Suite(&AptSuite{})

type AptSuite struct {
	paccmder commands.PackageCommander
}

func (s *AptSuite) SetUpSuite(c *gc.C) {
	s.paccmder = commands.NewAptPackageCommander()
}

func (s *AptSuite) TestProxyConfigContentsEmpty(c *gc.C) {
	out := s.paccmder.ProxyConfigContents(proxy.Settings{})
	c.Assert(out, gc.Equals, "")
}

func (s *AptSuite) TestProxyConfigContentsPartial(c *gc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, "Acquire::http::Proxy \"dat-proxy.zone:8080\";")
}

func (s *AptSuite) TestProxyConfigContentsFull(c *gc.C) {
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

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, expected)
}

func (s *AptSuite) TestSetMirrorCommands(c *gc.C) {
	expected := `
old_archive_mirror=$(awk "/^deb .* $(awk -F= '/DISTRIB_CODENAME=/ {gsub(/"/,""); print $2}' /etc/lsb-release) .*main.*\$/{print \$2;exit}" /etc/apt/sources.list)
new_archive_mirror=http://mirror
sed -i s,$old_archive_mirror,$new_archive_mirror, /etc/apt/sources.list
old_prefix=/var/lib/apt/lists/$(echo $old_archive_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_archive_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done
old_security_mirror=$(awk "/^deb .* $(awk -F= '/DISTRIB_CODENAME=/ {gsub(/"/,""); print $2}' /etc/lsb-release)-security .*main.*\$/{print \$2;exit}" /etc/apt/sources.list)
new_security_mirror=http://security-mirror
sed -i s,$old_security_mirror,$new_security_mirror, /etc/apt/sources.list
old_prefix=/var/lib/apt/lists/$(echo $old_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done`[1:]
	cmds := s.paccmder.SetMirrorCommands("http://mirror", "http://security-mirror")
	output := strings.Join(cmds, "\n")
	c.Assert(output, gc.Equals, expected)
}
