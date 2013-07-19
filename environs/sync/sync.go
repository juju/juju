// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.sync")

// defaultToolsUrl leads to the juju distribution on S3.
var defaultToolsLocation string = "https://juju-dist.s3.amazonaws.com/"

// SyncContext describes the context for tool synchronization.
type SyncContext struct {
	EnvName       string
	AllVersions   bool
	DryRun        bool
	PublicBucket  bool
	Dev           bool
	Source        string
	Stdout        io.Writer
	Stderr        io.Writer
	sourceStorage environs.StorageReader
	targetStorage environs.Storage
}

// SyncTools copies the Juju tools tarball from the official bucket
// or a specified source directory into the users environment.
func SyncTools(ctx *SyncContext) (err error) {
	ctx.sourceStorage, err = selectSourceStorage(ctx)
	if err != nil {
		logger.Errorf("unable to select source: %v", err)
		return err
	}
	targetEnv, err := environs.NewFromName(ctx.EnvName)
	if err != nil {
		logger.Errorf("unable to read %q from environment", ctx.EnvName)
		return err
	}

	fmt.Fprintf(ctx.Stderr, "listing the source bucket\n")
	majorVersion := version.Current.Major
	sourceTools, err := tools.ReadList(ctx.sourceStorage, majorVersion)
	if err != nil {
		return err
	}
	if !ctx.Dev {
		// No development versions, only released ones.
		filter := tools.Filter{Released: true}
		if sourceTools, err = sourceTools.Match(filter); err != nil {
			return err
		}
	}
	fmt.Fprintf(ctx.Stderr, "found %d tools\n", len(sourceTools))
	if !ctx.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		fmt.Fprintf(ctx.Stderr, "found %d recent tools (version %s)\n", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source tool: %s", tool)
	}

	fmt.Fprintf(ctx.Stderr, "listing target bucket\n")
	ctx.targetStorage = targetEnv.Storage()
	if ctx.PublicBucket {
		switch _, err := tools.ReadList(ctx.targetStorage, majorVersion); err {
		case tools.ErrNoTools:
		case nil, tools.ErrNoMatches:
			return fmt.Errorf("private tools present: public tools would be ignored")
		default:
			return err
		}
		var ok bool
		if ctx.targetStorage, ok = targetEnv.PublicStorage().(environs.Storage); !ok {
			return fmt.Errorf("cannot write to public storage")
		}
	}
	targetTools, err := tools.ReadList(ctx.targetStorage, majorVersion)
	switch err {
	case nil, tools.ErrNoMatches, tools.ErrNoTools:
	default:
		return err
	}
	for _, tool := range targetTools {
		logger.Debugf("found target tool: %s", tool)
	}

	missing := sourceTools.Exclude(targetTools)
	fmt.Fprintf(ctx.Stdout, "found %d tools in target; %d tools to be copied\n", len(targetTools), len(missing))
	err = copyTools(missing, ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stderr, "copied %d tools\n", len(missing))
	return nil
}

// selectSourceStorage returns a storage reader based on the source setting.
func selectSourceStorage(ctx *SyncContext) (environs.StorageReader, error) {
	if ctx.Source == "" {
		return ec2.NewHTTPStorageReader(defaultToolsLocation), nil
	}
	return newFileStorageReader(ctx.Source)
}

// copyTools copies a set of tools from the source to the target.
func copyTools(tools []*state.Tools, ctx *SyncContext) error {
	for _, tool := range tools {
		logger.Infof("copying %s from %s", tool.Binary, tool.URL)
		if ctx.DryRun {
			continue
		}
		if err := copyOneTool(tool, ctx); err != nil {
			return err
		}
	}
	return nil
}

// copyOneTool copies one tool from the source to the target.
func copyOneTool(tool *state.Tools, ctx *SyncContext) error {
	toolsName := tools.StorageName(tool.Binary)
	fmt.Fprintf(ctx.Stderr, "copying %v", toolsName)
	srcFile, err := ctx.sourceStorage.Get(toolsName)
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
	logger.Infof("downloaded %v (%dkB), uploading", toolsName, (nBytes+512)/1024)
	fmt.Fprintf(ctx.Stderr, ", download %dkB, uploading\n", (nBytes+512)/1024)

	if err := ctx.targetStorage.Put(toolsName, buf, nBytes); err != nil {
		return err
	}
	return nil
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
