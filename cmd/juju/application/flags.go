// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/storage"
)

type storageFlag struct {
	stores       *map[string]storage.Constraints
	bundleStores *map[string]map[string]storage.Constraints
}

// Set implements gnuflag.Value.Set.
func (f storageFlag) Set(s string) error {
	fields := strings.SplitN(s, "=", 2)
	if len(fields) < 2 {
		if f.bundleStores != nil {
			return errors.New("expected [<application>:]<store>=<constraints>")
		}
		return errors.New("expected <store>=<constraints>")
	}
	var applicationName, storageName string
	if colon := strings.IndexRune(fields[0], ':'); colon >= 0 {
		if f.bundleStores == nil {
			return errors.New("expected <store>=<constraints>")
		}
		applicationName = fields[0][:colon]
		storageName = fields[0][colon+1:]
	} else {
		storageName = fields[0]
	}
	cons, err := storage.ParseConstraints(fields[1])
	if err != nil {
		return errors.Annotate(err, "cannot parse disk constraints")
	}
	var stores map[string]storage.Constraints
	if applicationName != "" {
		if *f.bundleStores == nil {
			*f.bundleStores = make(map[string]map[string]storage.Constraints)
		}
		stores = (*f.bundleStores)[applicationName]
		if stores == nil {
			stores = make(map[string]storage.Constraints)
			(*f.bundleStores)[applicationName] = stores
		}
	} else {
		if *f.stores == nil {
			*f.stores = make(map[string]storage.Constraints)
		}
		stores = *f.stores
	}
	stores[storageName] = cons
	return nil
}

// String implements gnuflag.Value.String.
func (f storageFlag) String() string {
	strs := make([]string, 0, len(*f.stores))
	for store, cons := range *f.stores {
		strs = append(strs, fmt.Sprintf("%s=%v", store, cons))
	}
	if f.bundleStores != nil {
		for application, stores := range *f.bundleStores {
			for store, cons := range stores {
				strs = append(strs, fmt.Sprintf("%s:%s=%v", application, store, cons))
			}
		}
	}
	return strings.Join(strs, " ")
}

type devicesFlag struct {
	devices       *map[string]devices.Constraints
	bundleDevices *map[string]map[string]devices.Constraints
}

// Set implements gnuflag.Value.Set.
func (f devicesFlag) Set(s string) error {
	fields := strings.SplitN(s, "=", 2)
	if len(fields) < 2 {
		if f.bundleDevices != nil {
			return errors.New("expected [<application>:]<device>=<constraints>")
		}
		return errors.New("expected <device>=<constraints>")
	}
	var applicationName, deviceName string
	if colon := strings.IndexRune(fields[0], ':'); colon >= 0 {
		if f.bundleDevices == nil {
			return errors.New("expected <device>=<constraints>")
		}
		applicationName = fields[0][:colon]
		deviceName = fields[0][colon+1:]
	} else {
		deviceName = fields[0]
	}
	cons, err := devices.ParseConstraints(fields[1])
	if err != nil {
		return errors.Annotate(err, "cannot parse device constraints")
	}
	var devs map[string]devices.Constraints
	if applicationName != "" {
		if *f.bundleDevices == nil {
			*f.bundleDevices = make(map[string]map[string]devices.Constraints)
		}
		devs = (*f.bundleDevices)[applicationName]
		if devs == nil {
			devs = make(map[string]devices.Constraints)
			(*f.bundleDevices)[applicationName] = devs
		}
	} else {
		if *f.devices == nil {
			*f.devices = make(map[string]devices.Constraints)
		}
		devs = *f.devices
	}
	devs[deviceName] = cons
	return nil
}

// String implements gnuflag.Value.String.
func (f devicesFlag) String() string {
	strs := make([]string, 0, len(*f.devices))
	for device, cons := range *f.devices {
		strs = append(strs, fmt.Sprintf("%s=%v", device, cons))
	}
	if f.bundleDevices != nil {
		for application, devices := range *f.bundleDevices {
			for device, cons := range devices {
				strs = append(strs, fmt.Sprintf("%s:%s=%v", application, device, cons))
			}
		}
	}
	return strings.Join(strs, " ")
}

type attachStorageFlag struct {
	storageIDs *[]string
}

// Set implements gnuflag.Value.Set.
func (f attachStorageFlag) Set(s string) error {
	if s == "" {
		return nil
	}
	for _, id := range strings.Split(s, ",") {
		if !names.IsValidStorage(id) {
			return errors.NotValidf("storage ID %q", id)
		}
		*f.storageIDs = append(*f.storageIDs, id)
	}
	return nil
}

// String implements gnuflag.Value.String.
func (f attachStorageFlag) String() string {
	return strings.Join(*f.storageIDs, ",")
}

// stringMap is a type that deserializes a CLI string using gnuflag's Value
// semantics.  It expects a name=value pair, and supports multiple copies of the
// flag adding more pairs, though the names must be unique.
type stringMap struct {
	mapping *map[string]string
}

// Set implements gnuflag.Value's Set method.
func (m stringMap) Set(s string) error {
	if *m.mapping == nil {
		*m.mapping = map[string]string{}
	}
	// make a copy so the following code is less ugly with dereferencing.
	mapping := *m.mapping

	vals := strings.SplitN(s, "=", 2)
	if len(vals) != 2 {
		return errors.NewNotValid(nil, "badly formatted name value pair: "+s)
	}
	name, value := vals[0], vals[1]
	if _, ok := mapping[name]; ok {
		return errors.Errorf("duplicate name specified: %q", name)
	}
	mapping[name] = value
	return nil
}

// String implements gnuflag.Value's String method
func (m stringMap) String() string {
	pairs := make([]string, 0, len(*m.mapping))
	for name, value := range *m.mapping {
		pairs = append(pairs, name+"="+value)
	}
	return strings.Join(pairs, ";")
}
