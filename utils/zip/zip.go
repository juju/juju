// Copyright 2011, 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package zip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Walk calls the supplied callback with every file in the supplied zip reader.
// If the callback ever returns an error, the process will abort.
func Walk(reader *zip.Reader, callback func(zipFile *zip.File) error) error {
	for _, zipFile := range reader.File {
		if err := callback(zipFile); err != nil {
			cleanName := path.Clean(zipFile.Name)
			return fmt.Errorf("cannot process %q: %v", cleanName, err)
		}
	}
	return nil
}

// FindAll returns the cleaned path of every file in the supplied zip reader.
func FindAll(reader *zip.Reader) ([]string, error) {
	return Find(reader, "*")
}

// Find returns the cleaned path of every file in the supplied zip reader whose
// base name matches the supplied pattern, which is interpreted as in path.Check.
func Find(reader *zip.Reader, pattern string) ([]string, error) {
	// path.Match will only return an error if the pattern is not
	// valid (*and* the supplied name is not empty, hence "check").
	if _, err := path.Match(pattern, "check"); err != nil {
		return nil, err
	}
	var matches []string
	callback := func(zipFile *zip.File) error {
		cleanPath := path.Clean(zipFile.Name)
		baseName := path.Base(cleanPath)
		if match, _ := path.Match(pattern, baseName); match {
			matches = append(matches, cleanPath)
		}
		return nil
	}
	// callback never returns an error, so nor will Walk.
	Walk(reader, callback)
	return matches, nil
}

// ExtractAll extracts the supplied zip reader to the target path, overwriting
// existing files and directories only where necessary.
func ExtractAll(reader *zip.Reader, targetPath string) error {
	return Extract(reader, targetPath, "")
}

// Extract extracts the supplied zip reader to the target path, omitting files
// not rooted at the source path, and overwriting existing files and directories
// only where necessary. If the source path identifies a single file, it will be
// extracted to the target path directly.
func Extract(reader *zip.Reader, targetPath, sourcePath string) error {
	sourcePath = path.Clean(sourcePath)
	if sourcePath == "." {
		sourcePath = ""
	} else if !isSanePath(sourcePath) {
		return fmt.Errorf("cannot extract files rooted at %q", sourcePath)
	}
	return Walk(reader, extractor{targetPath, sourcePath}.expand)
}

type extractor struct {
	targetPath string
	sourcePath string
}

func (x extractor) path(zipFile *zip.File) (string, bool) {
	cleanPath := path.Clean(zipFile.Name)
	if !strings.HasPrefix(cleanPath, x.sourcePath) {
		return "", false
	}
	relativePath := cleanPath[len(x.sourcePath):]
	if strings.HasPrefix(relativePath, "/") {
		relativePath = relativePath[1:]
	}
	return filepath.Join(x.targetPath, filepath.FromSlash(relativePath)), true
}

func (x extractor) expand(zipFile *zip.File) error {
	filePath, ok := x.path(zipFile)
	if !ok {
		return nil
	}
	parentPath := filepath.Dir(filePath)
	if err := os.MkdirAll(parentPath, os.ModePerm); err != nil {
		return err
	}
	mode := zipFile.Mode()
	modePerm := mode & os.ModePerm
	modeType := mode & os.ModeType
	switch modeType {
	case os.ModeDir:
		return x.writeDir(filePath, modePerm)
	case os.ModeSymlink:
		return x.writeSymlink(filePath, zipFile)
	case 0:
		return x.writeFile(filePath, zipFile, modePerm)
	}
	return fmt.Errorf("unknown file type %o", modeType)
}

func (x extractor) writeDir(filePath string, modePerm os.FileMode) error {
	fileInfo, err := os.Lstat(filePath)
	switch {
	case err == nil:
		mode := fileInfo.Mode()
		if mode.IsDir() {
			if mode&os.ModePerm != modePerm {
				return os.Chmod(filePath, modePerm)
			}
			return nil
		}
		fallthrough
	case !os.IsNotExist(err):
		if err := os.RemoveAll(filePath); err != nil {
			return err
		}
	}
	return os.MkdirAll(filePath, modePerm)
}

func (x extractor) writeFile(filePath string, zipFile *zip.File, modePerm os.FileMode) error {
	if _, err := os.Lstat(filePath); !os.IsNotExist(err) {
		if err := os.RemoveAll(filePath); err != nil {
			return err
		}
	}
	writer, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, modePerm)
	if err != nil {
		return err
	}
	return readTo(writer, zipFile)
}

func (x extractor) writeSymlink(filePath string, zipFile *zip.File) error {
	targetPath, err := x.checkSymlink(filePath, zipFile)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(filePath); !os.IsNotExist(err) {
		if err := os.RemoveAll(filePath); err != nil {
			return err
		}
	}
	return os.Symlink(targetPath, filePath)
}

func (x extractor) checkSymlink(filePath string, zipFile *zip.File) (string, error) {
	var buffer bytes.Buffer
	if err := readTo(&buffer, zipFile); err != nil {
		return "", err
	}
	targetPath := buffer.String()
	if filepath.IsAbs(targetPath) {
		return "", fmt.Errorf("symlink %q is absolute", targetPath)
	}
	finalPath := filepath.Join(filepath.Dir(filePath), targetPath)
	relativePath, err := filepath.Rel(x.targetPath, finalPath)
	if err != nil {
		// Not tested, because I don't know how to trigger this condition.
		return "", fmt.Errorf("symlink %q not comprehensible", targetPath)
	}
	if !isSanePath(relativePath) {
		return "", fmt.Errorf("symlink %q leads out of scope", targetPath)
	}
	return targetPath, nil
}

func readTo(writer io.Writer, zipFile *zip.File) error {
	reader, err := zipFile.Open()
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, reader)
	reader.Close()
	return err
}

func isSanePath(path string) bool {
	if path == ".." || strings.HasPrefix(path, "../") {
		return false
	}
	return true
}
