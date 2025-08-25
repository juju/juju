// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/naturalsort"
)

const showUnitDoc = `
The command takes deployed unit names as an argument.

Optionally, relation data for only a specified endpoint
or related unit may be shown, or just the application data.
`

const showUnitExamples = `
To show information about a unit:

    juju show-unit mysql/0

To show information about multiple units:

    juju show-unit mysql/0 wordpress/1

To show only the application relation data for a unit:

    juju show-unit mysql/0 --app

To show only the relation data for a specific endpoint:

    juju show-unit mysql/0 --endpoint db

To show only the relation data for a specific related unit:

    juju show-unit mysql/0 --related-unit wordpress/2
`

// NewShowUnitCommand returns a command that displays unit info.
func NewShowUnitCommand() cmd.Command {
	s := &showUnitCommand{}
	s.newAPIFunc = func() (UnitsInfoAPI, error) {
		return s.newUnitAPI()
	}
	return modelcmd.Wrap(s)
}

type showUnitCommand struct {
	modelcmd.ModelCommandBase

	out         cmd.Output
	units       []string
	endpoint    string
	relatedUnit string
	appOnly     bool

	newAPIFunc func() (UnitsInfoAPI, error)
}

// Info implements Command.Info.
func (c *showUnitCommand) Info() *cmd.Info {
	showCmd := &cmd.Info{
		Name:     "show-unit",
		Args:     "<unit name>",
		Purpose:  "Displays information about a unit.",
		Doc:      showUnitDoc,
		Examples: showUnitExamples,
		SeeAlso: []string{
			"add-unit",
			"remove-unit",
		},
	}
	return jujucmd.Info(showCmd)
}

// Init implements Command.Init.
func (c *showUnitCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("an unit name must be supplied")
	}
	c.units = args
	if c.relatedUnit != "" && !names.IsValidUnit(c.relatedUnit) {
		return errors.NotValidf("related unit name %v", c.relatedUnit)
	}
	var invalid []string
	for _, one := range c.units {
		if !names.IsValidUnit(one) {
			invalid = append(invalid, one)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	plural := "s"
	if len(invalid) == 1 {
		plural = ""
	}
	return errors.NotValidf(`unit name%v %v`, plural, strings.Join(invalid, `, `))
}

// SetFlags implements Command.SetFlags.
func (c *showUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters.Formatters())
	f.StringVar(&c.endpoint, "endpoint", "", "Only show relation data for the specified endpoint")
	f.StringVar(&c.relatedUnit, "related-unit", "", "Only show relation data for the specified unit")
	f.BoolVar(&c.appOnly, "app", false, "Only show application relation data")
}

// UnitsInfoAPI defines the API methods that show-unit command uses.
type UnitsInfoAPI interface {
	Close() error
	UnitsInfo([]names.UnitTag) ([]application.UnitInfo, error)
}

