package deployer

type Addresser interface {
	Addrs() []string
}
