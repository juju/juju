// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package version contains versioning information for juju.  It also
// acts as guardian of the current client Juju version number.
package version

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	semversion "github.com/juju/version"
)

// The presence and format of this constant is very important.
// The debian/rules build recipe uses this value for the version
// number of the release package.
const version = "2.0-rc2"

// The version that we switched over from old style numbering to new style.
var switchOverVersion = semversion.MustParse("1.19.9")

// Current gives the current version of the system.  If the file
// "FORCE-VERSION" is present in the same directory as the running
// binary, it will override this.
var Current = semversion.MustParse(version)

var Compiler = runtime.Compiler

func init() {
	toolsDir := filepath.Dir(os.Args[0])
	v, err := ioutil.ReadFile(filepath.Join(toolsDir, "FORCE-VERSION"))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "WARNING: cannot read forced version: %v\n", err)
		}
		return
	}
	Current = semversion.MustParse(strings.TrimSpace(string(v)))
}

func isOdd(x int) bool {
	return x%2 != 0
}

// IsDev returns whether the version represents a development version. A
// version with a tag or a nonzero build component is considered to be a
// development version.  Versions older than or equal to 1.19.3 (the switch
// over time) check for odd minor versions.
func IsDev(v semversion.Number) bool {
	if v.Compare(switchOverVersion) <= 0 {
		return isOdd(v.Minor) || v.Build > 0
	}
	return v.Tag != "" || v.Build > 0
}
