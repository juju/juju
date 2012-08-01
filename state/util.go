package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/version"
	pathpkg "path"
	"sort"
	"strings"
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
	zk    *zookeeper.Conn
	path  string
	agent string
}

func (av *agentVersion) agentVersion(attr string) (version.Number, error) {
	text := strings.Replace(attr, "-", " ", -1) // e.g. "proposed version"
	sv, err := getConfigString(av.zk, av.path, attr,
		"%s agent %s", av.agent, text)
	if err != nil {
		return version.Number{}, err
	}
	v, err := version.Parse(sv)
	if err != nil {
		return version.Number{}, fmt.Errorf("cannot parse %s agent %s: %v", av.agent, text, err)
	}
	return v, nil
}

func (av *agentVersion) setAgentVersion(attr string, v version.Number) error {
	return setConfigString(av.zk, av.path, attr, v.String(),
		"%s agent %s", av.agent, strings.Replace(attr, "-", " ", -1))
}

// AgentVersion returns the current version of the agent.
// It returns a *NotFoundError if the version has not been set.
func (av *agentVersion) AgentVersion() (version.Number, error) {
	return av.agentVersion("version")
}

// SetAgentVersion sets the currently running version of the agent.
func (av *agentVersion) SetAgentVersion(v version.Number) error {
	return av.setAgentVersion("version", v)
}

// ProposedAgent version returns the version of the agent that is
// proposed to be run.  It returns a *NotFoundError if the proposed
// version has not been set.
func (av *agentVersion) ProposedAgentVersion() (version.Number, error) {
	return av.agentVersion("proposed-version")
}

// ProposeAgentVersion sets the the version of the agent that
// is proposed to be run.
func (av *agentVersion) ProposeAgentVersion(v version.Number) error {
	return av.setAgentVersion("proposed-version", v)
}
