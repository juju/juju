// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filecopy

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.internal.filecopy")

// MakeTar writes srcPath to writer as a tar archive rooted at destPath.
//
// This is based on code from
// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go.
func MakeTar(srcPath, destPath string, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()
	srcPath = path.Clean(srcPath)
	destPath = path.Clean(destPath)
	return recursiveTar(path.Dir(srcPath), path.Base(srcPath), path.Dir(destPath), path.Base(destPath), tarWriter)
}

// recursiveTar is based on code from
// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/cp/cp.go.
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

// UntarAll extracts an archive created from srcPath into destDir.
func UntarAll(srcPath string, reader io.Reader, destDir string) error {
	prefix := getPrefix(srcPath)
	prefix = path.Clean(prefix)
	// Remove extraneous path shortcuts - these could occur if a path contained
	// extra "../" and attempted to navigate beyond "/" in a remote filesystem.
	prefix = stripPathShortcuts(prefix)

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
		// if the prefix is missing it means the tar was tampered with.
		// For the case where prefix is empty we need to ensure that the path
		// is not absolute, which also indicates that the tar was tampered with.
		if !strings.HasPrefix(header.Name, prefix) {
			return errors.New("tar contents corrupted")
		}

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
