// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	stderrors "errors"
	"fmt"
	"os"
	"path"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/sync"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	envcmd.EnvCommandBase
	vers        string
	Version     version.Number
	UploadTools bool
	Series      []string
}

var upgradeJujuDoc = `
The upgrade-juju command upgrades a running environment by setting a version
number for all juju agents to run. By default, it chooses the most recent
supported version compatible with the command-line tools version.

A development version is defined to be any version with an odd minor
version or a nonzero build component (for example version 2.1.1, 3.3.0
and 2.0.0.1 are development versions; 2.0.3 and 3.4.1 are not). A
development version may be chosen in two cases:

 - when the current agent version is a development one and there is
   a more recent version available with the same major.minor numbers;
 - when an explicit --version major.minor is given (e.g. --version 1.17,
   or 1.17.2, but not just 1)

For development use, the --upload-tools flag specifies that the juju tools will
packaged (or compiled locally, if no jujud binaries exists, for which you will
need the golang packages installed) and uploaded before the version is set.
Currently the tools will be uploaded as if they had the version of the current
juju tool, unless specified otherwise by the --version flag.

When run without arguments. upgrade-juju will try to upgrade to the
following versions, in order of preference, depending on the current
value of the environment's agent-version setting:

 - The highest patch.build version of the *next* stable major.minor version.
 - The highest patch.build version of the *current* major.minor version.

Both of these depend on tools availability, which some situations (no
outgoing internet access) and provider types (such as maas) require that
you manage yourself; see the documentation for "sync-tools".
`

func (c *UpgradeJujuCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-juju",
		Purpose: "upgrade the tools in a juju environment",
		Doc:     upgradeJujuDoc,
	}
}

func (c *UpgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.vers, "version", "", "upgrade to specific version")
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools")
	f.Var(newSeriesValue(nil, &c.Series), "series", "upload tools for supplied comma-separated series list")
}

func (c *UpgradeJujuCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := version.Parse(c.vers)
		if err != nil {
			return err
		}
		if vers.Major != version.Current.Major {
			return fmt.Errorf("cannot upgrade to version incompatible with CLI")
		}
		if c.UploadTools && vers.Build != 0 {
			// TODO(fwereade): when we start taking versions from actual built
			// code, we should disable --version when used with --upload-tools.
			// For now, it's the only way to experiment with version upgrade
			// behaviour live, so the only restriction is that Build cannot
			// be used (because its value needs to be chosen internally so as
			// not to collide with existing tools).
			return fmt.Errorf("cannot specify build number when uploading tools")
		}
		c.Version = vers
	}
	if len(c.Series) > 0 && !c.UploadTools {
		return fmt.Errorf("--series requires --upload-tools")
	}
	return cmd.CheckEmpty(args)
}

var errUpToDate = stderrors.New("no upgrades available")

// Run changes the version proposed for the juju envtools.
func (c *UpgradeJujuCommand) Run(_ *cmd.Context) (err error) {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	defer func() {
		if err == errUpToDate {
			logger.Infof(err.Error())
			err = nil
		}
	}()

	// Determine the version to upgrade to, uploading tools if necessary.
	attrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return err
	}
	context, err := c.initVersions(client, cfg)
	if err != nil {
		return err
	}
	if c.UploadTools {
		series := bootstrap.SeriesToUpload(cfg, c.Series)
		if err := context.uploadTools(series); err != nil {
			return err
		}
	}
	if err := context.validate(); err != nil {
		return err
	}
	logger.Infof("upgrade version chosen: %s", context.chosen)
	// TODO(fwereade): this list may be incomplete, pending envtools.Upload change.
	logger.Infof("available tools: %s", context.tools)

	if err := client.SetEnvironAgentVersion(context.chosen); err != nil {
		return err
	}
	logger.Infof("started upgrade to %s", context.chosen)
	return nil
}

// initVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available tools, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *UpgradeJujuCommand) initVersions(client *api.Client, cfg *config.Config) (*upgradeContext, error) {
	agent, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return nil, fmt.Errorf("incomplete environment configuration")
	}
	if c.Version == agent {
		return nil, errUpToDate
	}
	clientVersion := version.Current.Number
	findResult, err := client.FindTools(clientVersion.Major, -1, "", "")
	var availableTools coretools.List
	if params.IsCodeNotImplemented(err) {
		availableTools, err = findTools1dot17(cfg)
	} else {
		availableTools = findResult.List
	}
	if err != nil {
		return nil, err
	}
	err = findResult.Error
	if findResult.Error != nil {
		if !params.IsCodeNotFound(err) {
			return nil, err
		}
		if !c.UploadTools {
			// No tools found and we shouldn't upload any, so if we are not asking for a
			// major upgrade, pretend there is no more recent version available.
			if c.Version == version.Zero && agent.Major == clientVersion.Major {
				return nil, errUpToDate
			}
			return nil, err
		}
	}
	return &upgradeContext{
		agent:     agent,
		client:    clientVersion,
		chosen:    c.Version,
		tools:     availableTools,
		apiClient: client,
		config:    cfg,
	}, nil
}

