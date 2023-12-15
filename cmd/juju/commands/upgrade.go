// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/version/v2"
)

// baseUpgradeCommand is used by both the
// upgradeJujuCommand and upgradeControllerCommand
// to hold flags common to both.
type baseUpgradeCommand struct {
	AgentVersionParam string
	Version           version.Number
	DryRun            bool
	ResetPrevious     bool
	AssumeYes         bool
	AgentStream       string
	Timeout           time.Duration
	// IgnoreAgentVersions is used to allow an admin to request an agent
	// version without waiting for all agents to be at the right version.
	IgnoreAgentVersions bool

	rawArgs        []string
	upgradeMessage string

	modelConfigAPI   ModelConfigAPI
	modelUpgraderAPI ModelUpgraderAPI
}

func (c *baseUpgradeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.AgentVersionParam, "agent-version", "", "Upgrade to specific version")
	f.StringVar(&c.AgentStream, "agent-stream", "", "Check this agent stream for upgrades")
	f.BoolVar(&c.DryRun, "dry-run", false, "Don't change anything, just report what would be changed")
	f.BoolVar(&c.ResetPrevious, "reset-previous-upgrade", false, "Clear the previous (incomplete) upgrade status (use with care)")
	f.BoolVar(&c.AssumeYes, "y", false, "Answer 'yes' to confirmation prompts")
	f.BoolVar(&c.AssumeYes, "yes", false, "")
	f.BoolVar(&c.IgnoreAgentVersions, "ignore-agent-versions", false,
		"Don't check if all agents have already reached the current version")
	f.DurationVar(&c.Timeout, "timeout", 10*time.Minute, "Timeout before upgrade is aborted")
}

func (c *baseUpgradeCommand) Init(args []string) error {
	c.rawArgs = args
	if c.upgradeMessage == "" {
		c.upgradeMessage = "upgrade to this version by running\n    juju upgrade-model"
	}
	return cmd.CheckEmpty(args)
}

const resetPreviousUpgradeMessage = `
WARNING! using --reset-previous-upgrade when an upgrade is in progress
will cause the upgrade to fail. Only use this option to clear an
incomplete upgrade where the root cause has been resolved.

Continue [y/N]? `

func (c *baseUpgradeCommand) confirmResetPreviousUpgrade(ctx *cmd.Context) (bool, error) {
	if c.AssumeYes {
		return true, nil
	}
	fmt.Fprint(ctx.Stdout, resetPreviousUpgradeMessage)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(scanner.Text())
	return answer == "y" || answer == "yes", nil
}
