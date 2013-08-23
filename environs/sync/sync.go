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
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/ec2"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.sync")

// DefaultToolsLocation leads to the default juju distribution on S3.
var DefaultToolsLocation = "https://juju-dist.s3.amazonaws.com/"

// SyncContext describes the context for tool synchronization.
type SyncContext struct {
	// EnvName names the target environment for synchronization.
	EnvName string

	// AllVersions controls the copy of all versions, not only the latest.
	AllVersions bool

	// DryRun controls that nothing is copied. Instead it's logged
	// what would be coppied.
	DryRun bool

	// PublicBucket controls the copy into the public pucket of the
	// account instead of the private of the environment.
	PublicBucket bool

	// Dev controls the copy of development versions as well as released ones.
	Dev bool

	// Source allows to chose a location on the file system as source.
	Source string

	sourceStorage environs.StorageReader
	targetStorage environs.Storage
}

// SyncTools copies the Juju tools tarball from the official bucket
// or a specified source directory into the user's environment.
func SyncTools(ctx *SyncContext) error {
	var err error
	ctx.sourceStorage, err = selectSourceStorage(ctx)
	if err != nil {
		return fmt.Errorf("unable to select source: %v", err)
	}
	targetEnv, err := environs.NewFromName(ctx.EnvName)
	if err != nil {
		return fmt.Errorf("unable to read %q from environment", ctx.EnvName)
	}

	logger.Infof("listing available tools")
	majorVersion := version.Current.Major
	sourceTools, err := envtools.ReadList(ctx.sourceStorage, majorVersion)
	if err != nil {
		return err
	}
	if !ctx.Dev {
		// No development versions, only released ones.
		filter := coretools.Filter{Released: true}
		if sourceTools, err = sourceTools.Match(filter); err != nil {
			return err
		}
	}
	logger.Infof("found %d tools", len(sourceTools))
	if !ctx.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		logger.Infof("found %d recent tools (version %s)", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source tool: %s", tool)
	}

	logger.Infof("listing target bucket")
	ctx.targetStorage = targetEnv.Storage()
	if ctx.PublicBucket {
		switch _, err := envtools.ReadList(ctx.targetStorage, majorVersion); err {
		case envtools.ErrNoTools:
		case nil, coretools.ErrNoMatches:
			return fmt.Errorf("private tools present: public tools would be ignored")
		default:
			return err
		}
		var ok bool
		if ctx.targetStorage, ok = targetEnv.PublicStorage().(environs.Storage); !ok {
			return fmt.Errorf("cannot write to public storage")
		}
	}
	targetTools, err := envtools.ReadList(ctx.targetStorage, majorVersion)
	switch err {
	case nil, coretools.ErrNoMatches, envtools.ErrNoTools:
	default:
		return err
	}
	for _, tool := range targetTools {
		logger.Debugf("found target tool: %s", tool)
	}

	missing := sourceTools.Exclude(targetTools)
	logger.Infof("found %d tools in target; %d tools to be copied", len(targetTools), len(missing))
	err = copyTools(missing, ctx)
	if err != nil {
		return err
	}
	logger.Infof("copied %d tools", len(missing))
	return nil
}

// selectSourceStorage returns a storage reader based on the source setting.
func selectSourceStorage(ctx *SyncContext) (environs.StorageReader, error) {
	if ctx.Source == "" {
		return ec2.NewHTTPStorageReader(DefaultToolsLocation), nil
	}
	return newFileStorageReader(ctx.Source)
}

// copyTools copies a set of tools from the source to the target.
func copyTools(tools []*coretools.Tools, ctx *SyncContext) error {
	for _, tool := range tools {
		logger.Infof("copying %s from %s", tool.Version, tool.URL)
		if ctx.DryRun {
			continue
		}
		if err := copyOneToolsPackage(tool, ctx); err != nil {
			return err
		}
	}
	return nil
}

// copyOneToolsPackage copies one tool from the source to the target.
func copyOneToolsPackage(tool *coretools.Tools, ctx *SyncContext) error {
	toolsName := envtools.StorageName(tool.Version)
	logger.Infof("copying %v", toolsName)
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
	logger.Infof("download %dkB, uploading", (nBytes+512)/1024)

	if err := ctx.targetStorage.Put(toolsName, buf, nBytes); err != nil {
		return err
	}
	return nil
}

// fileStorageReader implements StorageReader backed
// by the local filesystem.
type fileStorageReader struct {
	path string
}

// newFileStorageReader returns a new storage reader for
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

// ConsistencyStrategy implements environs.StorageReader.ConsistencyStrategy.
func (f *fileStorageReader) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// NewSyncLogWriter creates a loggo writer for registration
// by the callers of Sync. This way the logged output can also
// be displayed otherwise, e.g. on the screen.
func NewSyncLogWriter(out, err io.Writer) loggo.Writer {
	return &syncLogWriter{out, err}
}

// syncLogWriter filters the log messages for
// "juju.environs.sync".
type syncLogWriter struct {
	out io.Writer
	err io.Writer
}

// Write implements loggo's Writer interface.
func (s *syncLogWriter) Write(level loggo.Level, name, filename string, line int, timestamp time.Time, message string) {
	if name == "juju.environs.sync" {
		if level <= loggo.INFO {
			fmt.Fprintf(s.out, "%s\n", message)
		} else {
			fmt.Fprintf(s.err, "%s\n", message)
		}
	}
}
