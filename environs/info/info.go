// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package info

// TODO add json tags
type EnvironInfo struct {
	User         string
	Password     string
	StateServers []string               `yaml:"state-servers"`
	CACert       string                 `yaml:"ca-cert"`
	Config       map[string]interface{} `yaml:"bootstrap-config,omitempty"`
}
