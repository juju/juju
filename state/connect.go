package state
import (
	"fmt"
)

// Info encapsulates information about a juju state server
// or servers and can be used to make a connection to
// that state.
type Info struct {
	Addrs []string
}

// Connect connects to the server at the given
// info and returns a new State representing
// the environment connected to.
func (s *Info) Connect() (*State, error) {
	return nil, fmt.Errorf("TODO")
//	addrs := strings.Join(s.Addrs, ",")
}
