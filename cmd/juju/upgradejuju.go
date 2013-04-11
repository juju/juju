package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	EnvCommandBase
	UploadTools  bool
	BumpVersion  bool
	Version      version.Number
	Development  bool
	conn         *juju.Conn
	toolsList    *environs.ToolsList
	agentVersion version.Number
	vers         string
}

var uploadTools = tools.Upload

func (c *UpgradeJujuCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-juju",
		Purpose: "upgrade the tools in a juju environment",
	}
}

func (c *UpgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools")
	f.StringVar(&c.vers, "version", "", "version to upgrade to (defaults to highest available version with the current major version number)")
	f.BoolVar(&c.BumpVersion, "bump-version", false, "upload the tools with a higher build number if necessary, and use that version (overrides --version)")
	f.BoolVar(&c.Development, "dev", false, "allow development versions to be chosen")
}

func (c *UpgradeJujuCommand) Init(args []string) error {
	if c.vers != "" {
		var err error
		c.Version, err = version.Parse(c.vers)
		if err != nil {
			return err
		}
		if c.Version.Major != version.Current.Major {
			return fmt.Errorf("cannot upgrade to incompatible version")
		}
		if c.Version == (version.Number{}) {
			return fmt.Errorf("cannot upgrade to version 0.0.0")
		}
	}
	return cmd.CheckEmpty(args)
}

// Run changes the version proposed for the juju tools.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) error {
	var err error
	c.conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.conn.Close()

	// Collect the current state of the environment.
	cfg, err := c.conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	var ok bool
	c.agentVersion, ok = cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return fmt.Errorf("incomplete environment configuration")
	}
	c.toolsList, err = environs.ListTools(c.conn.Environ, version.Current.Major)
	if err != nil {
		return err
	}

	// Determine what tools to upgrade to.
	if c.UploadTools {
		var forceVersion *version.Number
		if c.BumpVersion {
			vers := c.bumpedVersion()
			forceVersion = &vers.Number
		}
		// TODO(fwereade): we should split out building from uploading, so
		// we can tell when we're about to upload incompatible tools and
		// abort before we upload them.
		tools, err := uploadTools(c.conn.Environ.Storage(), forceVersion)
		if err != nil {
			return err
		}
		c.toolsList.Private = append(c.toolsList.Private, tools)
		c.Version = tools.Number
	} else if c.Version == (version.Number{}) {
		c.Version, err = c.newestVersion()
		if err != nil {
			return fmt.Errorf("cannot find newest version: %v", err)
		}
	} else {
		list := c.toolsList.Private
		if len(c.toolsList.Private) == 0 {
			list = c.toolsList.Public
		}
		if _, err := list.Match(tools.Filter{Number: c.Version}); err != nil {
			return err
		}
	}

	// Validate that the requested change is a good one, then make it.
	if c.Version.Major > c.agentVersion.Major {
		return fmt.Errorf("major version upgrades are not supported yet")
	} else if c.Version.Major < c.agentVersion.Major {
		// TODO(fwereade): I'm a bit concerned about old agent/CLI versions even
		// *connecting* to environments with higher agent-versions; but ofc they
		// have to connect in order to discover this information. However, once
		// any of our tools detect an incompatible version, they should act to
		// minimize damage: the CLI should abort politely, and the agents should
		// run an upgrader but no other tasks.
		return fmt.Errorf("cannot downgrade major version from %d to %d", c.agentVersion.Major, c.Version.Major)
	}
	if c.Version == c.agentVersion && c.Development == cfg.Development() {
		return nil
	}
	if err := c.checkVersion(); err != nil {
		return err
	}
	return SetAgentVersion(c.conn.State, c.Version, c.Development)
}

// newestVersion returns the newest version of any tool.
// Private tools take precedence over public tools.
func (c *UpgradeJujuCommand) newestVersion() (version.Number, error) {
	// When choosing a default version, don't choose
	// a dev version if the current version is a release version.
	allowDev := c.agentVersion.IsDev() || c.Development
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

// bumpedVersion returns the current version with a build version higher than
// any of the same version in the private tools storage.
func (c *UpgradeJujuCommand) bumpedVersion() version.Binary {
	vers := version.Current
	// We ignore the public tools because anything in the private
	// storage will override them.
	for _, t := range c.toolsList.Private {
		if t.Major != vers.Major || t.Minor != vers.Minor || t.Patch != vers.Patch {
			continue
		}
		if t.Build >= vers.Build {
			vers.Build = t.Build + 1
		}
	}
	return vers
}

// checkVersion returns an error if no available tools match the Version field.
// It assumes that tools have been chosen sensibly, and does not differentiate
// between tools in public and private storage.
func (c *UpgradeJujuCommand) checkVersion() error {
	list := append(c.toolsList.Private, c.toolsList.Public...)
	_, err := list.Match(tools.Filter{Number: c.Version})
	return err
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

// SetAgentVersion sets the current agent version and
// development flag in the state's environment configuration.
func SetAgentVersion(st *state.State, vers version.Number, development bool) error {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := cfg.AllAttrs()
	attrs["agent-version"] = vers.String()
	attrs["development"] = development
	cfg, err = config.New(attrs)
	if err != nil {
		panic(fmt.Errorf("config refused agent-version: %v", err))
	}
	return st.SetEnvironConfig(cfg)
}
