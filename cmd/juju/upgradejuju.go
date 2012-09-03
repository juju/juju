package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/juju"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	EnvName     string
	UploadTools bool
	BumpVersion bool
	Version version.Number
	Dev bool
	conn *juju.Conn
}

func (c *UpgradeJujuCommand) Info() *cmd.Info {
	return &cmd.Info{"upgrade-juju", "", "upgrade the tools in a juju environment", ""}
}

func (c *UpgradeJujuCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before upgrading")
	f.StringVar(&vers, "version", "", "version to upgrade to (defaults to highest available version with the current major version number)")
	f.BoolVar(&c.BumpVersion, "bump-version", false, "upload the tools as a higher version number if necessary")
	f.BoolVar(&c.Dev, "dev", false, "allow development versions to be chosen")

	if err := f.Parse(true, args); err != nil {
		return err
	}
	if vers != "" {
		var err error
		c.Version, err = version.Parse(vers)
		if err != nil {
			return err
		}
		if c.Version == version.Number{} {
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

	st, err := c.conn.State()
	if err != nil {
		return err
	}

	if c.Version == version.Number{} {
		c.Version, err = getLatestVersion(st)
		if err != nil {
			return err
		}
	}

	
	// We assume that the client is running the same major version
	// as the tools. Given that major-version upgrades require
	// extra logic, this seems OK for the time being.
	toolsList, err := environs.ListTools(c.conn.Environ, version.Current.Major)
	if err != nil {
		return err
	}
	if c.UploadTools {
		if err := c.uploadTools(toolsList); err != nil {
			return err
		}
	}
	// TODO first upgrade the provisioning agent, so that we know that any
	// new instances will be running the new tools.

	var agents []agentState
	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	for _, m := range machines {
		agents = append(agents, m)
	}

	// TODO units
	return c.upgrade(toolsList, agents)
}

func getLatestVersion(st *state.State) (version.Number, error) {
	currentVers := cfg.AgentVersion()
	
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}

// uploadTools uploads the current tools to the given environment.
// It adds the uploaded tools to the tools list.
func (c *UpgradeJujuCommand) uploadTools(toolsList *environs.ToolsList) error {
	var forceVersion *version.Binary
	if c.BumpVersion {
		vers := version.Current
		if max := highestToolsVersion(toolsList); max != nil {
			// Increment in units of 10000 so we can still
			// see the original version number in the least
			// significant digits of the bumped version
			// number (also vers.IsDev remains unaffected).
			for !max.Number.Less(vers.Number) {
				vers.Minor += 10000
				vers.Patch += 10000
				if vers.Minor < 0 || vers.Patch < 0 {
					panic("version number too large (probable DOS attack)")
				}
			}
		}
		if vers != version.Current {
			forceVersion = &vers
		}
	}
	tools, err := environs.PutTools(c.conn.Environ.Storage(), forceVersion)
	if err != nil {
		return err
	}
	toolsList.Private = append(toolsList.Private, tools)
	return nil
}

// highestToolsVersion returns the private tools with the highest
// version number, or nil if there are no private tools.
// We ignore the public tools because anything in the private
// storage will override them.
func highestToolsVersion(list *environs.ToolsList) *state.Tools {
	var max *state.Tools
	for _, t := range list.Private {
		if max == nil || max.Number.Less(t.Number) {
			max = t
		}
	}
	return max
}

func (c *UpgradeJujuCommand) upgrade(list *environs.ToolsList, agents []agentState) error {
	// TODO(rog) do the ProposeTools concurrently to avoid
	// many round-trips in strict sequence.
	for _, a := range agents {
		// We use the agent's proposed tools to work out
		// what series/architecture the agent is running on,
		// as the agent may not yet be running, so the	current
		// tools might not yet be set.
		proposedTools, err := a.ProposedAgentTools()
		if err != nil {
			return err
		}
		tools := environs.BestTools(list, proposedTools.Binary, c.Dev)
		if tools == nil {
			log.Printf("cannot find any tools appropriate for %s", agentName(a))
			continue
		}
		if proposedTools.Number.Less(tools.Number) {
			if err := a.ProposeAgentTools(tools); err != nil {
				return err
			}
			log.Printf("propose version for %s: %v", tools.Binary, agentName(a))
		} else {
			log.Printf("%s is already running its latest version: %v", agentName(a), tools.Binary)
		}
	}
	return nil
}

func agentName(a agentState) string {
	switch a := a.(type) {
	case *state.Machine:
		return fmt.Sprintf("machine %d", a.Id())
	}
	panic("unknown agent type")
}
