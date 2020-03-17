// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/os/series"
)

// DistroSource is the source of the underlying distro source for supported
// series.
type DistroSource interface {
	// Refresh will attempt to update the information it has about each distro
	// and if the distro is supported or not.
	Refresh() error

	// SeriesInfo returns the DistroInfoSerie for the series name.
	SeriesInfo(seriesName string) (series.DistroInfoSerie, bool)
}

// SupportedInfo represents all the supported info available.
type SupportedInfo struct {
	mutex sync.RWMutex

	source DistroSource
	values map[SeriesName]SeriesVersion
}

// NewSupportedInfo creates a supported info type for knowning if a series is
// supported or not.
func NewSupportedInfo(source DistroSource, preset map[SeriesName]SeriesVersion) *SupportedInfo {
	return &SupportedInfo{
		source: source,
		values: preset,
	}
}

// Compile compiles a list of supported info.
func (s *SupportedInfo) Compile(now time.Time) error {
	if err := s.source.Refresh(); err != nil {
		return errors.Trace(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// First thing here, is walk over the controller, workload maps to work out
	// if something was previously supported and is no longer or the reverse.
	for seriesName, version := range s.values {
		distroInfo, ok := s.source.SeriesInfo(seriesName.String())
		if !ok {
			// The series isn't found in the distro info, we should continue
			// onward as we don't know what to do here.
			continue
		}

		current := version.Supported
		supported := current

		// To prevent the distro info from overriding the supported flag and to
		// ensure that we keep the same Supported version as we have set as the
		// default (see below). Using the IgnoreDistroInfoUpdate flag states that
		// we want to keep the current value.
		// Example: adding a new LTS and setting it to be supported will become
		// false when reading in the distro information. Setting OverrideSupport
		// to true, will force it to be the same value as the default.
		if !version.IgnoreDistroInfoUpdate {
			supported = distroInfo.Supported(now)
		}

		s.values[seriesName] = SeriesVersion{
			WorkloadType:             version.WorkloadType,
			Version:                  version.Version,
			LTS:                      version.LTS,
			Supported:                supported,
			ESMSupported:             version.ESMSupported,
			IgnoreDistroInfoUpdate:   version.IgnoreDistroInfoUpdate,
			UpdatedByLocalDistroInfo: current != supported,
		}
	}

	return nil
}

// ControllerSeries returns a slice of series that are supported to run on a
// controller.
func (s *SupportedInfo) ControllerSeries() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var result []string
	for name, version := range s.values {
		if version.WorkloadType != ControllerWorkloadType {
			continue
		}

		if version.ESMSupported || version.Supported {
			result = append(result, name.String())
		}
	}
	return result
}

// WorkloadSeries returns a slice of series that are supported to run on a
// target workload (charm).
// Note: workload series will also include controller workload types, as they
// can also be used for workloads.
func (s *SupportedInfo) WorkloadSeries() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var result []string
	for name, version := range s.values {
		if version.ESMSupported || version.Supported {
			result = append(result, name.String())
		}
	}
	return result
}

// WorkloadType defines what type of workload the series is aimed at.
// Controllers only support Ubuntu systems.
type WorkloadType int

const (
	// ControllerWorkloadType defines a workload type that is for controllers
	// only.
	ControllerWorkloadType WorkloadType = iota

	// OtherWorkloadType workload type is for everything else.
	// In the future we might want to differentiate this.
	OtherWorkloadType
)

// SeriesVersion represents a ubuntu series that includes the version, if the
// series is an LTS and the supported defines if Juju supports the series
// version.
type SeriesVersion struct {
	// WorkloadType defines what type the series version is intended to work
	// against.
	WorkloadType WorkloadType

	// Version represents the version of the series.
	Version string

	// LTS provides a lookup for a LTS series.  Like seriesVersions,
	// the values here are current at the time of writing.
	LTS bool

	// Supported defines if Juju classifies the series as officially supported.
	Supported bool

	// Extended security maintenance for customers, extends the supported bool
	// for how Juju classifies the series.
	ESMSupported bool

	// IgnoreDistroInfoUpdate overrides the supported value to ensure that we
	// can force supported series, by ignoring the distro info update.
	IgnoreDistroInfoUpdate bool

	// UpdatedByLocalDistroInfo indicates that the series version was created
	// by the local distro-info information on the system.
	// This is useful to understand why a version appears yet is not supported.
	UpdatedByLocalDistroInfo bool
}

// DefaultSeries returns back all the series that Juju is aware of.
func DefaultSeries() map[SeriesName]SeriesVersion {
	all := make(map[SeriesName]SeriesVersion)
	for k, v := range ubuntuSeries {
		all[k] = v
	}
	for k, v := range nonUbuntuSeries {
		all[k] = v
	}
	return all
}

// SetSupported updates a series map based on the series name and sets it to
// be supported.
func SetSupported(series map[SeriesName]SeriesVersion, name string) bool {
	if version, ok := series[SeriesName(name)]; ok {
		version.Supported = true
		version.IgnoreDistroInfoUpdate = true
		series[SeriesName(name)] = version
		return true
	}
	return false
}

// SeriesName represents a series name for distros
type SeriesName string

func (s SeriesName) String() string {
	return string(s)
}

