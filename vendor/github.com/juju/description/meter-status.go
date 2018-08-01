// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

// MeterStatus represents the meter status of the model.
type MeterStatus interface {
	// Code returns the traffic light colour code of meter status.
	Code() string
	// Info returns extra information corresponding to the traffic light colour.
	Info() string
}

type meterStatus struct {
	Code_ string `yaml:"code"`
	Info_ string `yaml:"info"`
}

// Code returns the traffic light colour code of meter status.
func (m meterStatus) Code() string {
	return m.Code_
}

// Info returns extra information corresponding to the traffic light colour.
func (m meterStatus) Info() string {
	return m.Info_
}
