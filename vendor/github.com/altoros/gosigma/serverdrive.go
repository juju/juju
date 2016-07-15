// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"
	"strings"

	"github.com/altoros/gosigma/data"
)

// A ServerDrive interface represents drive, connected to server instance
type ServerDrive interface {
	// CloudSigma resource
	Resource

	// BootOrder of drive
	BootOrder() int

	// Channel of drive
	Channel() string

	// Device name of drive
	Device() string

	// Drive object. Note, returned Drive object carries only UUID and URI, so it needs
	// to perform Drive.Refresh to access additional information.
	Drive() Drive
}

// A serverDrive implements drive, connected to server instance
type serverDrive struct {
	client *Client
	obj    *data.ServerDrive
}

var _ ServerDrive = serverDrive{}

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (sd serverDrive) String() string {
	return fmt.Sprintf(`{BootOrder: %d, Channel: %q, Device: %q, UUID: %q}`,
		sd.BootOrder(), sd.Channel(), sd.Device(), sd.UUID())
}

// URI of instance
func (sd serverDrive) URI() string { return sd.obj.Drive.URI }

// UUID of drive
func (sd serverDrive) UUID() string { return sd.obj.Drive.UUID }

// BootOrder of drive
func (sd serverDrive) BootOrder() int { return sd.obj.BootOrder }

// Channel of drive
func (sd serverDrive) Channel() string { return sd.obj.Channel }

// Device name of drive
func (sd serverDrive) Device() string { return sd.obj.Device }

// Drive object. Note, returned Drive object carries only UUID and URI, so it needs
// to perform Drive.Refresh to access additional information.
func (sd serverDrive) Drive() Drive {
	obj := data.Drive{Resource: sd.obj.Drive}
	libdrive := strings.Contains(sd.obj.Drive.UUID, "libdrives")
	if libdrive {
		return &drive{sd.client, &obj, LibraryMedia}
	}
	return &drive{sd.client, &obj, LibraryAccount}
}
