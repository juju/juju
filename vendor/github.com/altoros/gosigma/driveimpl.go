// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"errors"

	"github.com/altoros/gosigma/data"
)

func (d drive) clone(params CloneParams, avoid []string) (*data.Drive, error) {
	obj, err := d.client.cloneDrive(d.UUID(), d.Library(), params, avoid)
	if err != nil {
		return nil, err
	}

	if d.Library() == LibraryMedia {
		obj.LibraryDrive = d.obj.LibraryDrive
	}

	return obj, nil
}

func (d *drive) resize(newSize uint64) error {
	// if drive object contains only UUID, we need to refresh it
	if d.Status() == "" {
		if err := d.Refresh(); err != nil {
			return err
		}
	}

	// check the current drive size
	if uint64(d.Size()) == newSize {
		return nil
	}

	// check the library
	if d.Library() == LibraryMedia {
		return errors.New("can not resize drive from media library")
	}

	// check the drive status
	if d.Status() != DriveUnmounted {
		return errors.New("drive size can be changed only for unmounted drives")
	}

	// do the resize
	obj, err := d.client.resizeDrive(*d.obj, newSize)
	if err != nil {
		return err
	}

	// update drive data
	d.obj = obj

	return nil
}
