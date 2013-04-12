package main

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	EnvCommandBase
	vers        string
	Version     version.Number
	Development bool
	UploadTools bool
	Series      []string
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
	f.StringVar(&c.vers, "version", "", "version to upgrade to (defaults to highest available version with the current major version number)")
	f.BoolVar(&c.Development, "dev", false, "allow development versions to be chosen")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools")
	f.Var(seriesVar{&c.Series}, "series", "upload tools for supplied comma-separated series")
}

func (c *UpgradeJujuCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := version.Parse(c.vers)
		if err != nil {
			return err
		}
		if vers.Major != version.Current.Major {
			return fmt.Errorf("cannot upgrade to incompatible version")
		}
		c.Version = vers
	}
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	return cmd.CheckEmpty(args)
}

var errUpToDate = errors.New("no upgrades available")

// Run changes the version proposed for the juju tools.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer func() {
		if err == errUpToDate {
			log.Noticef("no upgrade available")
			err = nil
		}
	}()

	env := conn.Environ
	cfg, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	v, err := c.initVersions(cfg, env)
	if err != nil {
		return err
	}
	if c.UploadTools {
		series := getUploadSeries(cfg, c.Series)
		if err := v.uploadTools(env.Storage(), series); err != nil {
			return err
		}
	}
	if err := v.validate(c.Development); err != nil {
		return err
	}
	log.Infof("upgrade version chosen: %s", v.chosen)
	log.Infof("available tools: %s", v.tools)

	// Write updated config back to state if necessary. Note that this is
	// crackful and racy, because we have no idea what incompatible agent-
	// version might be set by another administrator in the meantime. If
	// this happens, tough: I'm not going to pretend to do it right when
	// I'm not.
	// TODO(fwereade): Do this right. Warning: scope unclear.
	// TODO(fwereade): I don't think Config.Development does anything useful.
	if v.chosen == v.agent && c.Development == cfg.Development() {
		return nil
	}
	if cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": v.chosen.String(),
		"development":   c.Development,
	}); err != nil {
		return err
	}
	if err := conn.State.SetEnvironConfig(cfg); err != nil {
		return err
	}
	log.Noticef("started upgrade to %s", v.chosen)
	return nil
}

func (c *UpgradeJujuCommand) initVersions(cfg *config.Config, env environs.Environ) (*upgradeVersions, error) {
	agent, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return nil, fmt.Errorf("incomplete environment configuration")
	}
	client := version.Current.Number
	available, err := environs.FindAvailableTools(env, client.Major)
	switch err {
	case nil, tools.ErrNoMatches, tools.ErrNoTools:
		if err != nil && !c.UploadTools {
			return nil, err
		}
	default:
		return nil, err
	}
	return &upgradeVersions{
		agent:  agent,
		client: client,
		chosen: c.Version,
		tools:  available,
	}, nil
}

type upgradeVersions struct {
	agent  version.Number
	client version.Number
	chosen version.Number
	tools  tools.List
}

func (v *upgradeVersions) uploadTools(storage environs.Storage, series []string) error {
	// TODO(fwereade): this is kinda crack: we should not assume that
	// version.Current matches whatever source happens to be built. The
	// ideal would be:
	//  1) compile jujud from $GOPATH into some build dir
	//  2) get actual version with `jujud version`
	//  3) check actual version for compatibility with CLI tools
	//  4) generate unique build version with reference to available tools
	//  5) force-version that unique version into the dir directly
	//  6) archive and upload the build dir
	// ...but there's no way we have time for that now. In the meantime,
	// considering the use cases, this should work well enough; but it
	// won't detect an incompatible major-version change, which is a shame.
	if v.chosen == version.Zero {
		v.chosen = v.client
	}
	v.chosen = uniqueVersion(v.chosen, v.tools)

	// TODO(fwereade): tools.Upload should return a tools.List, and should
	// include all the extra series we build, so we can set *that* onto
	// v.available and maybe one day be able to check that a given upgrade
	// won't leave out-of-date machines lying around.
	uploaded, err := uploadTools(storage, &v.chosen, series...)
	if err != nil {
		return err
	}
	v.tools = tools.List{uploaded}
	return nil
}

func (v *upgradeVersions) validate(dev bool) (err error) {
	// If not completely specified already, pick a single tools version.
	dev = dev || v.agent.IsDev() || v.client.IsDev() || v.chosen.IsDev()
	filter := tools.Filter{Number: v.chosen, Released: !dev}
	if v.tools, err = v.tools.Match(filter); err != nil {
		return err
	}
	v.tools, v.chosen = v.tools.Newest()
	if v.chosen == v.agent {
		return errUpToDate
	}

	// Major version upgrade
	if v.chosen.Major < v.agent.Major {
		// TODO(fwereade): I'm a bit concerned about old agent/CLI tools even
		// *connecting* to environments with higher agent-versions; but ofc they
		// have to connect in order to discover they shouldn't. However, once
		// any of our tools detect an incompatible version, they should act to
		// minimize damage: the CLI should abort politely, and the agents should
		// run an upgrader but no other tasks.
		return fmt.Errorf("cannot change major version from %d to %d", v.agent.Major, v.chosen.Major)
	} else if v.chosen.Major > v.agent.Major {
		return fmt.Errorf("major version upgrades are not supported yet")
	}

	return nil
}

// uniqueVersion returns a copy of the supplied version with a build number
// higher than any tools in existing that share its major, minor and patch.
func uniqueVersion(vers version.Number, existing tools.List) version.Number {
	for _, t := range existing {
		if t.Major != vers.Major || t.Minor != vers.Minor || t.Patch != vers.Patch {
			continue
		}
		if t.Build >= vers.Build {
			vers.Build = t.Build + 1
		}
	}
	return vers
}
