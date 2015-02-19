// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"io"
	"os"
	"path"

	"github.com/juju/errors"
)

// TODO(ericsnow) Use filepath.FromSlash on Windows to convert paths
// in file operations? Have fs.Operations handle that?

// TODO(ericsnow) For the moment init systems only need the conf file.
// Still, how do we handle the situation where they need more than that?
// `ConfHandler.Serialize` could return a list of `FileData` that gets
// stored on `ConfDir.InitFiles` and `FileData` could have a `Target`
// field that allows the init system to specify where symlinks should
// go. However, the init system should be in charge of that. Would we
// need to pass a list of `FileData` to the `InitSystem.Enable`?

// confFileOperations exposes the parts of fs.Operations used by confDir.
type confFileOperations interface {
	// Exists implements fs.Operations.
	Exists(string) (bool, error)

	// ListDir implements fs.Operations.
	ListDir(string) ([]os.FileInfo, error)

	// MkdirAll implements fs.Operations.
	MkdirAll(string, os.FileMode) error

	// ReadFile implements fs.Operations.
	ReadFile(string) ([]byte, error)

	// CreateFile implements fs.Operations.
	CreateFile(string) (io.WriteCloser, error)

	// Chmod implements fs.Operations.
	Chmod(string, os.FileMode) error
}

// confDirInfo holds the common data between ConfDirInfo and ConfDir.
type confDirInfo struct {
	// DirName is the absolute path to the conf dir. The path should
	// be in the generic slash-delimited format.
	DirName string

	// Name is the name of the service to which this conf dir corresponds.
	Name string

	// InitSystem is the name of the corresponding init system.
	InitSystem string
}

// Remove deletes the conf directory.
func (cdi confDirInfo) Remove(removeAll func(string) error) error {
	err := removeAll(cdi.DirName)
	if os.IsNotExist(err) {
		// Already deleted.
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "while removing conf dir for %q", cdi.Name)
	}
	return nil
}

// ConfDirInfo describes a single directory where files, including the
// conf file, for an init system "service" are stored.
type ConfDirInfo struct {
	confDirInfo
}

// NewConfDirInfo creates a new ConfDirInfo based on the provided
// information and returns it.
func NewConfDirInfo(name, parentDir, initSystem string) ConfDirInfo {
	dirName := path.Join(parentDir, name)
	return ConfDirInfo{confDirInfo{
		DirName:    dirName,
		Name:       name,
		InitSystem: initSystem,
	}}
}

// Read creates a new ConfDir by extracting the data from the actual
// conf directory and returns it.
func (cdi ConfDirInfo) Read(fops confFileOperations) (ConfDir, error) {
	var confDir ConfDir

	if cdi.InitSystem == "" {
		return confDir, errors.New("InitSystem not set")
	}
	initDir := path.Join(cdi.DirName, cdi.InitSystem)

	// Extract the metadata.
	meta, err := readMetadata(initDir, fops.ReadFile)
	if err != nil {
		return confDir, errors.Trace(err)
	}

	// Initalize the ConfDir.
	confDir = ConfDir{
		confDirInfo: cdi.confDirInfo,
		ConfName:    meta.ConfName,
		Conf:        meta.Conf,
	}

	// Extract the conf file.
	data, err := fops.ReadFile(path.Join(initDir, meta.ConfName))
	if err != nil {
		return confDir, errors.Trace(err)
	}
	confDir.ConfFile.FileName = meta.ConfName
	confDir.ConfFile.Data = data

	// TODO(ericsnow) Fail for unrecognized files?

	// Extract top-level files.
	topFiles, err := fops.ListDir(cdi.DirName)
	if err != nil {
		return confDir, errors.Trace(err)
	}
	for _, info := range topFiles {
		if info.IsDir() {
			continue
		}
		file, err := readFileData(cdi.DirName, info, fops.ReadFile)
		if err != nil {
			return confDir, errors.Trace(err)
		}
		confDir.Files = append(confDir.Files, file)
	}

	// TODO(ericsnow) Verify that the path in Conf.Cmd, if a path,
	// exists as a file.

	return confDir, nil
}

