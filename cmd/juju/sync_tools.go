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
// bucket.
type SyncToolsCommand struct {
	EnvCommandBase
	allVersions  bool
	dryRun       bool
	publicBucket bool
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

	// BUG(lp:1163164)  jam 2013-04-2 we would like to add a "source"
	// location, rather than only copying from us-east-1
}

func (c *SyncToolsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

var officialBucketAttrs = map[string]interface{}{
	"name":            "juju-public",
	"type":            "ec2",
	"control-bucket":  "juju-dist",
	"access-key":      "",
	"secret-key":      "",
	"authorized-keys": "not-really", // We shouldn't need ssh access
}

func copyOne(
	tool *state.Tools, source environs.StorageReader,
	target environs.Storage, ctx *cmd.Context,
) error {
	toolsPath := environs.ToolsStoragePath(tool.Binary)
	fmt.Fprintf(ctx.Stderr, "copying %v", toolsPath)
	srcFile, err := source.Get(toolsPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	// We have to buffer the content, because Put requires the content
	// length, but Get only returns us a ReadCloser
	buf := &bytes.Buffer{}
	nBytes, err := io.Copy(buf, srcFile)
	if err != nil {
		return err
	}
	log.Infof("cmd/juju: downloaded %v (%dkB), uploading", toolsPath, (nBytes+512)/1024)
	fmt.Fprintf(ctx.Stderr, ", download %dkB, uploading\n", (nBytes+512)/1024)

	if err := target.Put(toolsPath, buf, nBytes); err != nil {
		return err
	}
	return nil
}

func copyTools(
	tools []*state.Tools, source environs.StorageReader,
	target environs.Storage, dryRun bool, ctx *cmd.Context,
) error {
	for _, tool := range tools {
		log.Infof("cmd/juju: copying %s from %s", tool.Binary, tool.URL)
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
		log.Errorf("cmd/juju: failed to initialize the official bucket environment")
		return err
	}
	fmt.Fprintf(ctx.Stderr, "listing the source bucket\n")
	sourceToolsList, err := environs.ListTools(officialEnviron, version.Current.Major)
	if err != nil {
		return err
	}
	targetEnv, err := environs.NewFromName(c.EnvName)
	if err != nil {
		log.Errorf("cmd/juju: unable to read %q from environment", c.EnvName)
		return err
	}
	toolsToCopy := sourceToolsList.Public
	if !c.allVersions {
		toolsToCopy = toolsToCopy.Newest()
	}
	fmt.Fprintf(ctx.Stderr, "found %d tools in source (%d recent ones)\n",
		len(sourceToolsList.Public), len(toolsToCopy))
	for _, tool := range toolsToCopy {
		log.Debugf("cmd/juju: found source tool: %s", tool)
	}
	fmt.Fprintf(ctx.Stderr, "listing target bucket\n")
	targetToolsList, err := environs.ListTools(targetEnv, version.Current.Major)
	if err != nil {
		return err
	}
	for _, tool := range targetToolsList.Private {
		log.Debugf("cmd/juju: found target tool: %s", tool)
	}
	targetTools := targetToolsList.Private
	targetStorage := targetEnv.Storage()
	if c.publicBucket {
		targetTools = targetToolsList.Public
		var ok bool
		if targetStorage, ok = targetEnv.PublicStorage().(environs.Storage); !ok {
			return fmt.Errorf("Cannot write to PublicStorage")
		}

	}
	missing := toolsToCopy.Difference(targetTools)
	fmt.Fprintf(ctx.Stdout, "found %d tools in target; %d tools to be copied\n",
		len(targetTools), len(missing))
	err = copyTools(missing, officialEnviron.PublicStorage(), targetStorage, c.dryRun, ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stderr, "copied %d tools\n", len(missing))
	return nil
}
