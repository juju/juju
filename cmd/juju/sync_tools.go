package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
)

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket
type SyncToolsCommand struct {
	EnvCommandBase
	sourceToolsList *environs.ToolsList
	targetToolsList *environs.ToolsList
	agentVersion    version.Number
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
	// f.BoolVar(&c.Development, "dev", false, "allow development versions to be chosen")

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
	for _, tool := range c.sourceToolsList.Public {
		fmt.Printf("Found: %s\n", tool.URL)
	}
	_, err = environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	return nil
}
