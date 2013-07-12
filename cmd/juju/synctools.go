// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// defaultToolsUrl leads to the juju distribution on S3.
var defaultToolsLocation string = "https://juju-dist.s3.amazonaws.com/"

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket.
type SyncToolsCommand struct {
	EnvCommandBase
	allVersions  bool
	dryRun       bool
	publicBucket bool
	dev          bool
	source       string
}

var _ cmd.Command = (*SyncToolsCommand)(nil)

func (c *SyncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "copy tools from the official bucket into a local environment",
		Doc: `
This copies the Juju tools tarball from the official bucket into
your environment. This is generally done when you want Juju to be able
to run without having to access Amazon. Alternatively you can specify
a local directory as source.

Sometimes this is because the environment does not have public access, 
and sometimes you just want to avoid having to access data outside of 
the local cloud.
`,
	}
}

func (c *SyncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "copy all versions, not just the latest")
	f.BoolVar(&c.dryRun, "dry-run", false, "don't copy, just print what would be copied")
	f.BoolVar(&c.dev, "dev", false, "consider development versions as well as released ones")
	f.BoolVar(&c.publicBucket, "public", false, "write to the public-bucket of the account, instead of the bucket private to the environment.")
	f.StringVar(&c.source, "source", "", "chose a location on the file system as source")

	// BUG(lp:1163164)  jam 2013-04-2 we would like to add a "source"
	// location, rather than only copying from us-east-1
}

func (c *SyncToolsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func copyOne(
	tool *state.Tools, source environs.StorageReader,
	target environs.Storage, ctx *cmd.Context,
) error {
	toolsName := tools.StorageName(tool.Binary)
	fmt.Fprintf(ctx.Stderr, "copying %v", toolsName)
	srcFile, err := source.Get(toolsName)
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
	log.Infof("downloaded %v (%dkB), uploading", toolsName, (nBytes+512)/1024)
	fmt.Fprintf(ctx.Stderr, ", download %dkB, uploading\n", (nBytes+512)/1024)

	if err := target.Put(toolsName, buf, nBytes); err != nil {
		return err
	}
	return nil
}

func copyTools(
	tools []*state.Tools, source environs.StorageReader,
	target environs.Storage, dryRun bool, ctx *cmd.Context,
) error {
	for _, tool := range tools {
		log.Infof("copying %s from %s", tool.Binary, tool.URL)
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
	sourceStorage, err := selectSourceStorage(c.source)
	if err != nil {
		log.Errorf("unable to select source: %v", err)
		return err
	}
	targetEnv, err := environs.NewFromName(c.EnvName)
	if err != nil {
		log.Errorf("unable to read %q from environment", c.EnvName)
		return err
	}

	fmt.Fprintf(ctx.Stderr, "listing the source bucket\n")
	majorVersion := version.Current.Major
	sourceTools, err := tools.ReadList(sourceStorage, majorVersion)
	if err != nil {
		return err
	}
	if !c.dev {
		filter := tools.Filter{Released: true}
		if sourceTools, err = sourceTools.Match(filter); err != nil {
			return err
		}
	}
	fmt.Fprintf(ctx.Stderr, "found %d tools\n", len(sourceTools))
	if !c.allVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		fmt.Fprintf(ctx.Stderr, "found %d recent tools (version %s)\n", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		log.Debugf("found source tool: %s", tool)
	}

	fmt.Fprintf(ctx.Stderr, "listing target bucket\n")
	targetStorage := targetEnv.Storage()
	if c.publicBucket {
		switch _, err := tools.ReadList(targetStorage, majorVersion); err {
		case tools.ErrNoTools:
		case nil, tools.ErrNoMatches:
			return fmt.Errorf("private tools present: public tools would be ignored")
		default:
			return err
		}
		var ok bool
		if targetStorage, ok = targetEnv.PublicStorage().(environs.Storage); !ok {
			return fmt.Errorf("cannot write to public storage")
		}
	}
	targetTools, err := tools.ReadList(targetStorage, majorVersion)
	switch err {
	case nil, tools.ErrNoMatches, tools.ErrNoTools:
	default:
		return err
	}
	for _, tool := range targetTools {
		log.Debugf("found target tool: %s", tool)
	}

	missing := sourceTools.Exclude(targetTools)
	fmt.Fprintf(ctx.Stdout, "found %d tools in target; %d tools to be copied\n",
		len(targetTools), len(missing))
	err = copyTools(missing, sourceStorage, targetStorage, c.dryRun, ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stderr, "copied %d tools\n", len(missing))
	return nil
}

// selectSourceStorage returns a storage reader based on the passed source flag.
func selectSourceStorage(sourceFlagValue string) (environs.StorageReader, error) {
	if sourceFlagValue == "" {
		return ec2.NewHTTPStorageReader(defaultToolsLocation), nil
	}
	return newFileStorageReader(sourceFlagValue)
}

// fileStorageReader implements StorageReader backed by the local filesystem.
type fileStorageReader struct {
	path string
}

// newFileStorageReader return a new storage reader for
// a directory inside the local file system.
func newFileStorageReader(path string) (environs.StorageReader, error) {
	p := filepath.Clean(path)
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsDir() {
		return nil, fmt.Errorf("specified source path is not a directory: %s", path)
	}
	return &fileStorageReader{p}, nil
}

// Get implements environs.StorageReader.Get.
func (f *fileStorageReader) Get(name string) (io.ReadCloser, error) {
	filename, err := f.URL(name)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// List implements environs.StorageReader.List.
func (f *fileStorageReader) List(prefix string) ([]string, error) {
	// Add one for the missing path separator.
	pathlen := len(f.path) + 1
	pattern := filepath.Join(f.path, prefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	list := []string{}
	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			return nil, err
		}
		if !fi.Mode().IsDir() {
			filename := match[pathlen:]
			list = append(list, filename)
		}
	}
	sort.Strings(list)
	return list, nil
}

// URL implements environs.StorageReader.URL.
func (f *fileStorageReader) URL(name string) (string, error) {
	return path.Join(f.path, name), nil
}
