package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/version"
	pathpkg "path"
	"sort"
)

var (
	// stateChanged is a common error inside the state processing.
	stateChanged = errors.New("environment state has changed")
	// zkPermAll is a convenience variable for creating new nodes.
	zkPermAll = zookeeper.WorldACL(zookeeper.PERM_ALL)
)

// zkRemoveTree recursively removes a zookeeper node and all its
// children.  It does not delete "/zookeeper" or the root node itself
// and it does not consider deleting a nonexistent node to be an error.
func zkRemoveTree(zk *zookeeper.Conn, path string) (err error) {
	defer errorContextf(&err, "cannot clean up data")
	// If we try to delete the zookeeper node (for example when
	// calling ZkRemoveTree(zk, "/")) we silently ignore it.
	if path == "/zookeeper" {
		return
	}
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			return nil
		}
		return
	}
	for _, child := range children {
		if err = zkRemoveTree(zk, pathpkg.Join(path, child)); err != nil {
			return
		}
	}
	// Now delete the path itself unless it's the root node.
	if path == "/" {
		return nil
	}
	err = zk.Delete(path, -1)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		return err
	}
	return nil
}

// errorContextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}

// diff returns all the elements that exist in A but not B.
func diff(A, B []string) (missing []string) {
next:
	for _, a := range A {
		for _, b := range B {
			if a == b {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}

type portSlice []Port

func (p portSlice) Len() int      { return len(p) }
func (p portSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p portSlice) Less(i, j int) bool {
	p1 := p[i]
	p2 := p[j]
	if p1.Protocol != p2.Protocol {
		return p1.Protocol < p2.Protocol
	}
	return p1.Number < p2.Number
}

// SortPorts sorts the given ports, first by protocol,
// then by number.
func SortPorts(ports []Port) {
	sort.Sort(portSlice(ports))
}

type agentVersion struct {
	zk *zookeeper.Conn
	path string
	agent string
}

func (av *agentVersion) agentVersion(prefix string) (version.Version, error) {
	sv, err := getConfigString(av.zk, av.path, fmt.Sprintf("%s agent %sversion", av.agent), prefix+"version")
	if err != nil {
		return version.Version{}, err
	}
	v, err := version.Parse(sv)
	if err != nil {
		return version.Version{}, fmt.Errorf("cannot parse %s agent %sversion: %v", av.agent, prefix, err)
	}
	return v, nil
}

func (av *agentVersion) setAgentVersion(prefix string, v version.Version) error {
	return setConfigString(av.zk, av.path, fmt.Sprintf("%s agent %sversion", av.agent, prefix), prefix+"version", v.String())
}

func (av *agentVersion) AgentVersion() (version.Version, error) {
	return av.agentVersion("")
}

func (av *agentVersion) SetAgentVersion(v version.Version) error {
	return av.setAgentVersion("", v)
}
	
func (av *agentVersion) ProposedAgentVersion() (version.Version, error) {
	return av.agentVersion("proposed-")
}

func (av *agentVersion) ProposeAgentVersion(v version.Version) error {
	return av.setAgentVersion("proposed-", v)
}
