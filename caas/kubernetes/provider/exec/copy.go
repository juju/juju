// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

// FileResource holds all the necessary parameters for the source or destination of a copy request.
type FileResource struct {
	Path          string
	PodName       string
	ContainerName string
}

func (cp *FileResource) validate() (err error) {
	if cp.Path == "" {
		return errors.New("path was missing")
	}
	return nil
}

// CopyParams holds all the necessary parameters for a copy request.
type CopyParams struct {
	Src                           FileResource
	Dest                          FileResource
	overwriteOwnershipPermissions bool
}

func (cp *CopyParams) validate() error {
	if err := cp.Src.validate(); err != nil {
		return errors.Trace(err)
	}
	if err := cp.Dest.validate(); err != nil {
		return errors.Trace(err)
	}
	if cp.Src.PodName != "" && cp.Dest.PodName != "" {
		return errors.New("cross pods copy is not supported")
	}
	if cp.Src.PodName == "" && cp.Dest.PodName == "" {
		return errors.New("copy either from pod to host or from host to pod")
	}
	return nil
}

// Exec copy files/directories from host to a pod or from a pod to host.
func (c client) Copy(params CopyParams, cancel <-chan struct{}) error {
	if err := params.validate(); err != nil {
		return errors.Trace(err)
	}
	if params.Src.PodName != "" {
		return c.copyFromPod(params, cancel)
	}
	if params.Dest.PodName != "" {
		return c.copyToPod(params, cancel)
	}
	return nil
}

func (c client) copyFromPod(params CopyParams, cancel <-chan struct{}) error {
	// TODO(caas): implement when we need later.
	return errors.NotSupportedf("copy from pod")
}

// this is inspired by kubectl cmd package.
// - https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go
func (c client) copyToPod(params CopyParams, cancel <-chan struct{}) (err error) {
	src := params.Src
	dest := params.Dest
	logger.Debugf("copying from %v to %v", src, dest)

	if _, err = os.Stat(src.Path); err != nil {
		return errors.NewNotValid(nil, fmt.Sprintf("%q does not exist on local", src.Path))
	}

	if dest.Path != "/" && strings.HasSuffix(dest.Path, "/") {
		dest.Path = strings.TrimSuffix(dest.Path, "/")
	}

	if err = c.checkRemotePathIsDir(dest, cancel); err == nil {
		dest.Path = path.Join(dest.Path, path.Base(src.Path))
	}

	reader, writer := c.pipGetter()

	go func() {
		defer writer.Close()
		err = makeTar(src.Path, dest.Path, writer)
		if err != nil {
			logger.Errorf("make tar %q failed: %v", src.Path, err)
		}
	}()

	cmds := []string{"tar", "-xmf", "-"}
	if params.overwriteOwnershipPermissions {
		cmds = []string{"tar", "--no-same-permissions", "--no-same-owner", "-xmf", "-"}
	}
	destDir := path.Dir(dest.Path)
	if len(destDir) > 0 {
		cmds = append(cmds, "-C", destDir)
	}
	var stdout, stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       dest.PodName,
		ContainerName: dest.ContainerName,
		Commands:      cmds,
		Stdin:         reader,
		Stdout:        &stdout,
		Stderr:        &stderr,
	}
	return errors.Trace(c.Exec(execParams, cancel))
}

func (c client) checkRemotePathIsDir(rec FileResource, cancel <-chan struct{}) error {
	if rec.PodName == "" {
		return errors.NotValidf("empty pod name")
	}
	var stdout, stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       rec.PodName,
		ContainerName: rec.ContainerName,
		Commands:      []string{"test", "-d", rec.Path},
		Stdout:        &stdout,
		Stderr:        &stderr,
	}
	return errors.Trace(c.Exec(execParams, cancel))
}

// Based on code from https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go
func makeTar(srcPath, destPath string, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()
	srcPath = path.Clean(srcPath)
	destPath = path.Clean(destPath)
	return recursiveTar(path.Dir(srcPath), path.Base(srcPath), path.Dir(destPath), path.Base(destPath), tarWriter)
}

// Based on code from https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go
func recursiveTar(srcBase, srcFile, destBase, destFile string, tw *tar.Writer) error {
	srcPath := path.Join(srcBase, srcFile)
	matchedPaths, err := filepath.Glob(srcPath)
	if err != nil {
		return err
	}
	for _, fpath := range matchedPaths {
		stat, err := os.Lstat(fpath)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			files, err := ioutil.ReadDir(fpath)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				//case empty directory
				hdr, _ := tar.FileInfoHeader(stat, fpath)
				hdr.Name = destFile
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
			}
			for _, f := range files {
				if err := recursiveTar(srcBase, path.Join(srcFile, f.Name()), destBase, path.Join(destFile, f.Name()), tw); err != nil {
					return err
				}
			}
			return nil
		} else if stat.Mode()&os.ModeSymlink != 0 {
			//case soft link
			hdr, _ := tar.FileInfoHeader(stat, fpath)
			target, err := os.Readlink(fpath)
			if err != nil {
				return err
			}

			hdr.Linkname = target
			hdr.Name = destFile
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		} else if stat.Mode()&os.ModeSocket != 0 {
			logger.Warningf("socket file %q ignored", fpath)
		} else {
			//case regular file or other file type like pipe
			hdr, err := tar.FileInfoHeader(stat, fpath)
			if err != nil {
				return err
			}
			hdr.Name = destFile

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			return f.Close()
		}
	}
	return nil
}
