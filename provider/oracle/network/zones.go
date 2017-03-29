package network

type AvailabilityZone struct {
	name string
}

func NewAvailabilityZone(name string) AvailabilityZone {
	return AvailabilityZone{
		name: name,
	}
}

func (a AvailabilityZone) Name() string {
	return a.name
}

func (a AvailabilityZone) Available() bool {
	// we don't really have availability zones in oracle cloud. We only
	// have regions
	return true
}
