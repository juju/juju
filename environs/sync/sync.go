// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"

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

	// Copy tools with major version, if MajorVersion > 0.
	MajorVersion int

	// Copy tools with minor version, if MinorVersion > 0.
	MinorVersion int

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
func SyncTools(syncContext *SyncContext) error {
	sourceStorage, err := selectSourceStorage(syncContext)
	if err != nil {
		return fmt.Errorf("unable to select source: %v", err)
	}

	logger.Infof("listing available tools")
	if syncContext.MajorVersion == 0 && syncContext.MinorVersion == 0 {
		syncContext.MajorVersion = version.Current.Major
		syncContext.MinorVersion = -1
		if !syncContext.AllVersions {
			syncContext.MinorVersion = version.Current.Minor
		}
	} else {
		// If a major.minor version is specified, we allow dev versions.
		syncContext.Dev = syncContext.MinorVersion != -1
	}
	sourceTools, err := envtools.ReadList(sourceStorage, syncContext.MajorVersion, syncContext.MinorVersion)
	if err != nil {
		return err
	}
	if !syncContext.Dev {
		// If we are running from a dev version, then it is appropriate to allow
		// dev version tools to be used.
		filter := coretools.Filter{Released: !version.Current.IsDev()}
		if sourceTools, err = sourceTools.Match(filter); err != nil {
			return err
		}
	}
	logger.Infof("found %d tools", len(sourceTools))
	if !syncContext.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		logger.Infof("found %d recent tools (version %s)", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source tool: %s", tool)
	}

	logger.Infof("listing target bucket")
	targetStorage := syncContext.Target
	targetTools, err := envtools.ReadList(targetStorage, syncContext.MajorVersion, -1)
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
	err = copyTools(missing, syncContext, targetStorage)
	if err != nil {
		return err
	}
	logger.Infof("copied %d tools", len(missing))

	logger.Infof("generating tools metadata")
	if !syncContext.DryRun {
		targetTools = append(targetTools, missing...)
		err = envtools.WriteMetadata(targetTools, true, targetStorage)
		if err != nil {
			return err
		}
	}
	logger.Infof("tools metadata written")
	return nil
}

// selectSourceStorage returns a storage reader based on the source setting.
func selectSourceStorage(syncContext *SyncContext) (environs.StorageReader, error) {
	if syncContext.Source == "" {
		return ec2.NewHTTPStorageReader(DefaultToolsLocation), nil
	}
	return filestorage.NewFileStorageReader(syncContext.Source)
}

// copyTools copies a set of tools from the source to the target.
func copyTools(tools []*coretools.Tools, syncContext *SyncContext, dest environs.Storage) error {
	for _, tool := range tools {
		logger.Infof("copying %s from %s", tool.Version, tool.URL)
		if syncContext.DryRun {
			continue
		}
		if err := copyOneToolsPackage(tool, dest); err != nil {
			return err
		}
	}
	return nil
}

// copyOneToolsPackage copies one tool from the source to the target.
func copyOneToolsPackage(tool *coretools.Tools, dest environs.Storage) error {
	defer func() {
		envtools.SetToolPrefix(envtools.DefaultToolPrefix)
	}()
	envtools.SetToolPrefix(envtools.NewToolPrefix)
	toolsName := envtools.StorageName(tool.Version)
	logger.Infof("copying %v", toolsName)
	resp, err := http.Get(tool.URL)
	if err != nil {
		return err
	}
	srcFile := resp.Body
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
	return dest.Put(toolsName, buf, nBytes)
}
