// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"regexp"

	"github.com/juju/packaging/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	coretesting "github.com/juju/juju/testing"
)

type configureSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&configureSuite{})

type testProvider struct {
	environs.CloudEnvironProvider
}

func init() {
	environs.RegisterProvider("sshinit_test", &testProvider{})
}

var aptgetRegexp = "(.|\n)*" + regexp.QuoteMeta("apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::Options::=--force-unsafe-io --assume-yes --quiet ")

func assertScriptMatches(c *gc.C, cfg cloudinit.CloudConfig, pattern string, match bool) {
	script, err := cfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)
	checker := gc.Matches
	if !match {
		checker = gc.Not(checker)
	}
	c.Assert(script, checker, pattern)
}

func (s *configureSuite) TestAptUpdate(c *gc.C) {
	// apt-get update is run only if AptUpdate is set.
	aptGetUpdatePattern := aptgetRegexp + "update(.|\n)*"
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.SystemUpdate(), jc.IsFalse)
	c.Assert(cfg.PackageSources(), gc.HasLen, 0)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, false)

	cfg.SetSystemUpdate(true)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, true)

	// If we add sources, but disable updates, display an error.
	cfg.SetSystemUpdate(false)
	source := packaging.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	_, err = cfg.RenderScript()
	c.Check(err, gc.ErrorMatches, "update sources were specified, but OS updates have been disabled.")
}

func (s *configureSuite) TestAptUpgrade(c *gc.C) {
	// apt-get upgrade is only run if AptUpgrade is set.
	aptGetUpgradePattern := aptgetRegexp + "upgrade(.|\n)*"
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	cfg.SetSystemUpdate(true)
	source := packaging.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, false)
	cfg.SetSystemUpgrade(true)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, true)
}

func (s *configureSuite) TestAptMirrorWrapper(c *gc.C) {
	expectedCommands := regexp.QuoteMeta(`
echo 'Changing apt mirror to "http://woat.com"' >&$JUJU_PROGRESS_FD
old_archive_mirror=$(awk "/^deb .* $(awk -F= '/DISTRIB_CODENAME=/ {gsub(/"/,""); print $2}' /etc/lsb-release) .*main.*\$/{print \$2;exit}" /etc/apt/sources.list)
new_archive_mirror=http://woat.com
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
new_security_mirror=http://woat.com
sed -i s,$old_security_mirror,$new_security_mirror, /etc/apt/sources.list
old_prefix=/var/lib/apt/lists/$(echo $old_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_security_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    if [ -f $old ]; then
      mv $old $new
    fi
done`)
	aptMirrorRegexp := "(.|\n)*" + expectedCommands + "(.|\n)*"
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	cfg.SetPackageMirror("http://woat.com")
	assertScriptMatches(c, cfg, aptMirrorRegexp, true)
}
