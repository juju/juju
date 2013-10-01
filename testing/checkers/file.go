// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"os"
	"reflect"

	gc "launchpad.net/gocheck"
)

// IsNonEmptyFile checker

type isNonEmptyFileChecker struct {
	*gc.CheckerInfo
}

var IsNonEmptyFile gc.Checker = &isNonEmptyFileChecker{
	&gc.CheckerInfo{Name: "IsNonEmptyFile", Params: []string{"obtained"}},
}

func (checker *isNonEmptyFileChecker) Check(params []interface{}, names []string) (result bool, error string) {
	filename, isString := stringOrStringer(params[0])
	if isString {
		fileInfo, err := os.Stat(filename)
		if os.IsNotExist(err) {
			return false, fmt.Sprintf("%s does not exist", filename)
		} else if err != nil {
			return false, fmt.Sprintf("other stat error: %v", err)
		}
		if fileInfo.Size() > 0 {
			return true, ""
		} else {
			return false, fmt.Sprintf("%s is empty", filename)
		}
	}

	value := reflect.ValueOf(params[0])
	return false, fmt.Sprintf("obtained value is not a string and has no .String(), %s:%#v", value.Kind(), params[0])
}

// IsDirectory checker

type isDirectoryChecker struct {
	*gc.CheckerInfo
}

var IsDirectory gc.Checker = &isDirectoryChecker{
	&gc.CheckerInfo{Name: "IsDirectory", Params: []string{"obtained"}},
}

func (checker *isDirectoryChecker) Check(params []interface{}, names []string) (result bool, error string) {
	path, isString := stringOrStringer(params[0])
	if isString {
		fileInfo, err := os.Stat(path)
		if os.IsNotExist(err) {
			return false, fmt.Sprintf("%s does not exist", path)
		} else if err != nil {
			return false, fmt.Sprintf("other stat error: %v", err)
		}
		if fileInfo.IsDir() {
			return true, ""
		} else {
			return false, fmt.Sprintf("%s is not a directory", path)
		}
	}

	value := reflect.ValueOf(params[0])
	return false, fmt.Sprintf("obtained value is not a string and has no .String(), %s:%#v", value.Kind(), params[0])
}

// IsSymlink checker

type isSymlinkChecker struct {
	*gc.CheckerInfo
}

var IsSymlink gc.Checker = &isSymlinkChecker{
	&gc.CheckerInfo{Name: "IsSymlink", Params: []string{"obtained"}},
}

func (checker *isSymlinkChecker) Check(params []interface{}, names []string) (result bool, error string) {
	path, isString := stringOrStringer(params[0])
	if isString {
		fileInfo, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return false, fmt.Sprintf("%s does not exist", path)
		} else if err != nil {
			return false, fmt.Sprintf("other stat error: %v", err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return true, ""
		} else {
			return false, fmt.Sprintf("%s is not a symlink: %+v", path, fileInfo)
		}
	}

	value := reflect.ValueOf(params[0])
	return false, fmt.Sprintf("obtained value is not a string and has no .String(), %s:%#v", value.Kind(), params[0])
}

// DoesNotExist checker makes sure the path specified doesn't exist.

type doesNotExistChecker struct {
	*gc.CheckerInfo
}

var DoesNotExist gc.Checker = &doesNotExistChecker{
	&gc.CheckerInfo{Name: "DoesNotExist", Params: []string{"obtained"}},
}

func (checker *doesNotExistChecker) Check(params []interface{}, names []string) (result bool, error string) {
	path, isString := stringOrStringer(params[0])
	if isString {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return true, ""
		} else if err != nil {
			return false, fmt.Sprintf("other stat error: %v", err)
		}
		return false, fmt.Sprintf("%s exists", path)
	}

	value := reflect.ValueOf(params[0])
	return false, fmt.Sprintf("obtained value is not a string and has no .String(), %s:%#v", value.Kind(), params[0])
}

// SymlinkDoesNotExist checker makes sure the path specified doesn't exist.

type symlinkDoesNotExistChecker struct {
	*gc.CheckerInfo
}

var SymlinkDoesNotExist gc.Checker = &symlinkDoesNotExistChecker{
	&gc.CheckerInfo{Name: "SymlinkDoesNotExist", Params: []string{"obtained"}},
}

func (checker *symlinkDoesNotExistChecker) Check(params []interface{}, names []string) (result bool, error string) {
	path, isString := stringOrStringer(params[0])
	if isString {
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return true, ""
		} else if err != nil {
			return false, fmt.Sprintf("other stat error: %v", err)
		}
		return false, fmt.Sprintf("%s exists", path)
	}

	value := reflect.ValueOf(params[0])
	return false, fmt.Sprintf("obtained value is not a string and has no .String(), %s:%#v", value.Kind(), params[0])
}
