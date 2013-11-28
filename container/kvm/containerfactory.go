// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

type containerFactory struct {
}

var _ ContainerFactory = (*containerFactory)(nil)

func (factory *containerFactory) New(name string) Container {
	return &kvmContainer{
		factory: factory,
		name:    name,
	}
}

func isRunning(value string) *bool {
	var result *bool = new(bool)
	if value == "running" {
		*result = true
	}
	return result
}

func (factory *containerFactory) List() (result []Container, err error) {
	machines, err := ListMachines()
	if err != nil {
		return nil, err
	}
	for hostname, status := range machines {
		result = append(result, &kvmContainer{
			factory: factory,
			name:    hostname,
			started: isRunning(status),
		})

	}
	return result, nil
}
