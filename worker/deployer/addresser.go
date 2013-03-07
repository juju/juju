package deployer

// Addresser implementations provide the capability to lookup a list of server addresses.
type Addresser interface {
	Addresses() []string
}
