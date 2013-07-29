package names

import (
	"fmt"
	"regexp"
	"strings"
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

// MachineTag returns the tag for the machine wi th the given id.
func MachineTag(id string) string {
	tag := fmt.Sprintf("%s%s", MachineTagPrefix, id)
	// Containers require "/" to be replaced by "-".
	tag = strings.Replace(tag, "/", "-", -1)
	return tag
}

// MachineIdFromTag returns the machine id that was used to create the
// tag.
func MachineIdFromTag(tag string) (string, error) {
	if !strings.HasPrefix(tag, MachineTagPrefix) {
		return "", fmt.Errorf("invalid machine tag format: %v", tag)
	}
	// Strip off the "machine-" prefix.
	id := tag[len(MachineTagPrefix):]
	// Put the slashes back.
	id = strings.Replace(id, "-", "/", -1)
	return id, nil
}
