// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	environsmetadata "github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.imagemetadataworker")

// updatePublicImageMetadataPeriod is how frequently we check for
// public image metadata updates.
const updatePublicImageMetadataPeriod = time.Hour * 24

// ListPublishedMetadataFunc is the type of a function that is supplied to
// NewWorker for listing environment-specific published images metadata.
type ListPublishedMetadataFunc func(env environs.Environ) ([]*environsmetadata.ImageMetadata, error)

// DefaultListBlockDevices is the default function for listing block
// devices for the operating system of the local host.
var DefaultListPublishedMetadata ListPublishedMetadataFunc

func init() {
	DefaultListPublishedMetadata = list
}

// NewWorker returns a worker that lists published cloud
// images metadata, and records them in state.
func NewWorker(cl *imagemetadata.Client, l ListPublishedMetadataFunc, env environs.Environ) worker.Worker {
	// TODO (anastasiamac 2015-09-02) Bug#1491353 - don't ignore stop channel.
	f := func(stop <-chan struct{}) error {
		return doWork(cl, l, env)
	}
	return worker.NewPeriodicWorker(f, updatePublicImageMetadataPeriod, worker.NewTimer)
}

func doWork(cl *imagemetadata.Client, listf ListPublishedMetadataFunc, env environs.Environ) error {
	published, err := listf(env)
	if err != nil {
		return errors.Annotatef(err, "getting published images metadata")
	}
	err = save(cl, published)
	return errors.Annotatef(err, "saving published images metadata")
}

func list(env environs.Environ) ([]*environsmetadata.ImageMetadata, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}

	// We want all metadata, hence empty constraints.
	cons := environsmetadata.ImageConstraint{}
	metadata, _, err := environsmetadata.Fetch(sources, &cons, false)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func save(client *imagemetadata.Client, published []*environsmetadata.ImageMetadata) error {
	// Store converted metadata.Note that whether the metadata actually needs
	// to be stored will be determined within this call.
	errs, err := client.Save(convertToParams(published))
	if err != nil {
		return errors.Annotatef(err, "saving published images metadata")
	}
	return processErrors(errs)
}

// convertToParams converts environment-specific images metadata to structured metadata format.
var convertToParams = func(published []*environsmetadata.ImageMetadata) []params.CloudImageMetadata {
	metadata := make([]params.CloudImageMetadata, len(published))
	for i, p := range published {
		metadata[i] = params.CloudImageMetadata{
			Source:          "public",
			ImageId:         p.Id,
			Stream:          p.Stream,
			Region:          p.RegionName,
			Arch:            p.Arch,
			VirtualType:     p.VirtType,
			RootStorageType: p.Storage,
		}
		// Translate version (eg.14.04) to a series (eg. "trusty")
		metadata[i].Series = versionSeries(p.Version)
	}

	return metadata
}

var seriesVersion = version.SeriesVersion

func versionSeries(v string) string {
	if v == "" {
		return v
	}
	for _, s := range version.SupportedSeries() {
		sv, err := seriesVersion(s)
		if err != nil {
			logger.Errorf("cannot determine version for series %v: %v", s, err)
		}
		if v == sv {
			return s
		}
	}
	return v
}

func processErrors(errs []params.ErrorResult) error {
	msgs := []string{}
	for _, e := range errs {
		if e.Error != nil && e.Error.Message != "" {
			msgs = append(msgs, e.Error.Message)
		}
	}
	if len(msgs) != 0 {
		return errors.Errorf("saving some image metadata:\n%v", strings.Join(msgs, "\n"))
	}
	return nil
}