// TODO (stickupkid): We should get all of these from the os/series package.
const (
	Precise SeriesName = "precise"
	Quantal SeriesName = "quantal"
	Raring  SeriesName = "raring"
	Saucy   SeriesName = "saucy"
	Trusty  SeriesName = "trusty"
	Utopic  SeriesName = "utopic"
	Vivid   SeriesName = "vivid"
	Wily    SeriesName = "wily"
	Xenial  SeriesName = "xenial"
	Yakkety SeriesName = "yakkety"
	Zesty   SeriesName = "zesty"
	Artful  SeriesName = "artful"
	Bionic  SeriesName = "bionic"
	Cosmic  SeriesName = "cosmic"
	Disco   SeriesName = "disco"
	Eoan    SeriesName = "eoan"
	Focal   SeriesName = "focal"
)

var ubuntuSeries = map[SeriesName]SeriesVersion{
	Precise: {
		WorkloadType: ControllerWorkloadType,
		Version:      "12.04",
	},
	Quantal: {
		WorkloadType: ControllerWorkloadType,
		Version:      "12.10",
	},
	Raring: {
		WorkloadType: ControllerWorkloadType,
		Version:      "13.04",
	},
	Saucy: {
		WorkloadType: ControllerWorkloadType,
		Version:      "13.10",
	},
	Trusty: {
		WorkloadType: ControllerWorkloadType,
		Version:      "14.04",
		LTS:          true,
		ESMSupported: true,
	},
	Utopic: {
		WorkloadType: ControllerWorkloadType,
		Version:      "14.10",
	},
	Vivid: {
		WorkloadType: ControllerWorkloadType,
		Version:      "15.04",
	},
	Wily: {
		WorkloadType: ControllerWorkloadType,
		Version:      "15.10",
	},
	Xenial: {
		WorkloadType: ControllerWorkloadType,
		Version:      "16.04",
		LTS:          true,
		Supported:    true,
		ESMSupported: true,
	},
	Yakkety: {
		WorkloadType: ControllerWorkloadType,
		Version:      "16.10",
	},
	Zesty: {
		WorkloadType: ControllerWorkloadType,
		Version:      "17.04",
	},
	Artful: {
		WorkloadType: ControllerWorkloadType,
		Version:      "17.10",
	},
	Bionic: {
		WorkloadType: ControllerWorkloadType,
		Version:      "18.04",
		LTS:          true,
		Supported:    true,
		ESMSupported: true,
	},
	Cosmic: {
		WorkloadType: ControllerWorkloadType,
		Version:      "18.10",
	},
	Disco: {
		WorkloadType: ControllerWorkloadType,
		Version:      "19.04",
	},
	Eoan: {
		WorkloadType: ControllerWorkloadType,
		Version:      "19.10",
		Supported:    true,
	},
	Focal: {
		WorkloadType: ControllerWorkloadType,
		Version:      "20.04",
		LTS:          true,
		Supported:    false,
	},
}

// TODO (stickupkid): We should get all of these from the os/series package.
const (
	Win2008r2    SeriesName = "win2008r2"
	Win2012hvr2  SeriesName = "win2012hvr2"
	Win2012hv    SeriesName = "win2012hv"
	Win2012r2    SeriesName = "win2012r2"
	Win2012      SeriesName = "win2012"
	Win2016      SeriesName = "win2016"
	Win2016hv    SeriesName = "win2016hv"
	Win2016nano  SeriesName = "win2016nano"
	Win2019      SeriesName = "win2019"
	Win7         SeriesName = "win7"
	Win8         SeriesName = "win8"
	Win81        SeriesName = "win81"
	Win10        SeriesName = "win10"
	Centos7      SeriesName = "centos7"
	OpenSUSELeap SeriesName = "opensuseleap"
	GenericLinux SeriesName = "genericlinux"
	Kubernetes   SeriesName = "kubernetes"
)

var nonUbuntuSeries = map[SeriesName]SeriesVersion{
	Win2008r2: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2008r2",
		Supported:    true,
	},
	Win2012hvr2: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2012hvr2",
		Supported:    true,
	},
	Win2012hv: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2012hv",
		Supported:    true,
	},
	Win2012r2: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2012r2",
		Supported:    true,
	},
	Win2012: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2012",
		Supported:    true,
	},
	Win2016: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2016",
		Supported:    true,
	},
	Win2016hv: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2016hv",
		Supported:    true,
	},
	Win2016nano: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2016nano",
		Supported:    true,
	},
	Win2019: {
		WorkloadType: OtherWorkloadType,
		Version:      "win2019",
		Supported:    true,
	},
	Win7: {
		WorkloadType: OtherWorkloadType,
		Version:      "win7",
		Supported:    true,
	},
	Win8: {
		WorkloadType: OtherWorkloadType,
		Version:      "win8",
		Supported:    true,
	},
	Win81: {
		WorkloadType: OtherWorkloadType,
		Version:      "win81",
		Supported:    true,
	},
	Win10: {
		WorkloadType: OtherWorkloadType,
		Version:      "win10",
		Supported:    true,
	},
	Centos7: {
		WorkloadType: OtherWorkloadType,
		Version:      "centos7",
		Supported:    true,
	},
	OpenSUSELeap: {
		WorkloadType: OtherWorkloadType,
		Version:      "opensuse42",
		Supported:    true,
	},
	GenericLinux: {
		WorkloadType: OtherWorkloadType,
		Version:      "genericlinux",
		Supported:    true,
	},
	Kubernetes: {
		WorkloadType: OtherWorkloadType,
		Version:      "kubernetes",
		Supported:    true,
	},
}
