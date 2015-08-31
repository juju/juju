// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	environsmetadata "github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.imagemetadataworker")

// interval sets how often the resuming is called.
var interval = 24 * time.Hour

var _ worker.Worker = (*MetadataUpdateWorker)(nil)

// MetadataUpdateWorker is responsible for a periodical retrieval of image metadata
// according to the established image search paths, and recording metadata for new images.
type MetadataUpdateWorker struct {
	environ environs.Environ
	client  *imagemetadata.Client
	tomb    tomb.Tomb
}

// NewMetadataUpdateWorker periodically updates image metadata based on images search path.
func NewMetadataUpdateWorker(env environs.Environ, cl *imagemetadata.Client) *MetadataUpdateWorker {
	muw := &MetadataUpdateWorker{client: cl}
	go func() {
		defer muw.tomb.Done()
		muw.tomb.Kill(muw.loop())
	}()
	return muw
}

func (muw *MetadataUpdateWorker) String() string {
	return fmt.Sprintf("image metadata update worker")
}

// Stop stops the worker.
func (muw *MetadataUpdateWorker) Stop() error {
	muw.tomb.Kill(nil)
	return muw.tomb.Wait()
}

// Kill is defined on the worker.Worker interface.
func (muw *MetadataUpdateWorker) Kill() {
	muw.tomb.Kill(nil)
}

// Wait is defined on the worker.Worker interface.
func (muw *MetadataUpdateWorker) Wait() error {
	return muw.tomb.Wait()
}

func (muw *MetadataUpdateWorker) loop() error {
	err := muw.updateMetadata()
	if err != nil {
		return err
	}
	for {
		select {
		case <-muw.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			err := muw.updateMetadata()
			if err != nil {
				return err
			}
		}
	}
}

func (muw *MetadataUpdateWorker) updateMetadata() error {
	return UpdateMetadata(muw)
}

var UpdateMetadata = func(muw *MetadataUpdateWorker) error {
	if err := muw.saveMetadata(); err != nil {
		logger.Errorf("cannot update image metadata: %v", err)
		return errors.Annotatef(err, "failed updating image metadata")
	}
	return nil
}

func (muw *MetadataUpdateWorker) getAllPublishedMetadata() ([]*environsmetadata.ImageMetadata, error) {
	sources, err := environs.ImageMetadataSources(muw.environ)
	if err != nil {
		return nil, err
	}
	// TODO(anastasiamac 2015-08-31) Once it is added to search path,
	// remove state ? (structured metadta) so that we do not repeat ourselves...

	// We want all metadata, hence empty contraints.
	cons := environsmetadata.ImageConstraint{}
	metadata, _, err := environsmetadata.Fetch(sources, &cons, false)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func (muw *MetadataUpdateWorker) saveMetadata() error {

	// 1. Get all current metadata from search path
	published, err := muw.getAllPublishedMetadata()
	if err != nil {
		return errors.Annotatef(err, "getting published images")
	}

	// 2. Convert to structured metadata format.
	metadata := make([]params.CloudImageMetadata, len(published))
	for i, p := range published {
		metadata[i] = params.CloudImageMetadata{
			Source:          "public", // what happens when this is a custom image that is returned from search path?
			ImageId:         p.Id,     // Is This image id?
			Stream:          p.Stream,
			Region:          p.RegionName,
			Series:          p.Version, // Is this series?
			Arch:            p.Arch,
			VirtualType:     p.VirtType,
			RootStorageType: p.Storage,
		}
	}

	// 3. Store converted metadata.Note that whether the metadata actually needs
	// to be stored will be determined within this call.
	errs, err := muw.client.Save(metadata)
	if err != nil {
		return errors.Annotatef(err, "saving published images")
	}
	return processErrorsFromSave(errs)
}

func processErrorsFromSave(errs []params.ErrorResult) error {
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
