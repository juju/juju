// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storetesting // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting"

import (
	"flag"
	"os"

	jujutesting "github.com/juju/testing"
)

var noTestMongoJs *bool = flag.Bool("notest-mongojs", false, "Disable MongoDB tests that require JavaScript")

func init() {
	if os.Getenv("JUJU_NOTEST_MONGOJS") == "1" || jujutesting.MgoServer.WithoutV8 {
		*noTestMongoJs = true
	}
}

// MongoJSEnabled reports whether testing code should run tests
// that rely on JavaScript inside MongoDB.
func MongoJSEnabled() bool {
	return !*noTestMongoJs
}
