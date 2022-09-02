// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import "github.com/juju/juju/environs/imagemetadata"

type containerFactory struct {
	fetcher imagemetadata.SimplestreamsFetcher
}

var _ ContainerFactory = (*containerFactory)(nil)

func (factory *containerFactory) New(name string) Container {
	return factory.new(name, nil)
}

func (factory *containerFactory) List() (result []Container, err error) {
	machines, err := ListMachines(run)
	if err != nil {
		return nil, err
	}
	for hostname, status := range machines {
		result = append(result, factory.new(hostname, isRunning(status)))
	}
	return result, nil
}

func (factory *containerFactory) new(name string, started *bool) *kvmContainer {
	return &kvmContainer{
		fetcher: factory.fetcher,
		factory: factory,
		name:    name,
		started: started,
	}
}

func isRunning(value string) *bool {
	result := value == "running"
	return &result
}
