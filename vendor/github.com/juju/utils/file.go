// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
)

// UserHomeDir returns the home directory for the specified user, or the
// home directory for the current user if the specified user is empty.
func UserHomeDir(userName string) (hDir string, err error) {
	if userName == "" {
		// TODO (wallyworld) - fix tests on Windows
		// Ordinarily, we'd always use user.Current() to get the current user
		// and then get the HomeDir from that. But our tests rely on poking
		// a value into $HOME in order to override the normal home dir for the
		// current user. So we're forced to use Home() to make the tests pass.
		// All of our tests currently construct paths with the default user in
		// mind eg "~/foo".
		return Home(), nil
	}
	hDir, err = homeDir(userName)
	if err != nil {
		return "", err
	}
	return hDir, nil
}

// Only match paths starting with ~ (~user/test, ~/test). This will prevent
// accidental expansion on Windows when short form paths are present (C:\users\ADMINI~1\test)
var userHomePathRegexp = regexp.MustCompile("(^~(?P<user>[^/]*))(?P<path>.*)")

// NormalizePath expands a path containing ~ to its absolute form,
// and removes any .. or . path elements.
func NormalizePath(dir string) (string, error) {
	if userHomePathRegexp.MatchString(dir) {
		user := userHomePathRegexp.ReplaceAllString(dir, "$user")
		userHomeDir, err := UserHomeDir(user)
		if err != nil {
			return "", err
		}
		dir = userHomePathRegexp.ReplaceAllString(dir, fmt.Sprintf("%s$path", userHomeDir))
	}
	return filepath.Clean(dir), nil
}

// EnsureBaseDir ensures that path is always prefixed by baseDir,
// allowing for the fact that path might have a Window drive letter in
// it.
func EnsureBaseDir(baseDir, path string) string {
	if baseDir == "" {
		return path
	}
	volume := filepath.VolumeName(path)
	return filepath.Join(baseDir, path[len(volume):])
}

// JoinServerPath joins any number of path elements into a single path, adding
// a path separator (based on the current juju server OS) if necessary. The
// result is Cleaned; in particular, all empty strings are ignored.
func JoinServerPath(elem ...string) string {
	return path.Join(elem...)
}

// UniqueDirectory returns "path/name" if that directory doesn't exist.  If it
// does, the method starts appending .1, .2, etc until a unique name is found.
func UniqueDirectory(path, name string) (string, error) {
	dir := filepath.Join(path, name)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return dir, nil
	}
	for i := 1; ; i++ {
		dir := filepath.Join(path, fmt.Sprintf("%s.%d", name, i))
		_, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return dir, nil
		} else if err != nil {
			return "", err
		}
	}
}

// CopyFile writes the contents of the given source file to dest.
func CopyFile(dest, source string) error {
	df, err := os.Create(dest)
	if err != nil {
		return err
	}
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(df, f)
	return err
}

// AtomicWriteFileAndChange atomically writes the filename with the
// given contents and calls the given function after the contents were
// written, but before the file is renamed.
func AtomicWriteFileAndChange(filename string, contents []byte, change func(*os.File) error) (err error) {
	dir, file := filepath.Split(filename)
	f, err := ioutil.TempFile(dir, file)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %v", err)
	}
	defer f.Close()
	defer func() {
		if err != nil {
			// Don't leave the temp file lying around on error.
			// Close the file before removing. Trying to remove an open file on
			// Windows will fail.
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err := f.Write(contents); err != nil {
		return fmt.Errorf("cannot write %q contents: %v", filename, err)
	}
	if err := change(f); err != nil {
		return err
	}
	f.Close()
	if err := ReplaceFile(f.Name(), filename); err != nil {
		return fmt.Errorf("cannot replace %q with %q: %v", f.Name(), filename, err)
	}
	return nil
}

// AtomicWriteFile atomically writes the filename with the given
// contents and permissions, replacing any existing file at the same
// path.
func AtomicWriteFile(filename string, contents []byte, perms os.FileMode) (err error) {
	return AtomicWriteFileAndChange(filename, contents, func(f *os.File) error {
		// FileMod.Chmod() is not implemented on Windows, however, os.Chmod() is
		if err := os.Chmod(f.Name(), perms); err != nil {
			return fmt.Errorf("cannot set permissions: %v", err)
		}
		return nil
	})
}