// findTools1dot17 allows 1.17.x versions to be upgraded.
func findTools1dot17(cfg *config.Config) (coretools.List, error) {
	logger.Warningf("running find tools in 1.17 compatibility mode")
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	clientVersion := version.Current.Number
	return envtools.FindTools(env, clientVersion.Major, -1, coretools.Filter{}, envtools.DoNotAllowRetry)
}

// upgradeContext holds the version information for making upgrade decisions.
type upgradeContext struct {
	agent     version.Number
	client    version.Number
	chosen    version.Number
	tools     coretools.List
	config    *config.Config
	apiClient *api.Client
}

// uploadTools compiles jujud from $GOPATH and uploads it into the supplied
// storage. If no version has been explicitly chosen, the version number
// reported by the built tools will be based on the client version number.
// In any case, the version number reported will have a build component higher
// than that of any otherwise-matching available envtools.
// uploadTools resets the chosen version and replaces the available tools
// with the ones just uploaded.
func (context *upgradeContext) uploadTools(series []string) (err error) {
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
	if context.chosen == version.Zero {
		context.chosen = context.client
	}
	context.chosen = uploadVersion(context.chosen, context.tools)

	builtTools, err := sync.BuildToolsTarball(&context.chosen)
	if err != nil {
		return err
	}
	defer os.RemoveAll(builtTools.Dir)

	var uploaded *coretools.Tools
	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof("uploading tools %v (%dkB) to Juju state server", builtTools.Version, (builtTools.Size+512)/1024)
	uploaded, err = context.apiClient.UploadTools(toolsPath, builtTools.Version, series...)
	if params.IsCodeNotImplemented(err) {
		uploaded, err = context.uploadTools1dot17(builtTools, series...)
	}
	if err != nil {
		return err
	}
	context.tools = coretools.List{uploaded}
	return nil
}

func (context *upgradeContext) uploadTools1dot17(builtTools *sync.BuiltTools,
	series ...string) (*coretools.Tools, error) {

	logger.Warningf("running upload tools in 1.17 compatibility mode")
	env, err := environs.New(context.config)
	if err != nil {
		return nil, err
	}
	return sync.SyncBuiltTools(env.Storage(), builtTools, series...)
}

// validate chooses an upgrade version, if one has not already been chosen,
// and ensures the tools list contains no entries that do not have that version.
// If validate returns no error, the environment agent-version can be set to
// the value of the chosen field.
func (context *upgradeContext) validate() (err error) {
	if context.chosen == version.Zero {
		// No explicitly specified version, so find the version to which we
		// need to upgrade. If the CLI and agent major versions match, we find
		// next available stable release to upgrade to by incrementing the
		// minor version, starting from the current agent version and doing
		// major.minor+1 or +2 as needed. If the CLI has a greater major version,
		// we just use the CLI version as is.
		nextVersion := context.agent
		if nextVersion.Major == context.client.Major {
			if context.agent.IsDev() {
				nextVersion.Minor += 1
			} else {
				nextVersion.Minor += 2
			}
		} else {
			nextVersion = context.client
		}

		newestNextStable, found := context.tools.NewestCompatible(nextVersion)
		if found {
			logger.Debugf("found a more recent stable version %s", newestNextStable)
			context.chosen = newestNextStable
		} else {
			newestCurrent, found := context.tools.NewestCompatible(context.agent)
			if found {
				logger.Debugf("found more recent current version %s", newestCurrent)
				context.chosen = newestCurrent
			} else {
				if context.agent.Major != context.client.Major {
					return fmt.Errorf("no compatible tools available")
				} else {
					return fmt.Errorf("no more recent supported versions available")
				}
			}
		}
	} else {
		// If not completely specified already, pick a single tools version.
		filter := coretools.Filter{Number: context.chosen, Released: !context.chosen.IsDev()}
		if context.tools, err = context.tools.Match(filter); err != nil {
			return err
		}
		context.chosen, context.tools = context.tools.Newest()
	}
	if context.chosen == context.agent {
		return errUpToDate
	}

	// Disallow major.minor version downgrades.
	if context.chosen.Major < context.agent.Major ||
		context.chosen.Major == context.agent.Major && context.chosen.Minor < context.agent.Minor {
		// TODO(fwereade): I'm a bit concerned about old agent/CLI tools even
		// *connecting* to environments with higher agent-versions; but ofc they
		// have to connect in order to discover they shouldn't. However, once
		// any of our tools detect an incompatible version, they should act to
		// minimize damage: the CLI should abort politely, and the agents should
		// run an Upgrader but no other tasks.
		return fmt.Errorf("cannot change version from %s to %s", context.agent, context.chosen)
	}

	return nil
}

// uploadVersion returns a copy of the supplied version with a build number
// higher than any of the supplied tools that share its major, minor and patch.
func uploadVersion(vers version.Number, existing coretools.List) version.Number {
	vers.Build++
	for _, t := range existing {
		if t.Version.Major != vers.Major || t.Version.Minor != vers.Minor || t.Version.Patch != vers.Patch {
			continue
		}
		if t.Version.Build >= vers.Build {
			vers.Build = t.Version.Build + 1
		}
	}
	return vers
}
