package lxdprofile

import (
	"fmt"

	"github.com/juju/juju/juju/names"
)

// Name returns a serialisable name that we can use to identify profiles
// juju-<model>-<application>-<charm-revision>
func Name(modelName, appName string, revision int) string {
	return fmt.Sprintf("%s-%s-%s-%d", names.Juju, modelName, appName, revision)
}
