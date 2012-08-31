package state

import (
	"fmt"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/trivial"
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

func (at *agentTools) SetAgentTools(t *Tools) (err error) {
	defer trivial.ErrorContextf(&err, "cannot set tools for %s agent", at.agent)
	if t.Series == "" || t.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	config, err := readConfigNode(at.st.zk, at.path)
	if err != nil {
		return err
	}
	config.Set("tools-version", t.Binary.String())
	config.Set("tools-url", t.URL)
	log.Printf("writing agent tools %v; url %q\n", t.Binary, t.URL)
	_, err = config.Write()
	return err
}

// AgentTools returns the tools that the agent is currently running.
func (at *agentTools) AgentTools() (t *Tools, err error) {
	defer trivial.ErrorContextf(&err, "cannot get %s agent tools", at.agent)
	cn, err := readConfigNode(at.st.zk, at.path)
	if err != nil {
		return nil, err
	}
	vi, ok0 := cn.Get("tools-version")
	ui, ok1 := cn.Get("tools-url")
	// Initial state is the zero Tools.
	t = &Tools{}
	if !ok0 || !ok1 {
		return t, nil
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
	return t, nil
}
