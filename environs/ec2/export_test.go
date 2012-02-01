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

func MakeIdentity(name, password string) string {
	return makeIdentity(name, password)
}

func AuthorizedKeys(path string) (string, error) {
	return authorizedKeys(path)
}
