// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

func NewVersionCommand(version string, versionDetail interface{}) Command {
	return newVersionCommand(version, versionDetail)
}

func FormatCommand(command Command, super *SuperCommand, title bool, commandSeq []string) string {
	docCmd := &documentationCommand{super: super}
	ref := commandReference{command: command}
	return docCmd.formatCommand(ref, title, commandSeq)
}
