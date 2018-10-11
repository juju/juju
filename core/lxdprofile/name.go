package lxdprofile

import (
	"fmt"
)

// PrefixName defines what the prefix for the name will be
const PrefixName = "juju"

// Name returns a serialisable name that we can use to identify profiles
// juju-<model>-<application>-<charm-revision>
func Name(modeName, appName string, revision int) string {
	return fmt.Sprintf("%s-%s-%s-%d", PrefixName, modeName, appName, revision)
}