func (c *showUnitCommand) newUnitAPI() (UnitsInfoAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Info implements Command.Run.
func (c *showUnitCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	tags, err := c.getUnitTags()
	if err != nil {
		return err
	}

	results, err := client.UnitsInfo(tags)
	if err != nil {
		return errors.Trace(err)
	}

	var errs []error
	var valid []application.UnitInfo
	for _, result := range results {
		if result.Error != nil {
			errs = append(errs, result.Error)
			continue
		}
		valid = append(valid, result)
	}
	if len(errs) > 0 {
		var errorStrings []string
		for _, r := range errs {
			errorStrings = append(errorStrings, r.Error())
		}
		return errors.New(strings.Join(errorStrings, "\n"))
	}

	output, err := c.formatUnitInfos(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

func (c *showUnitCommand) getUnitTags() ([]names.UnitTag, error) {
	tags := make([]names.UnitTag, len(c.units))
	for i, one := range c.units {
		if !names.IsValidUnit(one) {
			return nil, errors.Errorf("invalid unit name %v", one)
		}
		tags[i] = names.NewUnitTag(one)
	}
	return tags, nil
}

func (c *showUnitCommand) formatUnitInfos(all []application.UnitInfo) (map[string]UnitInfo, error) {
	if len(all) == 0 {
		return nil, nil
	}
	output := make(map[string]UnitInfo)
	for _, one := range all {
		tag, info, err := c.createUnitInfo(one)
		if err != nil {
			return nil, errors.Trace(err)
		}
		output[tag.Id()] = info
	}
	return output, nil
}

type UnitRelationData struct {
	InScope  bool                   `yaml:"in-scope" json:"in-scope"`
	UnitData map[string]interface{} `yaml:"data" json:"data"`
}

type RelationData struct {
	RelationId              int                         `yaml:"relation-id" json:"relation-id"`
	Endpoint                string                      `yaml:"endpoint" json:"endpoint"`
	CrossModel              bool                        `yaml:"cross-model,omitempty" json:"cross-model,omitempty"`
	RelatedEndpoint         string                      `yaml:"related-endpoint" json:"related-endpoint"`
	ApplicationRelationData map[string]interface{}      `yaml:"application-data" json:"application-data"`
	MyData                  UnitRelationData            `yaml:"local-unit,omitempty" json:"local-unit,omitempty"`
	Data                    map[string]UnitRelationData `yaml:"related-units,omitempty" json:"related-units,omitempty"`
}

// UnitInfo defines the serialization behaviour of the unit information.
type UnitInfo struct {
	WorkloadVersion string         `yaml:"workload-version,omitempty" json:"workload-version,omitempty"`
	Machine         string         `yaml:"machine,omitempty" json:"machine,omitempty"`
	OpenedPorts     []string       `yaml:"opened-ports" json:"opened-ports"`
	PublicAddress   string         `yaml:"public-address,omitempty" json:"public-address,omitempty"`
	Charm           string         `yaml:"charm" json:"charm"`
	Leader          bool           `yaml:"leader" json:"leader"`
	Life            string         `yaml:"life,omitempty" json:"life,omitempty"`
	RelationData    []RelationData `yaml:"relation-info,omitempty" json:"relation-info,omitempty"`

	// The following are for CAAS models.
	ProviderId string `yaml:"provider-id,omitempty" json:"provider-id,omitempty"`
	Address    string `yaml:"address,omitempty" json:"address,omitempty"`
}

func (c *showUnitCommand) createUnitInfo(details application.UnitInfo) (names.UnitTag, UnitInfo, error) {
	tag, err := names.ParseUnitTag(details.Tag)
	if err != nil {
		return names.UnitTag{}, UnitInfo{}, errors.Trace(err)
	}

	info := UnitInfo{
		WorkloadVersion: details.WorkloadVersion,
		Machine:         details.Machine,
		OpenedPorts:     details.OpenedPorts,
		PublicAddress:   details.PublicAddress,
		Charm:           details.Charm,
		Leader:          details.Leader,
		Life:            details.Life,
		ProviderId:      details.ProviderId,
		Address:         details.Address,
	}
	for _, rdparams := range details.RelationData {
		if c.endpoint != "" && rdparams.Endpoint != c.endpoint {
			continue
		}
		rd := RelationData{
			RelationId:              rdparams.RelationId,
			Endpoint:                rdparams.Endpoint,
			RelatedEndpoint:         rdparams.RelatedEndpoint,
			CrossModel:              rdparams.CrossModel,
			ApplicationRelationData: make(map[string]interface{}),
			Data:                    make(map[string]UnitRelationData),
		}
		for k, v := range rdparams.ApplicationData {
			rd.ApplicationRelationData[k] = v
		}
		if c.appOnly {
			info.RelationData = append(info.RelationData, rd)
			continue
		}
		var unitNames []string
		for remoteUnit := range rdparams.UnitRelationData {
			if c.relatedUnit != "" && remoteUnit != c.relatedUnit {
				continue
			}
			if remoteUnit == tag.Id() {
				data := rdparams.UnitRelationData[remoteUnit]
				urd := UnitRelationData{
					InScope:  data.InScope,
					UnitData: make(map[string]interface{}),
				}
				for k, v := range data.UnitData {
					urd.UnitData[k] = v
				}
				rd.MyData = urd
				continue
			}
			unitNames = append(unitNames, remoteUnit)
		}
		naturalsort.Sort(unitNames)
		for _, remoteUnit := range unitNames {
			data := rdparams.UnitRelationData[remoteUnit]
			urd := UnitRelationData{
				InScope:  data.InScope,
				UnitData: make(map[string]interface{}),
			}
			for k, v := range data.UnitData {
				urd.UnitData[k] = v
			}
			rd.Data[remoteUnit] = urd
		}
		if c.endpoint == rd.Endpoint || len(rd.ApplicationRelationData) > 0 ||
			len(rd.Data) > 0 || len(rd.MyData.UnitData) > 0 {
			info.RelationData = append(info.RelationData, rd)
		}
	}

	return tag, info, nil
}
