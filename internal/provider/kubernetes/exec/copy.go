// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
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

// Copy copies files/directories from host to a pod or from a pod to host.
func (c client) Copy(ctx context.Context, params CopyParams, cancel <-chan struct{}) error {
	if err := params.validate(); err != nil {
		return errors.Trace(err)
	}
	if params.Src.PodName != "" {
		return c.copyFromPod(ctx, params, cancel)
	}
	if params.Dest.PodName != "" {
		return c.copyToPod(ctx, params, cancel)
	}
	return errors.NewNotValid(nil, "either copy from a pod or to a pod")
}

func (c client) copyFromPod(ctx context.Context, params CopyParams, cancel <-chan struct{}) error {
	src := params.Src
	dest := params.Dest
	logger.Debugf(context.TODO(), "copying from %v to %v", src, dest)

	reader, writer := c.pipGetter()
	var stderr bytes.Buffer
	execParams := ExecParams{
		PodName:       src.PodName,
		ContainerName: src.ContainerName,
		Commands:      []string{"tar", "cf", "-", src.Path},
		Stdin:         nil,
		Stdout:        writer,
		Stderr:        &stderr,
	}

	go func() {
		defer writer.Close()
		if err := c.Exec(ctx, execParams, cancel); err != nil {
			logger.Errorf(context.TODO(), "make tar %q failed: %v", src.Path, err)
		}
	}()
	prefix := getPrefix(src.Path)
	prefix = path.Clean(prefix)
	// remove extraneous path shortcuts - these could occur if a path contained extra "../"
	// and attempted to navigate beyond "/" in a remote filesystem.
	prefix = stripPathShortcuts(prefix)
	return unTarAll(src, reader, dest.Path, prefix)
}

func getPrefix(file string) string {
	// tar strips the leading '/' if it's there, so we will too.
	return strings.TrimLeft(file, "/")
}

// stripPathShortcuts removes any leading or trailing "../" from a given path.
func stripPathShortcuts(p string) string {
	newPath := path.Clean(p)
	trimmed := strings.TrimPrefix(newPath, "../")

	for trimmed != newPath {
		newPath = trimmed
		trimmed = strings.TrimPrefix(newPath, "../")
	}

	// trim leftover {".", ".."}
	if newPath == "." || newPath == ".." {
		newPath = ""
	}

	if len(newPath) > 0 && string(newPath[0]) == "/" {
		return newPath[1:]
	}

	return newPath
}

// isDestRelative returns true if dest is pointing outside the base directory,
// false otherwise.
func isDestRelative(base, dest string) bool {
	relative, err := filepath.Rel(base, dest)
	if err != nil {
		return false
	}
	return relative == "." || relative == stripPathShortcuts(relative)
}

// this is inspired by kubectl cmd package.
// - https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go
func (c client) copyToPod(ctx context.Context, params CopyParams, cancel <-chan struct{}) (err error) {
	src := params.Src
	dest := params.Dest
	logger.Debugf(context.TODO(), "copying from %v to %v", src, dest)

	if _, err = os.Stat(src.Path); err != nil {
		return errors.NewNotValid(nil, fmt.Sprintf("%q does not exist on local", src.Path))
	}

	if dest.Path != "/" && strings.HasSuffix(dest.Path, "/") {
		dest.Path = strings.TrimSuffix(dest.Path, "/")
	}

	if err = c.checkRemotePathIsDir(ctx, dest, cancel); err == nil {
		dest.Path = path.Join(dest.Path, path.Base(src.Path))
	}

	reader, writer := c.pipGetter()

	go func() {
		defer writer.Close()

		if err := makeTar(src.Path, dest.Path, writer); err != nil {
			logger.Errorf(context.TODO(), "make tar %q failed: %v", src.Path, err)
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
	return errors.Trace(c.Exec(ctx, execParams, cancel))
}

func (c client) checkRemotePathIsDir(ctx context.Context, rec FileResource, cancel <-chan struct{}) error {
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
	return errors.Trace(c.Exec(ctx, execParams, cancel))
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
			files, err := os.ReadDir(fpath)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				// case empty directory.
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
			// case soft link.
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
			logger.Warningf(context.TODO(), "socket file %q ignored", fpath)
		} else {
			// case regular file or other file type like pipe.
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

			if _, err := io.Copy(tw, f); err != nil {
				_ = f.Close()
				return err
			}
			return f.Close()
		}
	}
	return nil
}

func unTarAll(src FileResource, reader io.Reader, destDir, prefix string) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err != io.EOF {
				return errors.Trace(err)
			}
			break
		}

		// All the files will start with the prefix, which is the directory where
		// they were located on the pod, we need to strip down that prefix, but
		// if the prefix is missing it means the tar was tempered with.
		// For the case where prefix is empty we need to ensure that the path
		// is not absolute, which also indicates the tar file was tampered with.
		if !strings.HasPrefix(header.Name, prefix) {
			return errors.New("tar contents corrupted")
		}

		// basic file information.
		mode := header.FileInfo().Mode()
		destFileName := filepath.Join(destDir, header.Name[len(prefix):])

		if !isDestRelative(destDir, destFileName) {
			logger.Warningf(context.TODO(), "file %q is outside target destination, skipping", destFileName)
			continue
		}

		baseName := filepath.Dir(destFileName)
		if err := os.MkdirAll(baseName, 0755); err != nil {
			return errors.Trace(err)
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(destFileName, 0755); err != nil {
				return errors.Trace(err)
			}
			continue
		}

		if mode&os.ModeSymlink != 0 {
			logger.Warningf(context.TODO(), "skipping symlink: %q -> %q", destFileName, header.Linkname)
			continue
		}
		outFile, err := os.Create(destFileName)
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := io.Copy(outFile, tarReader); err != nil {
			_ = outFile.Close()
			return errors.Trace(err)
		}
		if err := outFile.Close(); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}
