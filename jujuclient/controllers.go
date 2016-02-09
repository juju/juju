// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
)

// AllControllers implements ControllersGetter.AllControllers.
// This implementation gets all controllers defined in the controllers file.
func (s *store) AllControllers() (map[string]ControllerDetails, error) {
	lock, err := s.lock("read-all-controllers")
	if err != nil {
		return nil, errors.Annotate(err, "cannot read all controllers")
	}
	defer s.unlock(lock)
	return ReadControllersFile(JujuControllersPath())
}

// ControllerByName implements ControllersGetter.ControllerByName.
func (s *store) ControllerByName(name string) (*ControllerDetails, error) {
	lock, err := s.lock("read-controller-by-name")
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read controller %v", name)
	}
	defer s.unlock(lock)

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result, ok := controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController implements ControllersUpdater.UpdateController.
// Once controllers collection is updated, controllers file is written.
func (s *store) UpdateController(name string, one ControllerDetails) error {
	if err := ValidateControllerDetails(name, one); err != nil {
		return err
	}

	lock, err := s.lock("update-controller")
	if err != nil {
		return errors.Annotatef(err, "cannot update controller %v", name)
	}
	defer s.unlock(lock)

	all, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	if len(all) == 0 {
		all = make(map[string]ControllerDetails)
	}

	all[name] = one
	return WriteControllersFile(all)
}

// RemoveController implements ControllersRemover.RemoveController
// Once controllers collection is updated, controllers file is written.
func (s *store) RemoveController(name string) error {
	lock, err := s.lock("remove-controller")
	if err != nil {
		return errors.Annotatef(err, "cannot remove controller %v", name)
	}
	defer s.unlock(lock)

	all, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	delete(all, name)
	return WriteControllersFile(all)
}

// ValidateControllerDetails ensures that given controller details are valid.
func ValidateControllerDetails(name string, c ControllerDetails) error {
	if name == "" {
		return errors.NotValidf("missing name, controller info")
	}
	if c.ControllerUUID == "" {
		return errors.NotValidf("missing uuid, controller info")
	}
	if c.CACert == "" {
		return errors.NotValidf("missing ca-cert, controller info")
	}
	return nil
}
