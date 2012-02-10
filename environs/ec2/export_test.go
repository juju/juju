package ec2

import "launchpad.net/juju/go/environs"

type BootstrapState struct {
	ZookeeperInstances []string
}

func LoadState(e environs.Environ) (*BootstrapState, error) {
	s, err := e.(*environ).loadState()
	if err != nil {
		return nil, err
	}
	return &BootstrapState{s.ZookeeperInstances}, nil
}

func AuthorizedKeys(keys, path string) (string, error) {
	return authorizedKeys(keys, path)
}

var ZkPortSuffix = zkPortSuffix
