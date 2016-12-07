// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

// DetailsTopic is the topic name for the published message when the details of
// the api servers change. This message is normally published by the peergrouper
// when the set of API servers changes.
const DetailsTopic = "apiserver.details"

// APIServer defines a single api server machine.
type APIServer struct {
	Id        string   `yaml:"id"`
	Addresses []string `yaml:"addresses"`
}

// Details defines the message structure for the apiserver.details topic.
type Details struct {
	Servers   []APIServer `yaml:"servers"`
	LocalOnly bool        `yaml:"local-only"`
}
