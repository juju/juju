// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/gnuflag"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd"
)

const versionDoc = `
Print only the Juju CLI client version.`

const versionExamplesDoc = `
    juju version

Print all version information:

    juju version --all
`

// versionDetail is populated with version information from juju/juju/cmd
// and passed into each SuperCommand. It can be printed using `juju version --all`.
type versionDetail struct {
	// Version of the current binary.
	Version semversion.Binary `json:"version" yaml:"version"`
	// GitCommit of tree used to build the binary.
	GitCommit string `json:"git-commit,omitempty" yaml:"git-commit,omitempty"`
	// GitTreeState is "clean" if the working copy used to build the binary had no
	// uncommitted changes or untracked files, otherwise "dirty".
	GitTreeState string `json:"git-tree-state,omitempty" yaml:"git-tree-state,omitempty"`
	// Compiler reported by runtime.Compiler
	Compiler string `json:"compiler" yaml:"compiler"`
	// OfficialBuildNumber is a monotonic integer set by Jenkins.
	OfficialBuildNumber int `json:"official-build-number,omitempty" yaml:"official-build-number,omitempty"`
	// Official is true if this is an official build.
	Official bool `json:"official" yaml:"official"`
	// Grade reflects the snap grade value.
	Grade string `json:"grade,omitempty" yaml:"grade,omitempty"`
	// GoBuildTags is the build tags used to build the binary.
	GoBuildTags string `json:"go-build-tags,omitempty" yaml:"go-build-tags,omitempty"`
}

// versionCommand is a cmd.Command that prints the current version.
type versionCommand struct {
	cmd.CommandBase
	out           cmd.Output
	version       semversion.Binary
	versionDetail interface{}

	showAll bool
}

func newVersionCommand() *versionCommand {
	return &versionCommand{}
}

func (v *versionCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:     "version",
		Purpose:  "Print the Juju CLI client version.",
		Doc:      versionDoc,
		Examples: versionExamplesDoc,
		SeeAlso: []string{
			"show-controller",
			"show-model",
		},
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
	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	detail := versionDetail{
		Version:             current,
		GitCommit:           jujuversion.GitCommit,
		GitTreeState:        jujuversion.GitTreeState,
		Compiler:            jujuversion.Compiler,
		GoBuildTags:         jujuversion.GoBuildTags,
		OfficialBuildNumber: jujuversion.OfficialBuild,
		Official:            isOfficialClient(),
		Grade:               jujuversion.Grade,
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
