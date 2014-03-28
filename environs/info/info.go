// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package info

// TODO add json tags
type EnvironInfo struct {
	User         string
	Password     string
	StateServers []string               `json:"state-servers" yaml:"state-servers"`
	CACert       string                 `json:"ca-cert" yaml:"ca-cert"`
	Config       map[string]interface{} `json:"bootstrap-config,omitempty" yaml:"bootstrap-config,omitempty"`
}
