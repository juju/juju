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

// Tools describes a particular set of juju tools and where to find them.
type Tools struct {
	version.Binary
	URL string
}

type agentTools struct {
	zk    *zookeeper.Conn
	path  string
	agent string
}

func (at *agentTools) agentTools(prefix string) (tools *Tools, err error) {
	defer errorContextf(&err, "cannot get %s agent %s tools", at.agent, prefix)
	cn, err := readConfigNode(at.zk, at.path)
	if err != nil {
		return nil, err
	}
	var t Tools
	vi, ok0 := cn.Get(prefix + "-agent-tools-version")
	ui, ok1 := cn.Get(prefix + "-agent-tools-url")
	// Initial state is the zero Tools.
	if !ok0 || !ok1 {
		return &t, nil
	}
	vs, ok := vi.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type for value %#v of version: %T", vi, vi)
	}
	t.Binary, err = version.ParseBinary(vs)
	if err != nil {
		return nil, err
	}
	t.URL, ok = ui.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type for value %#v of URL: %T", ui, ui)
	}
	return &t, nil
}

func (at *agentTools) setAgentTools(prefix string, t *Tools) (err error) {
	defer errorContextf(&err, "cannot set %s agent %s tools", at.agent, prefix)
	if t.Series == "" || t.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	config, err := readConfigNode(at.zk, at.path)
	if err != nil {
		return err
	}
	config.Set(prefix+"-agent-tools-version", t.Binary.String())
	config.Set(prefix+"-agent-tools-url", t.URL)
	_, err = config.Write()
	return err
}

// AgentVersion returns the tools that the agent is current running.
func (at *agentTools) AgentTools() (*Tools, error) {
	return at.agentTools("current")
}

// SetAgentVersion sets the tools that the agent is currently running.
func (at *agentTools) SetAgentTools(t *Tools) error {
	return at.setAgentTools("current", t)
}

// ProposedAgent version returns the tools that are proposed for
// the agent to run.
func (at *agentTools) ProposedAgentTools() (*Tools, error) {
	return at.agentTools("proposed")
}

// ProposeAgentVersion proposes some tools for the agent to run.
func (at *agentTools) ProposeAgentTools(t *Tools) error {
	return at.setAgentTools("proposed", t)
}
