package oracle

import "github.com/juju/juju/constraints"

// Constraints
//TODO(Oracle cloud services) when powering up instances dosen't support special constraints values
type Constraints struct {
	//TODO(add attributes if needed)
}

func newConstraints() *Constraints {
	return &Constraints{}
}

func (c Constraints) RegisterConflicts(reds, blues []string) {
}

func (c Constraints) RegisterUnsupported(unsupported []string) {
}

func (c Constraints) RegisterVocabulary(attributeName string, allowedValues interface{}) {
}

func (c Constraints) Validate(cons constraints.Value) ([]string, error) {
	return nil, nil
}

func (c Constraints) Merge(consFallback, cons constraints.Value) (constraints.Value, error) {
	return constraints.Value{}, nil
}

func (c Constraints) UpdateVocabulary(attributeName string, newValues interface{}) {
}
