package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	EnvName      string
	UploadTools  bool
	BumpVersion  bool
	Version      version.Number
	DevVersion   bool
	conn         *juju.Conn
	toolsList    *environs.ToolsList
	agentVersion version.Number
}

var putTools = environs.PutTools

func (c *UpgradeJujuCommand) Info() *cmd.Info {
	return &cmd.Info{"upgrade-juju", "", "upgrade the tools in a juju environment", ""}
}

func (c *UpgradeJujuCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	var vers string
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools")
	f.StringVar(&vers, "version", "", "version to upgrade to (defaults to highest available version with the current major version number)")
	f.BoolVar(&c.BumpVersion, "bump-version", false, "upload the tools as a higher version number if necessary, and use that version (overrides --version)")
	f.BoolVar(&c.DevVersion, "dev", false, "allow development versions to be chosen")

	if err := f.Parse(true, args); err != nil {
		return err
	}
	if vers != "" {
		var err error
		c.Version, err = version.Parse(vers)
		if err != nil {
			return err
		}
		if c.Version == (version.Number{}) {
			return fmt.Errorf("cannot upgrade to version 0.0.0")
		}
	}

	return cmd.CheckEmpty(f.Args())
}

// Run changes the juju-managed firewall to expose any
// ports that were also explicitly marked by units as open.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) error {
	var err error
	c.conn, err = juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer c.conn.Close()

	cfg, err := c.conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	c.agentVersion = cfg.AgentVersion()
	c.toolsList, err = environs.ListTools(c.conn.Environ, c.agentVersion.Major)
	if err != nil {
		return err
	}
	if c.UploadTools {
		var forceVersion *version.Binary
		if c.BumpVersion {
			vers := c.bumpedVersion()
			forceVersion = &vers
		}
		tools, err := putTools(c.conn.Environ.Storage(), forceVersion)
		if err != nil {
			return err
		}
		c.toolsList.Private = append(c.toolsList.Private, tools)
	}
	if c.Version == (version.Number{}) {
		c.Version, err = c.newestVersion()
		if err != nil {
			return fmt.Errorf("cannot find newest version: %v", err)
		}
	}
	if c.Version.Major != c.agentVersion.Major {
		return fmt.Errorf("cannot upgrade major versions yet")
	}
	if c.Version == c.agentVersion && c.DevVersion == cfg.DevVersion() {
		return nil
	}
	return c.conn.State.SetAgentVersion(c.Version, c.DevVersion)
}

// newestVersion returns the newest version of any tool.
// Private tools take precedence over public tools.
func (c *UpgradeJujuCommand) newestVersion() (version.Number, error) {
	// When choosing a default version, don't choose
	// a dev version if the current version is a release version.
	allowDev := c.agentVersion.IsDev() || c.DevVersion
	max := c.highestVersion(c.toolsList.Private, allowDev)
	if max != nil {
		return max.Number, nil
	}
	max = c.highestVersion(c.toolsList.Public, allowDev)
	if max == nil {
		return version.Number{}, fmt.Errorf("no tools found")
	}
	return max.Number, nil
}

// bumpedVersion returns a version higher than anything in the private
// tools storage.
func (c *UpgradeJujuCommand) bumpedVersion() version.Binary {
	vers := version.Current
	// We ignore the public tools because anything in the private
	// storage will override them.
	max := c.highestVersion(c.toolsList.Private, true)
	if max == nil {
		return vers
	}
	// Increment in units of 10000 so we can still see the original
	// version number in the least significant digits of the bumped
	// version number (also vers.IsDev remains unaffected).
	vers.Minor = (max.Minor / 10000 * 10000) + (vers.Minor % 10000) + 10000
	vers.Patch = (max.Patch / 10000 * 10000) + (vers.Patch % 10000) + 10000
	return vers
}

// highestVersion returns the tools with the highest
// version number from the given list.
func (c *UpgradeJujuCommand) highestVersion(list []*state.Tools, allowDev bool) *state.Tools {
	var max *state.Tools
	for _, t := range list {
		if !allowDev && t.IsDev() {
			continue
		}
		if max == nil || max.Number.Less(t.Number) {
			max = t
		}
	}
	return max
}
