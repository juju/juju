// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// UpgradeJujuCommand upgrades the agents in a juju installation.
type UpgradeJujuCommand struct {
	envcmd.EnvCommandBase
	vers          string
	Version       version.Number
	UploadTools   bool
	DryRun        bool
	ResetPrevious bool
	AssumeYes     bool
	Series        []string
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

The upgrade-juju command will abort if an upgrade is already in
progress. It will also abort if a previous upgrade was partially
completed - this can happen if one of the state servers in a high
availability environment failed to upgrade. If a failed upgrade has
been resolved, the --reset-previous-upgrade flag can be used to reset
the environment's upgrade tracking state, allowing further upgrades.`

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
	f.BoolVar(&c.DryRun, "dry-run", false, "don't change anything, just report what would change")
	f.BoolVar(&c.ResetPrevious, "reset-previous-upgrade", false, "clear the previous (incomplete) upgrade status (use with care)")
	f.BoolVar(&c.AssumeYes, "y", false, "answer 'yes' to confirmation prompts")
	f.BoolVar(&c.AssumeYes, "yes", false, "")
	f.Var(newSeriesValue(nil, &c.Series), "series", "upload tools for supplied comma-separated series list (OBSOLETE)")
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

func formatTools(tools coretools.List) string {
	formatted := make([]string, len(tools))
	for i, tools := range tools {
		formatted[i] = fmt.Sprintf("    %s", tools.Version.String())
	}
	return strings.Join(formatted, "\n")
}

type upgradeJujuAPI interface {
	EnvironmentGet() (map[string]interface{}, error)
	FindTools(majorVersion, minorVersion int, series, arch string) (result params.FindToolsResult, err error)
	UploadTools(r io.Reader, vers version.Binary, additionalSeries ...string) (*coretools.Tools, error)
	AbortCurrentUpgrade() error
	SetEnvironAgentVersion(version version.Number) error
	Close() error
}

var getUpgradeJujuAPI = func(c *UpgradeJujuCommand) (upgradeJujuAPI, error) {
	return c.NewAPIClient()
}

// Run changes the version proposed for the juju envtools.
func (c *UpgradeJujuCommand) Run(ctx *cmd.Context) (err error) {
	if len(c.Series) > 0 {
		fmt.Fprintln(ctx.Stderr, "Use of --series is obsolete. --upload-tools now expands to all supported series of the same operating system.")
	}

	client, err := getUpgradeJujuAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	defer func() {
		if err == errUpToDate {
			ctx.Infof(err.Error())
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
	if c.UploadTools && !c.DryRun {
		if err := context.uploadTools(); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	if err := context.validate(); err != nil {
		return err
	}
	// TODO(fwereade): this list may be incomplete, pending envtools.Upload change.
	ctx.Infof("available tools:\n%s", formatTools(context.tools))
	ctx.Infof("best version:\n    %s", context.chosen)
	if c.DryRun {
		ctx.Infof("upgrade to this version by running\n    juju upgrade-juju --version=\"%s\"\n", context.chosen)
	} else {
		if c.ResetPrevious {
			if ok, err := c.confirmResetPreviousUpgrade(ctx); !ok || err != nil {
				const message = "previous upgrade not reset and no new upgrade triggered"
				if err != nil {
					return errors.Annotate(err, message)
				}
				return errors.New(message)
			}
			if err := client.AbortCurrentUpgrade(); err != nil {
				return block.ProcessBlockedError(err, block.BlockChange)
			}
		}
		if err := client.SetEnvironAgentVersion(context.chosen); err != nil {
			if params.IsCodeUpgradeInProgress(err) {
				return errors.Errorf("%s\n\n"+
					"Please wait for the upgrade to complete or if there was a problem with\n"+
					"the last upgrade that has been resolved, consider running the\n"+
					"upgrade-juju command with the --reset-previous-upgrade flag.", err,
				)
			} else {
				return block.ProcessBlockedError(err, block.BlockChange)
			}
		}
		logger.Infof("started upgrade to %s", context.chosen)
	}
	return nil
}

const resetPreviousUpgradeMessage = `
WARNING! using --reset-previous-upgrade when an upgrade is in progress
will cause the upgrade to fail. Only use this option to clear an
incomplete upgrade where the root cause has been resolved.

Continue [y/N]? `

func (c *UpgradeJujuCommand) confirmResetPreviousUpgrade(ctx *cmd.Context) (bool, error) {
	if c.AssumeYes {
		return true, nil
	}
	fmt.Fprintf(ctx.Stdout, resetPreviousUpgradeMessage)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(scanner.Text())
	return answer == "y" || answer == "yes", nil
}

// initVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available tools, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *UpgradeJujuCommand) initVersions(client upgradeJujuAPI, cfg *config.Config) (*upgradeContext, error) {
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
		tools:     findResult.List,
		apiClient: client,
		config:    cfg,
	}, nil
}

// upgradeContext holds the version information for making upgrade decisions.
type upgradeContext struct {
	agent     version.Number
	client    version.Number
	chosen    version.Number
	tools     coretools.List
	config    *config.Config
	apiClient upgradeJujuAPI
}

// uploadTools compiles jujud from $GOPATH and uploads it into the supplied
// storage. If no version has been explicitly chosen, the version number
// reported by the built tools will be based on the client version number.
// In any case, the version number reported will have a build component higher
// than that of any otherwise-matching available envtools.
// uploadTools resets the chosen version and replaces the available tools
// with the ones just uploaded.
func (context *upgradeContext) uploadTools() (err error) {
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

	builtTools, err := sync.BuildToolsTarball(&context.chosen, "upgrade")
	if err != nil {
		return err
	}
	defer os.RemoveAll(builtTools.Dir)

	var uploaded *coretools.Tools
	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof("uploading tools %v (%dkB) to Juju state server", builtTools.Version, (builtTools.Size+512)/1024)
	f, err := os.Open(toolsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	additionalSeries := version.OSSupportedSeries(builtTools.Version.OS)
	uploaded, err = context.apiClient.UploadTools(f, builtTools.Version, additionalSeries...)
	if err != nil {
		return err
	}
	context.tools = coretools.List{uploaded}
	return nil
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
		// major.minor+1.patch=0. If the CLI has a greater major version,
		// we just use the CLI version as is.
		nextVersion := context.agent
		if nextVersion.Major == context.client.Major {
			nextVersion.Minor += 1
			nextVersion.Patch = 0
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
		filter := coretools.Filter{Number: context.chosen}
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
