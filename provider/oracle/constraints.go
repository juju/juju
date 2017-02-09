package oracle

import "github.com/juju/juju/constraints"

// cons provides a wrapper-adaptor arount the default constraints
// implementation inside the constrains pakage.
// this type implements the constrains.Validator interface
type cons struct {
	c constraints.Validator
}

func newConstraintsAdaptor(validator constraints.Validator) *cons {
	return &cons{c: validator}
}

func (c *cons) RegisterConflicts(reds, blues []string) {
	c.c.RegisterConflicts(reds, blues)
}

func (c *cons) RegisterUnsupported(unsupported []string) {
	c.c.RegisterUnsupported(unsupported)
}

func (c *cons) RegisterVocabulary(attributeName string, allowedValues interface{}) {
	c.c.RegisterVocabulary(attributeName, allowedValues)
}

func (c *cons) Validate(cons constraints.Value) ([]string, error) {
	return c.c.Validate(cons)
}

func (c *cons) Merge(consFallback, cons constraints.Value) (constraints.Value, error) {
	logger.Infof("Checking provided constrains before merging")

	if !consFallback.HasMem() {
		logger.Infof("No memory specified, using the default mem shape")
		if consFallback.Mem == nil {
			consFallback.Mem = new(uint64)
		}
		*consFallback.Mem = 1024
	}

	if !consFallback.HasCpuCores() {
		logger.Infof("No cpu cores specified, using the default cpu shape")
		if consFallback.CpuCores == nil {
			consFallback.CpuCores = new(uint64)
		}
		*consFallback.CpuCores = 1

	}

	if consFallback.Arch != nil {
		if *consFallback.Arch != "" && *consFallback.Arch != "amd64" {
			logger.Warningf("Oracle provider does not support Arch constraint other than amd64")
			*consFallback.Arch = "amd64"
		}
	}

	if consFallback.HasCpuPower() {
		logger.Warningf("Oracle provider does not support Cpu power constraint, skipping this constraint")
		consFallback.CpuPower = nil
	}

	if consFallback.HaveSpaces() {
		logger.Warningf("Oracle provider does not support Spaces constraints, skipping this constraint")
		consFallback.Spaces = nil
	}

	if consFallback.HasContainer() {
		logger.Warningf("Oracle provider does not support Contianer constraints, skipping this constraint")
		consFallback.Container = nil
	}

	if consFallback.HasVirtType() {
		logger.Warningf("Oracle provider does not support HasVirtType constraints, skipping this constrain")
		consFallback.VirtType = nil
	}

	if consFallback.HasInstanceType() {
		logger.Warningf("Oracle provider does not support HasInstanceType constraints, skipping this constraint")
		consFallback.InstanceType = nil
	}

	return c.c.Merge(consFallback, cons)
}

func (c *cons) UpdateVocabulary(attributeName string, newValues interface{}) {
	c.c.UpdateVocabulary(attributeName, newValues)
}
