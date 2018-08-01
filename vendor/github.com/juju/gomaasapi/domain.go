// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type domain struct {
	authoritative       bool
	resourceRecordCount int
	ttl                 *int
	resourceURI         string
	id                  int
	name                string
}

// Name implements Domain interface
func (domain *domain) Name() string {
	return domain.name
}

func readDomains(controllerVersion version.Number, source interface{}) ([]*domain, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "domain base schema check failed")
	}
	valid := coerced.([]interface{})
	return readDomainList(valid)
}

func domain_(source map[string]interface{}) (*domain, error) {
	fields := schema.Fields{
		"authoritative":         schema.Bool(),
		"resource_record_count": schema.ForceInt(),
		"ttl":          schema.OneOf(schema.Nil("null"), schema.ForceInt()),
		"resource_uri": schema.String(),
		"id":           schema.ForceInt(),
		"name":         schema.String(),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "domain schema check failed")
	}
	valid := coerced.(map[string]interface{})

	var ttl *int = nil
	if valid["ttl"] != nil {
		i := valid["ttl"].(int)
		ttl = &i
	}

	result := &domain{
		authoritative:       valid["authoritative"].(bool),
		id:                  valid["id"].(int),
		name:                valid["name"].(string),
		resourceRecordCount: valid["resource_record_count"].(int),
		resourceURI:         valid["resource_uri"].(string),
		ttl:                 ttl,
	}

	return result, nil
}

// readDomainList expects the values of the sourceList to be string maps.
func readDomainList(sourceList []interface{}) ([]*domain, error) {
	result := make([]*domain, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for domain %d, %T", i, value)
		}
		domain, err := domain_(source)
		if err != nil {
			return nil, errors.Annotatef(err, "domain %d", i)
		}
		result = append(result, domain)
	}
	return result, nil
}
