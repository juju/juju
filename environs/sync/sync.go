// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.sync")

// SyncContext describes the context for tool synchronization.
type SyncContext struct {
	// Target holds the destination for the tool synchronization
	Target storage.Storage

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

	// Tools are being synced for a public cloud so include mirrors information.
	Public bool

	// Source, if non-empty, specifies a directory in the local file system
	// to use as a source.
	Source string
}

// SyncTools copies the Juju tools tarball from the official bucket
// or a specified source directory into the user's environment.
func SyncTools(syncContext *SyncContext) error {
	sourceDataSource, err := selectSourceDatasource(syncContext)
	if err != nil {
		return err
	}

	logger.Infof("listing available tools")
	if syncContext.MajorVersion == 0 && syncContext.MinorVersion == 0 {
		syncContext.MajorVersion = version.Current.Major
		syncContext.MinorVersion = -1
		if !syncContext.AllVersions {
			syncContext.MinorVersion = version.Current.Minor
		}
	} else if !syncContext.Dev && syncContext.MinorVersion != -1 {
		// If a major.minor version is specified, we allow dev versions.
		// If Dev is already true, leave it alone.
		syncContext.Dev = true
	}

	released := !syncContext.Dev && !version.Current.IsDev()
	sourceTools, err := envtools.FindToolsForCloud(
		[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{},
		syncContext.MajorVersion, syncContext.MinorVersion, coretools.Filter{Released: released})
	if err != nil {
		return err
	}

	logger.Infof("found %d tools", len(sourceTools))
	if !syncContext.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		logger.Infof("found %d recent tools (version %s)", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source tool: %v", tool)
	}

	logger.Infof("listing target tools storage")
	targetStorage := syncContext.Target
	targetTools, err := envtools.ReadList(targetStorage, syncContext.MajorVersion, -1)
	switch err {
	case nil, coretools.ErrNoMatches, envtools.ErrNoTools:
	default:
		return err
	}
	for _, tool := range targetTools {
		logger.Debugf("found target tool: %v", tool)
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
		writeMirrors := envtools.DoNotWriteMirrors
		if syncContext.Public {
			writeMirrors = envtools.WriteMirrors
		}
		err = envtools.MergeAndWriteMetadata(targetStorage, targetTools, writeMirrors)
		if err != nil {
			return err
		}
	}
	logger.Infof("tools metadata written")
	return nil
}

// selectSourceStorage returns a storage reader based on the source setting.
func selectSourceDatasource(syncContext *SyncContext) (simplestreams.DataSource, error) {
	source := syncContext.Source
	if source == "" {
		source = envtools.DefaultBaseURL
	}
	sourceURL, err := envtools.ToolsURL(source)
	if err != nil {
		return nil, err
	}
	logger.Infof("using sync tools source: %v", sourceURL)
	return simplestreams.NewURLDataSource(sourceURL, simplestreams.VerifySSLHostnames), nil
}

// copyTools copies a set of tools from the source to the target.
func copyTools(tools []*coretools.Tools, syncContext *SyncContext, dest storage.Storage) error {
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
func copyOneToolsPackage(tool *coretools.Tools, dest storage.Storage) error {
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

// UploadFunc is the type of Upload, which may be
// reassigned to control the behaviour of tools
// uploading.
type UploadFunc func(stor storage.Storage, forceVersion *version.Number, series ...string) (*coretools.Tools, error)

// Upload builds whatever version of launchpad.net/juju-core is in $GOPATH,
// uploads it to the given storage, and returns a Tools instance describing
// them. If forceVersion is not nil, the uploaded tools bundle will report
// the given version number; if any fakeSeries are supplied, additional copies
// of the built tools will be uploaded for use by machines of those series.
// Juju tools built for one series do not necessarily run on another, but this
// func exists only for development use cases.
var Upload UploadFunc = upload

func upload(stor storage.Storage, forceVersion *version.Number, fakeSeries ...string) (*coretools.Tools, error) {
	// TODO(rog) find binaries from $PATH when not using a development
	// version of juju within a $GOPATH.

	logger.Debugf("Uploading tools for %v", fakeSeries)
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	toolsVersion, sha256Hash, err := envtools.BundleTools(f, forceVersion)
	if err != nil {
		return nil, err
	}
	fileInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat newly made tools archive: %v", err)
	}
	size := fileInfo.Size()
	logger.Infof("built %v (%dkB)", toolsVersion, (size+512)/1024)
	baseToolsDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(baseToolsDir)

	// Copy the tools to the target storage, recording a Tools struct for each one.
	var targetTools coretools.List
	putTools := func(vers version.Binary) (string, error) {
		name := envtools.StorageName(vers)
		err = utils.CopyFile(filepath.Join(baseToolsDir, name), f.Name())
		if err != nil {
			return "", err
		}
		url, err := stor.URL(name)
		if err != nil {
			return "", err
		}
		targetTools = append(targetTools, &coretools.Tools{
			Version: vers,
			URL:     url,
			Size:    size,
			SHA256:  sha256Hash,
		})
		return name, nil
	}
	err = os.MkdirAll(filepath.Join(baseToolsDir, storage.BaseToolsPath, "releases"), 0755)
	if err != nil {
		return nil, err
	}
	for _, series := range fakeSeries {
		_, err := simplestreams.SeriesVersion(series)
		if err != nil {
			return nil, err
		}
		if series != toolsVersion.Series {
			fakeVersion := toolsVersion
			fakeVersion.Series = series
			if _, err := putTools(fakeVersion); err != nil {
				return nil, err
			}
		}
	}
	name, err := putTools(toolsVersion)
	if err != nil {
		return nil, err
	}
	// The tools have been uploaded, now write out the matching simplestreams metadata so that SyncTools
	// can find them.
	metadataStore, err := filestorage.NewFileStorageWriter(baseToolsDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return nil, err
	}
	err = envtools.MergeAndWriteMetadata(metadataStore, targetTools, false)
	if err != nil {
		return nil, err
	}

	syncContext := &SyncContext{
		Source:       baseToolsDir,
		Target:       stor,
		AllVersions:  true,
		Dev:          toolsVersion.IsDev(),
		MajorVersion: toolsVersion.Major,
		MinorVersion: -1,
	}
	err = SyncTools(syncContext)
	if err != nil {
		return nil, err
	}
	url, err := stor.URL(name)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{
		Version: toolsVersion,
		URL:     url,
		Size:    size,
		SHA256:  sha256Hash,
	}, nil
}
