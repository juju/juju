// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/state"
)

var (
	modelTypeFlag      = flag.String("model-type", "iaas", "model type to use (iaas|caas)")
	transitiveDepsFlag = flag.Bool("transitive-deps", false, "include transitive dependencies, not just direct ones")
)

func main() {
	flag.Parse()

	var manifolds dependency.Manifolds
	switch *modelTypeFlag {
	case "iaas":

		manifolds = machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		})
	case "caas":
		manifolds = machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		})
	default:
		panic("unknown model type")
	}

	root := NewDAG()

	includeTransitive := *transitiveDepsFlag
	for name, manifold := range manifolds {
		node := root.AddVertex(name)
		dependencies := manifoldDependencies(manifolds, manifold, includeTransitive)
		for _, dep := range dependencies.Values() {
			node.AddEdge(dep)
		}
	}

	fmt.Println(root.Render())
}

func manifoldDependencies(all dependency.Manifolds, manifold dependency.Manifold, includeTransitive bool) set.Strings {
	result := set.NewStrings()
	for _, input := range manifold.Inputs {
		result.Add(input)
		if !includeTransitive {
			continue
		}

		result = result.Union(manifoldDependencies(all, all[input], true))
	}
	return result
}

type Dag struct {
	nodes map[string]*DagNode
}

func NewDAG() *Dag {
	root := new(Dag)
	root.nodes = make(map[string]*DagNode)
	return root
}

func (d *Dag) AddVertex(name string) *DagNode {
	node := new(DagNode)
	node.name = name
	d.nodes[name] = node
	return node
}

func (d *Dag) Render() string {
	template := `
digraph depgraph {
`
	nodes := make([]string, len(d.nodes))
	for _, node := range d.nodes {
		b := new(bytes.Buffer)
		node.Render(b)
		nodes = append(nodes, b.String())
	}
	return fmt.Sprintf("%s\n%s}", template, strings.Join(nodes, ""))
}

type Writer interface {
	WriteString(string) (int, error)
}

type DagNode struct {
	name     string
	children []string
}

func (n *DagNode) AddEdge(to string) {
	n.children = append(n.children, to)
}

func (n *DagNode) Render(b Writer) {
	if len(n.children) == 0 {
		b.WriteString(fmt.Sprintf("\t\"%s\"\n", n.name))
		return
	}

	for _, v := range n.children {
		b.WriteString(fmt.Sprintf("\t\"%s\" -> \"%s\"\n", n.name, v))
	}
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag      names.Tag
	ssiSet   bool
	ssi      controller.StateServingInfo
	dataPath string
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}

func (mc *mockConfig) DataDir() string {
	if mc.dataPath != "" {
		return mc.dataPath
	}
	return "data-dir"
}

func preUpgradeSteps(state.ModelType) upgrades.PreUpgradeStepsFunc { return nil }
