package peergrouper

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	return nil
}