// confFile produces a new conf file for the conf dir based on the
// provided conf. The ConfHandler is used to validate and serialize
// the conf.
func (cdi ConfDirInfo) confFile(conf Conf, init ConfHandler) (FileData, error) {
	var confFile FileData

	// conf should already be normalized.
	confName, err := init.Validate(cdi.Name, conf)
	if err != nil {
		return confFile, errors.Trace(err)
	}

	data, err := init.Serialize(cdi.Name, conf)
	if err != nil {
		return confFile, errors.Trace(err)
	}

	confFile = FileData{
		FileName: confName,
		Data:     data,
	}
	return confFile, nil
}

// Populate builds a new ConfDir from the provided conf and returns it.
// The given ConfHandler is used to normalize the conf, serialize it, and
// generate the conf file name.
func (cdi ConfDirInfo) Populate(conf Conf, init ConfHandler) (ConfDir, error) {
	var confDir ConfDir

	// Validate the args.
	if cdi.InitSystem == "" {
		// cdi is a value receiver, so changing this value is okay.
		cdi.InitSystem = init.Name()
	} else if init.Name() != cdi.InitSystem {
		msg := "init system mismatch; expected %q, got %q"
		return confDir, errors.Errorf(msg, cdi.InitSystem, init.Name())
	}

	// Normalize the conf.
	normalConf, files, err := conf.Normalize(cdi.DirName, init)
	if err != nil {
		return confDir, errors.Trace(err)
	}
	// TODO(ericsnow) Should the ConfHandler have a chance to further
	// normalize the conf?

	// Build the conf file.
	confFile, err := cdi.confFile(normalConf, init)
	if err != nil {
		return confDir, errors.Trace(err)
	}

	// Return the conf dir.
	confDir = ConfDir{
		confDirInfo: cdi.confDirInfo,
		ConfName:    confFile.FileName,
		Conf:        normalConf,
		ConfFile:    confFile,
		Files:       files,
	}
	return confDir, nil
}

// ConfDir describes the contents of a directory where the conf file
// for an init system "service" is stored, along with any other
// associated files.
type ConfDir struct {
	confDirInfo

	// ConfName is the base name for the conf file as specified by the
	// init system.
	ConfName string

	// Conf is the normalized conf that pertains to this conf dir.
	Conf Conf

	// ConfFile is the description of the conf file in the conf dir
	// for a specific init system.
	ConfFile FileData

	// Files is the list of all non-init-system-specific files that
	// belong in the conf dir.
	Files []FileData
}

func (cd ConfDir) Filename() string {
	return path.Join(cd.DirName, cd.InitSystem, cd.ConfName)
}

// TODO(ericsnow) Add a Rebase method that creates a new ConfDir based
// in a different directory? This could be useful for copying an
// existing one.

// TODO(ericsnow) Do not fail in Write if existing files are identical
// to the corresponding new ones?

// Write creates the conf dir (if missing) and writes out all the files.
// If any of the files already exists then the operation fails with
// errors.AlreadyExists.
func (cd ConfDir) Write(fops confFileOperations) error {
	// TODO(ericsnow) Handle clobber-protection.

	initDir := path.Join(cd.DirName, cd.InitSystem)

	if err := fops.MkdirAll(initDir, 0755); err != nil {
		return errors.Trace(err)
	}

	createFile := func(filename string, perm os.FileMode, data []byte) error {
		// TODO(ericsnow) Handle the file permissions.

		file, err := fops.CreateFile(filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer file.Close()

		_, err = file.Write(data)
		return errors.Trace(err)
	}

	// Write out the conf file first.
	if err := cd.ConfFile.Write(initDir, createFile); err != nil {
		return errors.Trace(err)
	}

	// Next write out any top-level files.
	for _, file := range cd.Files {
		if err := file.Write(cd.DirName, createFile); err != nil {
			return errors.Trace(err)
		}
	}

	// Write out the metadata file last. That way it acts as an
	// indicator that all other files were successfully written.
	meta := metadata{
		Conf:     cd.Conf,
		Name:     cd.Name,
		ConfName: cd.ConfName,
	}
	metafile, err := meta.fileData()
	if err != nil {
		return errors.Trace(err)
	}
	if err := metafile.Write(initDir, createFile); err != nil {
		return errors.Trace(err)
	}

	return nil
}
