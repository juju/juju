// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/juju/api/jujuclient"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
	internalrecovery "github.com/juju/juju/internal/recovery"
)

type recoveryProvisioner func(*cmd.Context, jujuclient.ClientStore, *recovery.ArchiveInfo) error

func newRecoveryCommand() cmd.Command {
	return newRecoveryCommandWithProvisioner(provisionRecovery)
}

func newRecoveryCommandWithProvisioner(provision recoveryProvisioner) cmd.Command {
	c := &recoveryCommand{provision: provision}
	c.CanClearCurrentModel = true
	return modelcmd.Wrap(c, modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel)
}

type recoveryCommand struct {
	modelcmd.ModelCommandBase
	archivePath   string
	assertionPath string
	provision     recoveryProvisioner
}

func (c *recoveryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "recover", Args: "<backup-file> <assertion-file>",
		Purpose: "Allocate a replacement controller from a recovery archive.",
		Doc: "Validate an archive against its unsigned SHA-256 assertion and allocate a replacement controller machine. " +
			"Agent installation and controller initialisation are not available in this build. " +
			"A successful allocation returns the replacement provider instance ID in an initialisation-unavailable error; " +
			"the machine is left for inspection and the controller is not registered.",
		SeeAlso: []string{"bootstrap", "create-backup", "unregister"},
	})
}

func (c *recoveryCommand) Init(args []string) error {
	switch {
	case len(args) == 0 || args[0] == "":
		return errors.New("a backup archive is required")
	case len(args) == 1 || args[1] == "":
		return errors.New("a backup assertion file is required")
	}
	if err := cmd.CheckEmpty(args[2:]); err != nil {
		return err
	}
	c.archivePath = args[0]
	c.assertionPath = args[1]
	return nil
}

func (c *recoveryCommand) Run(ctx *cmd.Context) error {
	archivePath := ctx.AbsPath(c.archivePath)
	assertionPath := ctx.AbsPath(c.assertionPath)
	assertion, err := c.readAssertion(assertionPath)
	if err != nil {
		return err
	}
	info, err := internalrecovery.ValidateArchive(ctx.Context, logger, archivePath, assertion.Checksum)
	if err != nil {
		return errors.Errorf("invalid recovery archive: %w", err)
	}
	return c.provision(ctx, c.ClientStore(), info)
}

// readAssertion opens and parses the backup assertion file. Every open,
// read and close failure carries the assertion filename; a parse error
// wins over a close failure.
func (c *recoveryCommand) readAssertion(assertionPath string) (*backups.Assertion, error) {
	file, err := c.Filesystem().Open(assertionPath)
	if err != nil {
		return nil, errors.Errorf("reading backup assertion %q: %w", assertionPath, err)
	}
	assertion, err := backups.ReadAssertion(file)
	closeErr := file.Close()
	if err != nil {
		return nil, errors.Errorf("reading backup assertion %q: %w", assertionPath, err)
	}
	if closeErr != nil {
		return nil, errors.Errorf("reading backup assertion %q: %w", assertionPath, closeErr)
	}
	return assertion, nil
}
