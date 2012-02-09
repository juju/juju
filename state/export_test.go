package state
import (
	"launchpad.net/gozk/zookeeper"
)

func (s *State) Zk() *zookeeper.Conn {
	return s.zk
}
