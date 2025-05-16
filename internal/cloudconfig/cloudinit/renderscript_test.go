// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"regexp"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/packaging/source"
	coretesting "github.com/juju/juju/internal/testing"
)

type configureSuite struct {
	coretesting.BaseSuite
}

func TestConfigureSuite(t *stdtesting.T) { tc.Run(t, &configureSuite{}) }

type testProvider struct {
	environs.CloudEnvironProvider
}

func init() {
	environs.RegisterProvider("sshinit_test", &testProvider{})
}

var aptgetRegexp = "(.|\n)*" + regexp.QuoteMeta("apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::Options::=--force-unsafe-io --assume-yes --quiet ")

func assertScriptMatches(c *tc.C, cfg cloudinit.CloudConfig, pattern string, match bool) {
	script, err := cfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)
	checker := tc.Matches
	if !match {
		checker = tc.Not(checker)
	}
	c.Assert(script, checker, pattern)
}

func assertScriptContains(c *tc.C, cfg cloudinit.CloudConfig, substring string) {
	script, err := cfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(script, tc.Contains, substring)
}

func (s *configureSuite) TestAptUpdate(c *tc.C) {
	// apt-get update is run only if AptUpdate is set.
	aptGetUpdatePattern := aptgetRegexp + "update(.|\n)*"
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.SystemUpdate(), tc.IsFalse)
	c.Assert(cfg.PackageSources(), tc.HasLen, 0)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, false)

	cfg.SetSystemUpdate(true)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, true)

	// If we add sources, but disable updates, display an error.
	cfg.SetSystemUpdate(false)
	source := source.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	_, err = cfg.RenderScript()
	c.Check(err, tc.ErrorMatches, "update sources were specified, but OS updates have been disabled.")
}

func (s *configureSuite) TestAptUpgrade(c *tc.C) {
	// apt-get upgrade is only run if AptUpgrade is set.
	aptGetUpgradePattern := aptgetRegexp + "upgrade(.|\n)*"
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	cfg.SetSystemUpdate(true)
	source := source.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, false)
	cfg.SetSystemUpgrade(true)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, true)
}

func (s *configureSuite) TestAptMirrorWrapper(c *tc.C) {
	expectedCommands := `
echo 'Changing apt mirror to "http://woat.com"' >&$JUJU_PROGRESS_FD
old_archive_mirror=$(apt-cache policy | grep http | awk '{ $1="" ; print }' | sed 's/^ //g'  | grep "$(lsb_release -c -s)/main" | awk '{print $1; exit}')
new_archive_mirror="http://woat.com"
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
new_security_mirror="http://woat.com"
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
done`
	cfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	cfg.SetPackageMirror("http://woat.com")
	assertScriptContains(c, cfg, expectedCommands)
}
