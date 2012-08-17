// This file enables juju executables to be built with a different
// version to the usual current version, enabling tests to test
// upgrading logic without actually creating a new version of the source
// code.

// +build bumpversion
package version

func init() {
	Current.Patch++
}
