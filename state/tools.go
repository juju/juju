package state

import (
	"fmt"
	"launchpad.net/juju-core/version"
)

// Tools describes a particular set of juju tools and where to find them.
type Tools struct {
	version.Binary
	URL string
}

type agentTools struct {
	st    *State
	path  string
	agent string
}

func getAgentTools(cn *ConfigNode, prefix string) (tools *Tools, err error) {
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

func (at *agentTools) agentTools(prefix string) (tools *Tools, err error) {
	defer errorContextf(&err, "cannot get %s agent %s tools", at.agent, prefix)
	cn, err := readConfigNode(at.st.zk, at.path)
	if err != nil {
		return nil, err
	}
	return getAgentTools(cn, prefix)
}

func (at *agentTools) setAgentTools(prefix string, t *Tools) (err error) {
	defer errorContextf(&err, "cannot set %s tools for %s agent", prefix, at.agent)
	if t.Series == "" || t.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	config, err := readConfigNode(at.st.zk, at.path)
	if err != nil {
		return err
	}
	config.Set(prefix+"-agent-tools-version", t.Binary.String())
	config.Set(prefix+"-agent-tools-url", t.URL)
	_, err = config.Write()
	return err
}

// AgentVersion returns the tools that the agent is currently running.
func (at *agentTools) AgentTools() (*Tools, error) {
	return at.agentTools("current")
}

// SetAgentVersion sets the tools that the agent is currently running.
func (at *agentTools) SetAgentTools(t *Tools) error {
	return at.setAgentTools("current", t)
}

// WatchAgentVersion watches the set of tools that the agent is currently
// running.
func (at *agentTools) WatchAgentTools() *AgentToolsWatcher {
	return newAgentToolsWatcher(at.st, at.path, "current")
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

// WatchProposedAgentVersion watches the set of tools that are
// proposed for the agent to run.
func (at *agentTools) WatchProposedAgentTools() *AgentToolsWatcher {
	return newAgentToolsWatcher(at.st, at.path, "proposed")
}
