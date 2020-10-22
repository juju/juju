// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"strings"

	"github.com/juju/juju/core/instance"
)

// ParentId returns the id of the host machine if machineId a container id, or ""
// if machineId is not for a container.
func ParentId(machineId string) string {
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return ""
	}
	return strings.Join(idParts[:len(idParts)-2], "/")
}

// ContainerTypeFromId returns the container type if machineId is a container id, or ""
// if machineId is not for a container.
func ContainerTypeFromId(machineId string) instance.ContainerType {
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return instance.ContainerType("")
	}
	return instance.ContainerType(idParts[len(idParts)-2])
}

// NestingLevel returns how many levels of nesting exist for a machine id.
func NestingLevel(machineId string) int {
	idParts := strings.Split(machineId, "/")
	return (len(idParts) - 1) / 2
}

// TopParentId returns the id of the top level host machine for a container id.
func TopParentId(machineId string) string {
	idParts := strings.Split(machineId, "/")
	return idParts[0]
}
