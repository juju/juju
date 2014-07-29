package environmentserver

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

var errStateServerNotAllowed = fmt.Errorf("state server jobs specified without calling EnsureAvailability")

type Deployer interface {
	state.EnvironmentProvider
	state.EnvironmentDistributor
	state.EnvironmentValidator
	state.EnvironmentDeployment

	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)

	PrecheckInstance(series string, cons constraints.Value, placement string) error
	InstanceDistributor(*config.Config) (InstanceDistributor, error)
}

type deployer struct {
	state *state.State
}

var _ Deployer = (*deployer)(nil)

func NewDeployer(state *state.State) Deployer {
	return &deployer{state: state}
}

// AddMachineInsideNewMachine creates a new machine within a container
// of the given type inside another new machine. The two given templates
// specify the form of the child and parent respectively.
func (d *deployer) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error) {
	if template.InstanceId != "" || parentTemplate.InstanceId != "" {
		return nil, fmt.Errorf("cannot specify instance id for a new container")
	}
	parentTemplate, err := d.effectiveMachineTemplate(parentTemplate, false)
	if err != nil {
		return nil, err
	}

	template, err = d.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, err
	}

	if containerType == "" {
		return nil, fmt.Errorf("no container type specified")
	}
	if parentTemplate.InstanceId == "" {
		// Adding a machine within a machine implies add-machine or placement.
		if err := d.SupportsUnitPlacement(); err != nil {
			return nil, err
		}
		if err := d.PrecheckInstance(parentTemplate.Series, parentTemplate.Constraints, parentTemplate.Placement); err != nil {
			return nil, err
		}
	}

	return d.AddMachineInsideNewMachine(template, parentTemplate, containerType)
}

// AddMachineInsideMachine adds a machine inside a container of the
// given type on the existing machine with id=parentId.
func (d *deployer) AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error) {
	if template.InstanceId != "" {
		return nil, fmt.Errorf("cannot specify instance id for a new container")
	}

	if containerType == "" {
		return nil, fmt.Errorf("no container type specified")
	}

	if err := d.verifyTemplates([]state.MachineTemplate{template}); err != nil {
		return nil, err
	}

	template, err := d.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, err
	}

	return d.state.AddMachineInsideMachine(template, parentId, containerType)
}

// AddMachine adds a machine with the given series and jobs.
// It is deprecated and around for testing purposes only.
func (d *deployer) AddMachine(series string, jobs ...state.MachineJob) (*state.Machine, error) {
	ms, err := d.AddMachines(state.MachineTemplate{
		Series: series,
		Jobs:   jobs,
	})
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddOneMachine machine adds a new machine configured according to the
// given template.
func (d *deployer) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	ms, err := d.AddMachines(template)
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddMachines adds new machines configured according to the
// given templates.
func (d *deployer) AddMachines(templates ...state.MachineTemplate) (_ []*state.Machine, err error) {
	if err := d.verifyTemplates(templates); err != nil {
		return nil, err
	}

	effectiveTemplates, err := d.makeEffectiveTemplates(templates)
	if err != nil {
		return nil, err
	}

	return d.state.AddMachines(effectiveTemplates...)
}

func (d *deployer) InstanceDistributor(cfg *config.Config) (InstanceDistributor, error) {
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	if p, ok := env.(InstanceDistributor); ok {
		return p, nil
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}

// makeEffectiveTemplates
func (d *deployer) makeEffectiveTemplates(templates []state.MachineTemplate) ([]state.MachineTemplate, error) {
	var effectiveTemplates []state.MachineTemplate
	for _, template := range templates {
		effectiveTemplate, err := d.effectiveMachineTemplate(template, false)
		if err != nil {
			return nil, err
		}
		effectiveTemplates = append(effectiveTemplates, effectiveTemplate)
	}
	return effectiveTemplates, nil
}

// verifyTemplates
func (d *deployer) verifyTemplates(templates []state.MachineTemplate) error {
	var err error

	if err = d.verifySupportsUnitPlacementIfNeeded(templates); err != nil {
		return err
	}

	if err = d.precheckInstanceIfNeeded(templates); err != nil {
		return err
	}
	return err
}

// precheckInstanceIfNeeded
func (d *deployer) precheckInstanceIfNeeded(templates []state.MachineTemplate) error {
	for _, template := range templates {
		if template.InstanceId == "" {
			if err := d.PrecheckInstance(template.Series, template.Constraints, template.Placement); err != nil {
				return err
			}
		}
	}
	return nil
}

// verifySupportsUnitPlacementIfNeeded
func (d *deployer) verifySupportsUnitPlacementIfNeeded(templates []state.MachineTemplate) error {
	for _, template := range templates {
		principals := template.Principals()
		if len(principals) == 0 && template.InstanceId == "" {
			if err := d.SupportsUnitPlacement(); err != nil {
				return err
			}
		}
	}
	return nil
}

// effectiveMachineTemplate verifies that the given template is
// valid and combines it with values from the state
// to produce a resulting template that more accurately
// represents the data that will be inserted into the state.
func (d *deployer) effectiveMachineTemplate(p state.MachineTemplate, allowStateServer bool) (tmpl state.MachineTemplate, err error) {
	// First check for obvious errors.
	if p.Series == "" {
		return tmpl, fmt.Errorf("no series specified")
	}
	if p.InstanceId != "" {
		if p.Nonce == "" {
			return tmpl, fmt.Errorf("cannot add a machine with an instance id and no nonce")
		}
	} else if p.Nonce != "" {
		return tmpl, fmt.Errorf("cannot specify a nonce without an instance id")
	}

	p.Constraints, err = d.ResolveConstraints(p.Constraints)
	if err != nil {
		return tmpl, err
	}
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same
	// constraints.Value struct so here we clear the container
	// value. Provisioning ignores the container value but clearing
	// it avoids potential confusion.
	p.Constraints.Container = nil

	if len(p.Jobs) == 0 {
		return tmpl, fmt.Errorf("no jobs specified")
	}
	jset := make(map[state.MachineJob]bool)
	for _, j := range p.Jobs {
		if jset[j] {
			return state.MachineTemplate{}, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	if jset[state.JobManageEnviron] {
		if !allowStateServer {
			return tmpl, errStateServerNotAllowed
		}
	}
	return p, nil
}
