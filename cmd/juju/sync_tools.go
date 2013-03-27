package main

import (
	//"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket
type SyncToolsCommand struct {
	EnvCommandBase
	sourceToolsList *environs.ToolsList
	targetToolsList *environs.ToolsList
	allVersions     bool
}

var _ cmd.Command = (*SyncToolsCommand)(nil)

func (c *SyncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "copy tools from another public bucket",
	}
}

func (c *SyncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "instead of copying only the newest, copy all versions")

}

func (c *SyncToolsCommand) Init(args []string) error {
	return nil
}

var officialBucketAttrs = map[string]interface{}{
	"name":           "juju-public",
	"type":           "ec2",
	"control-bucket": "juju-dist",
	"access-key":     "",
	"secret-key":     "",
}

// Find the set of tools at the 'newest' version
func findNewest(fullTools []*state.Tools) []*state.Tools {
	var curBest *state.Tools = nil
	var res []*state.Tools = nil
	for _, t := range fullTools {
		var add = false
		if curBest == nil || curBest.Number.Less(t.Number) {
			// This best is clearly better than all existing
			// entries, so reset the list
			res = make([]*state.Tools, 0, 1)
			add = true
			curBest = t
		}
		if curBest.Number == t.Number {
			add = true
		}
		if add {
			res = append(res, t)
		}
	}
	return res
}

func (c *SyncToolsCommand) Run(_ *cmd.Context) error {
	officialEnviron, err := environs.NewFromAttrs(officialBucketAttrs)
	if err != nil {
		log.Infof("Failed to create officialEnviron")
		return err
	}
	c.sourceToolsList, err = environs.ListTools(officialEnviron, version.Current.Major)
	if err != nil {
		return err
	}
	env, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	c.targetToolsList, err = environs.ListTools(env, version.Current.Major)
	if err != nil {
		return err
	}
	return nil
}
