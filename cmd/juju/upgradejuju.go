package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	EnvName      string
	UploadTools  bool
	Version      version.Number
	Development   bool
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
	f.BoolVar(&c.Development, "dev", false, "allow development versions to be chosen")

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

// Run changes the version proposed for the juju tools.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) error {
	var err error
	c.conn, err = juju.NewConnFromName(c.EnvName)
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
	if c.Version == c.agentVersion && c.Development == cfg.Development() {
		return nil
	}
	return SetStateAgentVersion(c.conn.State, c.Version, c.Development)
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

// SetStateAgentVersion sets the current agent version and
// development flag in the state's environment configuration.
func SetStateAgentVersion(st *state.State, vers version.Number, development bool) error {
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
