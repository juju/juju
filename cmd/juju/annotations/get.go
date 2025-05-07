// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/annotations"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/rpc/params"
)

const (
	getAnnotationsDoc = `
Get annotations for an entity.
`
	getAnnotationsExamples = `
	juju get-annotations model-<modelUUID1> model-<modelUUID2>
	juju get-annotations applicationoffer-<offerUUID>
`
)

// SetAnnotationsAPI is the annotations client API.
type GetAnnotationsAPI interface {
	Get(tags []string) ([]params.AnnotationsGetResult, error)
	Close() error
}

type annotation struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"vaule"`
}

type resourceAnnotations struct {
	Resource    string       `json:"resource" yaml:"resource"`
	Annotations []annotation `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Error       string       `json:"error,omitempty" yaml:"error,omitempty"`
}

type getAnnotationsCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	resourceTags       []string
	annotationsAPIFunc func() (GetAnnotationsAPI, error)
}

// NewGetAnnotationsCommand returns a command to get annotations for juju resources.
func NewGetAnnotationsCommand() cmd.Command {
	c := &getAnnotationsCommand{}
	c.annotationsAPIFunc = c.annotationsAPI

	return modelcmd.Wrap(c)
}

func (c *getAnnotationsCommand) annotationsAPI() (GetAnnotationsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return annotations.NewClient(root), nil
}

// Info implements cmd.Command.
func (c *getAnnotationsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "get-annotations",
		Args:     "<resource tag> [<resource tag 2>...]",
		Purpose:  "Get annotations.",
		Doc:      getAnnotationsDoc,
		Examples: getAnnotationsExamples,
	})
}

// SetFlags implements cmd.SetFlags.
func (c *getAnnotationsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		"tabular": func(writer io.Writer, value interface{}) error {
			return formatAnnotationsTabular(writer, value)
		},
	})
}

// Init implements cmd.Command.
func (c *getAnnotationsCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("at least one resource tag needs to be supplied")
	}

	for _, arg := range args {
		_, err := names.ParseTag(arg)
		if err != nil {
			return err
		}
		c.resourceTags = append(c.resourceTags, arg)
	}

	return nil
}

// Run implements cmd.Command.
func (c *getAnnotationsCommand) Run(ctx *cmd.Context) error {
	api, err := c.annotationsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	results, err := api.Get(c.resourceTags)
	if err != nil {
		return err
	}

	var ras []resourceAnnotations
	for _, result := range results {
		annotations := []annotation{}
		for k, v := range result.Annotations {
			annotations = append(annotations, annotation{
				Key:   k,
				Value: v,
			})
		}
		ra := resourceAnnotations{
			Resource:    result.EntityTag,
			Annotations: annotations,
		}
		if result.Error.Error != nil {
			ra.Error = result.Error.Error.Message
		}
		ras = append(ras, ra)
	}
	return c.out.Write(ctx, ras)
}

// formatAnnotationsTabular writes a tabular summary of secret information.
func formatAnnotationsTabular(writer io.Writer, value interface{}) error {
	results, ok := value.([]resourceAnnotations)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", results, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{
		TabWriter: tw,
	}
	w.SetColumnAlignRight(3)

	w.Println("Resource", "Annotations", "Error")
	sort.Slice(results, func(i, j int) bool {
		return results[i].Resource < results[j].Resource
	})
	for _, r := range results {
		annotations := r.Annotations
		sort.Slice(annotations, func(i, j int) bool {
			return annotations[i].Key < annotations[j].Key
		})
		if len(annotations) == 0 {
			w.Print(r.Resource, "", r.Error)
		} else {
			var a annotation
			a, annotations = annotations[0], annotations[1:]
			w.Print(r.Resource, fmt.Sprintf("%s=%s", a.Key, a.Value), r.Error)
		}
		w.Println()
		for _, a := range annotations {
			w.Print("", fmt.Sprintf("%s=%s", a.Key, a.Value), "")
			w.Println()
		}
	}
	return tw.Flush()
}
