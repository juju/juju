package main

import (
	"bytes"
	"fmt"
	"io"
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
	dryRun          bool
	publicBucket    bool
}

var _ cmd.Command = (*SyncToolsCommand)(nil)

func (c *SyncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "copy tools from the official bucket into a local environment",
		Doc: `
This copies the Juju tools tarball from the official bucket into
your environment. This is generally done when you want Juju to be able
to run without having to access Amazon. Sometimes this is because the
environment does not have public access, and sometimes you just want
to avoid having to access data outside of the local cloud.
`,
	}
}

func (c *SyncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "copy all versions, not just the latest")
	f.BoolVar(&c.dryRun, "dry-run", false, "don't copy, just print what would be copied")
	f.BoolVar(&c.publicBucket, "public", false, "write to the public-bucket of the account, instead of the bucket private to the environment.")

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

// Find the set of tools at the 'latest' version
func findNewest(fullTools []*state.Tools) []*state.Tools {
	var curBest *state.Tools = nil
	var res []*state.Tools = nil
	for _, tool := range fullTools {
		var add = false
		if curBest == nil || curBest.Number.Less(tool.Number) {
			// This best is clearly better than all existing
			// entries, so reset the list
			res = make([]*state.Tools, 0, 1)
			add = true
			curBest = tool
		}
		if curBest.Number == tool.Number {
			add = true
		}
		if add {
			res = append(res, tool)
		}
	}
	return res
}

// Find tools that aren't present in target
func findMissing(sourceTools, targetTools []*state.Tools) []*state.Tools {
	var target = make(map[version.Binary]bool, len(targetTools))
	for _, tool := range targetTools {
		target[tool.Binary] = true
	}
	res := make([]*state.Tools, 0)
	for _, tool := range sourceTools {
		if present := target[tool.Binary]; !present {
			res = append(res, tool)
		}
	}
	return res
}

func copyOne(tool *state.Tools, source environs.StorageReader,
	target environs.Storage, ctx *cmd.Context) error {
	toolsPath := environs.ToolsStoragePath(tool.Binary)
	fmt.Fprintf(ctx.Stdout, "copying %v", toolsPath)
	toolFile, err := source.Get(toolsPath)
	if err != nil {
		return err
	}
	defer toolFile.Close()
	// We have to buffer the content, because Put requires the content
	// length, but Get only returns us a ReadCloser
	buf := bytes.NewBuffer(nil)
	nBytes, err := io.Copy(buf, toolFile)
	if err != nil {
		return err
	}
	log.Infof("downloaded %v (%dkB), uploading", toolsPath, (nBytes+512)/1024)
	fmt.Fprintf(ctx.Stdout, ", download %dkB, uploading\n", (nBytes+512)/1024)

	if err := target.Put(toolsPath, buf, nBytes); err != nil {
		return err
	}
	return nil
}

func copyTools(tools []*state.Tools, source environs.StorageReader,
	target environs.Storage, dryRun bool, ctx *cmd.Context) error {
	for _, tool := range tools {
		log.Infof("copying %s from %s\n", tool.Binary, tool.URL)
		if dryRun {
			continue
		}
		if err := copyOne(tool, source, target, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *SyncToolsCommand) Run(ctx *cmd.Context) error {
	officialEnviron, err := environs.NewFromAttrs(officialBucketAttrs)
	if err != nil {
		log.Errorf("failed to initialize the official bucket environment")
		return err
	}
	fmt.Fprintf(ctx.Stdout, "listing the source bucket\n")
	c.sourceToolsList, err = environs.ListTools(officialEnviron, version.Current.Major)
	if err != nil {
		return err
	}
	targetEnv, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	toolsToCopy := c.sourceToolsList.Public
	if !c.allVersions {
		toolsToCopy = findNewest(toolsToCopy)
	}
	fmt.Fprintf(ctx.Stdout, "found %d tools in source (%d recent ones)\n",
		len(c.sourceToolsList.Public), len(toolsToCopy))
	for _, tool := range toolsToCopy {
		log.Debugf("found source tool: %s", tool)
	}
	fmt.Fprintf(ctx.Stdout, "listing target bucket\n")
	c.targetToolsList, err = environs.ListTools(targetEnv, version.Current.Major)
	if err != nil {
		return err
	}
	for _, tool := range c.targetToolsList.Private {
		log.Debugf("found target tool: %s", tool)
	}
	targetTools := c.targetToolsList.Private
	targetStorage := targetEnv.Storage()
	if c.publicBucket {
		targetTools = c.targetToolsList.Public
		var ok bool
		if targetStorage, ok = targetEnv.PublicStorage().(environs.Storage); !ok {
			return fmt.Errorf("Cannot write to PublicStorage")
		}

	}
	missing := findMissing(toolsToCopy, targetTools)
	fmt.Fprintf(ctx.Stdout, "found %d tools in target; %d tools to be copied\n",
		len(targetTools), len(missing))
	err = copyTools(missing, officialEnviron.PublicStorage(), targetStorage, c.dryRun, ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "copied %d tools\n", len(missing))
	return nil
}
