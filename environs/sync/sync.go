// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/filestorage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/ec2"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.sync")

// DefaultToolsLocation leads to the default juju distribution on S3.
var DefaultToolsLocation = "https://juju-dist.s3.amazonaws.com/"

// SyncContext describes the context for tool synchronization.
type SyncContext struct {
	// Target holds the destination for the tool synchronization
	Target environs.Storage

	// AllVersions controls the copy of all versions, not only the latest.
	AllVersions bool

	// DryRun controls that nothing is copied. Instead it's logged
	// what would be coppied.
	DryRun bool

	// Dev controls the copy of development versions as well as released ones.
	Dev bool

	// Source, if non-empty, specifies a directory in the local file system
	// to use as a source.
	Source string
}

// SyncTools copies the Juju tools tarball from the official bucket
// or a specified source directory into the user's environment.
func SyncTools(ctx *SyncContext) error {
	sourceStorage, err := selectSourceStorage(ctx)
	if err != nil {
		return fmt.Errorf("unable to select source: %v", err)
	}

	logger.Infof("listing available tools")
	majorVersion := version.Current.Major
	minorVersion := -1
	if !ctx.AllVersions {
		minorVersion = version.Current.Minor
	}
	sourceTools, err := envtools.ReadList(sourceStorage, majorVersion, minorVersion)
	if err != nil {
		return err
	}
	if !ctx.Dev {
		// If we are running from a dev version, then it is appropriate to allow
		// dev version tools to be used.
		filter := coretools.Filter{Released: !version.Current.IsDev()}
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
	targetStorage := ctx.Target
	targetTools, err := envtools.ReadList(targetStorage, majorVersion, -1)
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
	err = copyTools(missing, ctx, targetStorage, sourceStorage)
	if err != nil {
		return err
	}
	logger.Infof("copied %d tools", len(missing))

	logger.Infof("generating tools metadata")
	if !ctx.DryRun {
		targetTools = append(targetTools, missing...)
		err = envtools.WriteMetadata(targetTools, false, targetStorage)
		if err != nil {
			return err
		}
	}
	logger.Infof("tools metadata written")
	return nil
}

// selectSourceStorage returns a storage reader based on the source setting.
func selectSourceStorage(ctx *SyncContext) (environs.StorageReader, error) {
	if ctx.Source == "" {
		return ec2.NewHTTPStorageReader(DefaultToolsLocation), nil
	}
	return filestorage.NewFileStorageReader(ctx.Source)
}

// copyTools copies a set of tools from the source to the target.
func copyTools(tools []*coretools.Tools, ctx *SyncContext, dest environs.Storage, source environs.StorageReader) error {
	for _, tool := range tools {
		logger.Infof("copying %s from %s", tool.Version, tool.URL)
		if err := copyOneToolsPackage(ctx, tool, dest, source); err != nil {
			return err
		}
	}
	return nil
}

// copyOneToolsPackage copies one tool from the source to the target.
func copyOneToolsPackage(ctx *SyncContext, tool *coretools.Tools, dest environs.Storage, src environs.StorageReader) error {
	toolsName := envtools.StorageName(tool.Version)
	logger.Infof("copying %v", toolsName)
	srcFile, err := src.Get(toolsName)
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
	sha256hash := sha256.New()
	sha256hash.Write(buf.Bytes())
	tool.SHA256 = fmt.Sprintf("%x", sha256hash.Sum(nil))
	tool.Size = nBytes

	if ctx.DryRun {
		return nil
	}
	return dest.Put(toolsName, buf, nBytes)
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
