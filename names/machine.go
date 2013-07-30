package names

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ContainerSnippet     = "(/[a-z]+/" + NumberSnippet + ")"
	MachineSnippet       = NumberSnippet + ContainerSnippet + "*"
	ContainerSpecSnippet = "(([a-z])+:)?"
)

var (
	validMachine               = regexp.MustCompile("^" + MachineSnippet + "$")
	validMachineOrNewContainer = regexp.MustCompile("^" + ContainerSpecSnippet + MachineSnippet + "$")
)

// IsMachineId returns whether id is a valid machine id.
func IsMachineId(id string) bool {
	return validMachine.MatchString(id)
}

// IsMachineOrNewContainer returns whether spec is a valid machine id
// or new container definition.
func IsMachineOrNewContainer(spec string) bool {
	return validMachineOrNewContainer.MatchString(spec)
}

// MachineTag returns the tag for the machine with the given id.
func MachineTag(id string) string {
	tag := fmt.Sprintf("%s%s", MachineTagPrefix, id)
	// Containers require "/" to be replaced by "-".
	tag = strings.Replace(tag, "/", "-", -1)
	return tag
}

// MachineIdFromTag returns the machine id that was used to create the
// tag, or an error if it's not the tag of a machine.
func MachineIdFromTag(tag string) (string, error) {
	if !strings.HasPrefix(tag, MachineTagPrefix) {
		return "", fmt.Errorf("%q is not a valid machine tag", tag)
	}
	// Strip off the "machine-" prefix.
	id := tag[len(MachineTagPrefix):]
	// Put the slashes back.
	id = strings.Replace(id, "-", "/", -1)
	if !IsMachineId(id) {
		return "", fmt.Errorf("%q is not a valid machine tag", tag)
	}
	return id, nil
}
