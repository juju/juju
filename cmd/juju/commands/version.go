// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"

	coreos "github.com/juju/juju/core/os"
	jujuversion "github.com/juju/juju/version"
)

const versionDoc = `
Print only the Juju CLI client version.

To see the version of Juju running on a particular controller, use
  juju show-controller

To see the version of Juju running on a particular model, use
  juju show-model

See also:
    show-controller
    show-model`

// versionDetail is populated with version information from juju/juju/cmd
// and passed into each SuperCommand. It can be printed using `juju version --all`.
type versionDetail struct {
	// Version of the current binary.
	Version version.Binary `json:"version" yaml:"version"`
	// GitCommit of tree used to build the binary.
	GitCommit string `json:"git-commit,omitempty" yaml:"git-commit,omitempty"`
	// GitTreeState is "clean" if the working copy used to build the binary had no
	// uncommitted changes or untracked files, otherwise "dirty".
	GitTreeState string `json:"git-tree-state,omitempty" yaml:"git-tree-state,omitempty"`
	// Compiler reported by runtime.Compiler
	Compiler string `json:"compiler" yaml:"compiler"`
	// OfficialBuild is a monotonic integer set by Jenkins.
	OfficialBuild int `json:"official-build,omitempty" yaml:"official-build,omitempty"`
}

// versionCommand is a cmd.Command that prints the current version.
type versionCommand struct {
	cmd.CommandBase
	out           cmd.Output
	version       version.Binary
	versionDetail interface{}

	showAll bool
}

func newVersionCommand() *versionCommand {
	return &versionCommand{}
}

func (v *versionCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "version",
		Purpose: "Print the Juju CLI client version.",
		Doc:     versionDoc,
	}
}

func (v *versionCommand) SetFlags(f *gnuflag.FlagSet) {
	formatters := make(map[string]cmd.Formatter, len(cmd.DefaultFormatters))
	for k, v := range cmd.DefaultFormatters {
		formatters[k] = v.Formatter
	}
	v.out.AddFlags(f, "smart", formatters)
	f.BoolVar(&v.showAll, "all", false, "Prints all version information")
}

func (v *versionCommand) Init(args []string) error {
	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	detail := versionDetail{
		Version:       current,
		GitCommit:     jujuversion.GitCommit,
		GitTreeState:  jujuversion.GitTreeState,
		Compiler:      jujuversion.Compiler,
		OfficialBuild: jujuversion.OfficialBuild,
	}

	v.version = detail.Version
	v.versionDetail = detail

	return v.CommandBase.Init(args)
}

func (v *versionCommand) Run(ctxt *cmd.Context) error {
	if v.showAll {
		return v.out.Write(ctxt, v.versionDetail)
	}
	return v.out.Write(ctxt, v.version)
}
