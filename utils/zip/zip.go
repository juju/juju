// Copyright 2011-2014 Canonical Ltd.
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

// FindAll returns the cleaned path of every file in the supplied zip reader.
func FindAll(reader *zip.Reader) ([]string, error) {
	return Find(reader, "*")
}

// Find returns the cleaned path of every file in the supplied zip reader whose
// base name matches the supplied pattern, which is interpreted as in path.Match.
func Find(reader *zip.Reader, pattern string) ([]string, error) {
	// path.Match will only return an error if the pattern is not
	// valid (*and* the supplied name is not empty, hence "check").
	if _, err := path.Match(pattern, "check"); err != nil {
		return nil, err
	}
	var matches []string
	for _, zipFile := range reader.File {
		cleanPath := path.Clean(zipFile.Name)
		baseName := path.Base(cleanPath)
		if match, _ := path.Match(pattern, baseName); match {
			matches = append(matches, cleanPath)
		}
	}
	return matches, nil
}

// ExtractAll extracts the supplied zip reader to the target path, overwriting
// existing files and directories only where necessary.
func ExtractAll(reader *zip.Reader, targetRoot string) error {
	return Extract(reader, targetRoot, "")
}

// Extract extracts files from the supplied zip reader, from the (internal, slash-
// separated) source path into the (external, OS-specific) target path. If the
// source path does not reference a directory, the referenced file will be written
// directly to the target path.
func Extract(reader *zip.Reader, targetRoot, sourceRoot string) error {
	sourceRoot = path.Clean(sourceRoot)
	if sourceRoot == "." {
		sourceRoot = ""
	}
	if !isSanePath(sourceRoot) {
		return fmt.Errorf("cannot extract files rooted at %q", sourceRoot)
	}
	extractor := extractor{targetRoot, sourceRoot}
	for _, zipFile := range reader.File {
		if err := extractor.extract(zipFile); err != nil {
			cleanName := path.Clean(zipFile.Name)
			return fmt.Errorf("cannot extract %q: %v", cleanName, err)
		}
	}
	return nil
}

type extractor struct {
	targetRoot string
	sourceRoot string
}

// targetPath returns the target path for a given zip file and whether
// it should be extracted.
func (x extractor) targetPath(zipFile *zip.File) (string, bool) {
	cleanPath := path.Clean(zipFile.Name)
	if cleanPath == x.sourceRoot {
		return x.targetRoot, true
	}
	if x.sourceRoot != "" {
		mustPrefix := x.sourceRoot + "/"
		if !strings.HasPrefix(cleanPath, mustPrefix) {
			return "", false
		}
		cleanPath = cleanPath[len(mustPrefix):]
	}
	return filepath.Join(x.targetRoot, filepath.FromSlash(cleanPath)), true
}

func (x extractor) extract(zipFile *zip.File) error {
	targetPath, ok := x.targetPath(zipFile)
	if !ok {
		return nil
	}
	parentPath := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentPath, 0777); err != nil {
		return err
	}
	mode := zipFile.Mode()
	modePerm := mode & os.ModePerm
	modeType := mode & os.ModeType
	switch modeType {
	case os.ModeDir:
		return x.writeDir(targetPath, modePerm)
	case os.ModeSymlink:
		return x.writeSymlink(targetPath, zipFile)
	case 0:
		return x.writeFile(targetPath, zipFile, modePerm)
	}
	return fmt.Errorf("unknown file type %d", modeType)
}

func (x extractor) writeDir(targetPath string, modePerm os.FileMode) error {
	fileInfo, err := os.Lstat(targetPath)
	switch {
	case err == nil:
		mode := fileInfo.Mode()
		if mode.IsDir() {
			if mode&os.ModePerm != modePerm {
				return os.Chmod(targetPath, modePerm)
			}
			return nil
		}
		fallthrough
	case !os.IsNotExist(err):
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	}
	return os.MkdirAll(targetPath, modePerm)
}

func (x extractor) writeFile(targetPath string, zipFile *zip.File, modePerm os.FileMode) error {
	if _, err := os.Lstat(targetPath); !os.IsNotExist(err) {
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	}
	writer, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, modePerm)
	if err != nil {
		return err
	}
	defer writer.Close()
	return copyTo(writer, zipFile)
}

func (x extractor) writeSymlink(targetPath string, zipFile *zip.File) error {
	symlinkTarget, err := x.checkSymlink(targetPath, zipFile)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(targetPath); !os.IsNotExist(err) {
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	}
	return os.Symlink(symlinkTarget, targetPath)
}

func (x extractor) checkSymlink(targetPath string, zipFile *zip.File) (string, error) {
	var buffer bytes.Buffer
	if err := copyTo(&buffer, zipFile); err != nil {
		return "", err
	}
	symlinkTarget := buffer.String()
	if filepath.IsAbs(symlinkTarget) {
		return "", fmt.Errorf("symlink %q is absolute", symlinkTarget)
	}
	finalPath := filepath.Join(filepath.Dir(targetPath), symlinkTarget)
	relativePath, err := filepath.Rel(x.targetRoot, finalPath)
	if err != nil {
		// Not tested, because I don't know how to trigger this condition.
		return "", fmt.Errorf("symlink %q not comprehensible", symlinkTarget)
	}
	if !isSanePath(relativePath) {
		return "", fmt.Errorf("symlink %q leads out of scope", symlinkTarget)
	}
	return symlinkTarget, nil
}

func copyTo(writer io.Writer, zipFile *zip.File) error {
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
