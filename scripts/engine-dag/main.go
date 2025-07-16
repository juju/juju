// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	"github.com/juju/juju/controller"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/state"
)

func main() {
	var (
		flags = flag.NewFlagSet("dag", flag.ExitOnError)

		modelTypeFlag      = flags.String("model-type", "iaas", "model type to use (iaas|caas)")
		useModelFlag       = flags.Bool("model", false, "use model manifolds")
		transitiveDepsFlag = flags.Int("dependency-depth", 0, "include transitive dependencies and how many levels to include")
		listManifoldsFlag  = flags.Bool("list-manifolds", false, "list all manifolds")

		manifoldsFlag = stringslice{}
	)

	flags.Var(&manifoldsFlag, "manifold", "manifold to select (empty indicates all)")
	flags.Usage = usageFor(flags, "dag")

	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}

	root := NewDAG()

	selectedManifolds := set.NewStrings(manifoldsFlag.Slice()...)

	manifolds := getManifolds(*useModelFlag, *modelTypeFlag)

	if *listManifoldsFlag {
		if *transitiveDepsFlag > 0 {
			fmt.Fprintf(os.Stderr, "cannot include transitive dependencies with listing manifolds\n")
			os.Exit(1)
		}
		if !selectedManifolds.IsEmpty() {
			fmt.Fprintf(os.Stderr, "cannot select a manifold whilst listing all manifolds\n")
			os.Exit(1)
		}

		for name := range manifolds {
			root.AddVertex(name)
		}
		fmt.Fprintln(os.Stdout, root.Render())
		os.Exit(0)
	}

	if selectedManifolds.IsEmpty() {
		// Spit out all the manifolds, with or without transitive dependencies,
		// depending on the flag.
		for name, manifold := range manifolds {
			node := root.AddVertex(name)
			dependencies := manifoldDependencies(manifolds, manifold, *transitiveDepsFlag)
			for _, dep := range dependencies.Values() {
				node.AddEdge(dep)
			}
		}

		// Render the graph and exit.
		fmt.Fprintln(os.Stdout, root.Render())
		os.Exit(0)
	}

	// Transitive dependencies are not supported with manifold selection.
	if *transitiveDepsFlag > 0 {
		fmt.Fprintf(os.Stderr, "cannot include transitive dependencies with manifold selection\n")
		os.Exit(1)
	}

	selection := make(map[string]dependency.Manifold)
	for name, manifold := range manifolds {
		if !selectedManifolds.Contains(name) {
			continue
		}
		selection[name] = manifold
	}

	if num := len(selection); num == 0 {
		fmt.Fprintf(os.Stderr, "manifold(s) not found: %q\n", selectedManifolds.SortedValues())
		os.Exit(1)
	} else if num < selectedManifolds.Size() {
		a := set.NewStrings()
		for name := range selection {
			a.Add(name)
		}
		fmt.Fprintf(os.Stderr, "not all manifolds found: %q\n", selectedManifolds.Difference(a).SortedValues())
	}

	for name, manifold := range selection {
		node := root.AddVertex(name)
		dependencies := manifoldDependencies(manifolds, manifold, 0)
		for _, dep := range dependencies.Values() {
			node.AddEdge(dep)
		}
	}

	fmt.Fprintln(os.Stdout, root.Render())
	os.Exit(0)
}

func manifoldDependencies(all dependency.Manifolds, manifold dependency.Manifold, transativeDepth int) set.Strings {
	result := set.NewStrings()
	for _, input := range manifold.Inputs {
		result.Add(input)
		if transativeDepth <= 0 {
			continue
		}

		result = result.Union(manifoldDependencies(all, all[input], transativeDepth-1))
	}
	return result
}

type Dag struct {
	nodes map[string]*DagNode
}

func NewDAG() *Dag {
	return &Dag{
		nodes: make(map[string]*DagNode),
	}
}

func (d *Dag) AddVertex(name string) *DagNode {
	if node, ok := d.nodes[name]; ok {
		return node
	}

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
		if _, err := b.WriteString(fmt.Sprintf("\t\"%s\"\n", n.name)); err != nil {
			panic(err)
		}
		return
	}

	for _, v := range n.children {
		if _, err := b.WriteString(fmt.Sprintf("\t\"%s\" -> \"%s\"\n", n.name, v)); err != nil {
			panic(err)
		}
	}
}

func getManifolds(useModel bool, modelType string) dependency.Manifolds {
	if useModel {
		switch modelType {
		case "iaas":
			return model.IAASManifolds(model.ManifoldsConfig{
				Agent:          &mockAgent{},
				LoggingContext: internallogger.DefaultContext(),
			})
		case "caas":
			return model.CAASManifolds(model.ManifoldsConfig{
				Agent:          &mockAgent{},
				LoggingContext: internallogger.DefaultContext(),
			})
		default:
			panic("unknown model type for model manifolds")
		}
	}

	switch modelType {
	case "iaas":
		return machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		})
	case "caas":
		return machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		})
	default:
		panic("unknown model type for machine manifolds")
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
	ssi      controller.ControllerAgentInfo
	dataPath string
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mock *mockConfig) Model() names.ModelTag {
	return names.NewModelTag("mock-model-uuid")
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.ControllerAgentInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.ControllerAgentInfo) {
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

func (mc *mockConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		Addrs:    []string{"here", "there"},
		CACert:   "trust-me",
		ModelTag: names.NewModelTag("mock-model-uuid"),
		Tag:      names.NewMachineTag("123"),
		Password: "12345",
		Nonce:    "11111",
	}, true
}

func (mc *mockConfig) OldPassword() string {
	return "do-not-use"
}

func preUpgradeSteps(state.ModelType) upgrades.PreUpgradeStepsFunc { return nil }

type stringslice []string

func (ss *stringslice) Set(s string) error {
	(*ss) = append(*ss, s)
	return nil
}

func (ss *stringslice) Slice() []string {
	return []string(*ss)
}

func (ss *stringslice) String() string {
	if len(*ss) <= 0 {
		return "..."
	}
	return strings.Join(*ss, ", ")
}

func usageFor(fs *flag.FlagSet, name string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", name)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")

		writer := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(writer, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		writer.Flush()

		fmt.Fprintf(os.Stderr, "\n")
	}
}
