package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"os"
)

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket
type SyncToolsCommand struct {
	EnvCommandBase
	sourceToolsList *environs.ToolsList
	targetToolsList *environs.ToolsList
	allVersions     bool
	dryRun          bool
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
	f.BoolVar(&c.dryRun, "dry-run", false, "don't copy, just print what would be copied")

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

func copyOne(tool *state.Tools, source environs.StorageReader, target environs.Storage) error {
	toolsPath := environs.ToolsStoragePath(tool.Binary)
	if tool == nil {
		log.Warningf("tool was nil?")
		return nil
	}
	if source == nil {
		log.Warningf("source was nil?")
		return nil
	}
	if target == nil {
		log.Warningf("target was nil?")
		return nil
	}
	toolFile, err := source.Get(toolsPath)
	if err != nil {
		return err
	}
	defer toolFile.Close()
	// We have to copy to a local temp file, because Put requires the content
	// length, but Get only returns us a ReadCloser
	tempFile, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return err
	}
	// defer is LIFO, so make sure to call close last, so that the file is
	// closed when we go to delete it.
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	nBytes, err := io.Copy(tempFile, toolFile)
	if err != nil {
		return err
	}
	log.Infof("environs: downloaded %v (%dkB), uploading", toolsPath, (nBytes+512)/1024)

	tempFile.Seek(0, os.SEEK_SET)
	if err := target.Put(toolsPath, tempFile, nBytes); err != nil {
		return err
	}
	return nil
}

func copyTools(tools []*state.Tools, source environs.StorageReader, target environs.Storage, dryRun bool) error {
	for _, tool := range tools {
		log.Infof("Copying %s from %s\n", tool.Binary, tool.URL)
		if !dryRun {
			if err := copyOne(tool, source, target); err != nil {
				return err
			}
		}
	}
	return nil
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
	targetEnv, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	toolsToCopy := c.sourceToolsList.Public
	if !c.allVersions {
		toolsToCopy = findNewest(toolsToCopy)
	}
	for _, t := range toolsToCopy {
		fmt.Printf("Found: %s\n", t)
	}
	c.targetToolsList, err = environs.ListTools(targetEnv, version.Current.Major)
	if err != nil {
		return err
	}
	for _, t := range c.targetToolsList.Private {
		fmt.Printf("Found Target: %s\n", t)
	}
	missing := findMissing(toolsToCopy, c.targetToolsList.Private)
	err = copyTools(missing, officialEnviron.PublicStorage(), targetEnv.Storage(), c.dryRun)
	if err != nil {
		return err
	}
	return nil
}
