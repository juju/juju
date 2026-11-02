// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"path/filepath"

	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/modelcmd"
	internalrecovery "github.com/juju/juju/internal/recovery"
)

func newRecoveryCommand() cmd.Command {
	c := &recoveryCommand{}
	c.CanClearCurrentModel = true
	return modelcmd.Wrap(c, modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel)
}

type recoveryCommand struct {
	modelcmd.ModelCommandBase
	archivePath   string
	assertionPath string
}

func (c *recoveryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "recover", Args: "<backup-file> <assertion-file>",
		Purpose: "Validate a controller recovery archive.",
		Doc:     "Validate an archive against its unsigned SHA-256 assertion. Controller recovery provisioning is not available in this build.",
		SeeAlso: []string{"bootstrap", "create-backup", "unregister"},
	})
}

func (c *recoveryCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("a backup archive is required")
	}
	if len(args) == 1 {
		return errors.New("a backup assertion file is required")
	}
	if err := cmd.CheckEmpty(args[2:]); err != nil {
		return err
	}
	if args[0] == "" {
		return errors.New("a backup archive is required")
	}
	if args[1] == "" {
		return errors.New("a backup assertion file is required")
	}
	c.archivePath = args[0]
	c.assertionPath = args[1]
	return nil
}

func (c *recoveryCommand) Run(ctx *cmd.Context) error {
	archivePath := c.archivePath
	if !filepath.IsAbs(archivePath) {
		archivePath = ctx.AbsPath(archivePath)
	}
	assertionPath := c.assertionPath
	if !filepath.IsAbs(assertionPath) {
		assertionPath = ctx.AbsPath(assertionPath)
	}
	assertion, err := c.readAssertion(assertionPath)
	if err != nil {
		return err
	}
	if _, err := internalrecovery.ValidateArchive(ctx.Context, logger, archivePath, assertion.Checksum); err != nil {
		return errors.Annotate(err, "invalid recovery archive")
	}
	return errors.New("controller recovery provisioning is not available in this build")
}

// readAssertion opens and parses the backup assertion file. Every open,
// read and close failure carries the assertion filename; a parse error
// wins over a close failure.
func (c *recoveryCommand) readAssertion(assertionPath string) (*backups.Assertion, error) {
	file, err := c.Filesystem().Open(assertionPath)
	if err != nil {
		return nil, errors.Annotatef(err, "reading backup assertion %q", assertionPath)
	}
	assertion, err := backups.ReadAssertion(file)
	closeErr := file.Close()
	if err != nil {
		return nil, errors.Annotatef(err, "reading backup assertion %q", assertionPath)
	}
	if closeErr != nil {
		return nil, errors.Annotatef(closeErr, "reading backup assertion %q", assertionPath)
	}
	return assertion, nil
}
