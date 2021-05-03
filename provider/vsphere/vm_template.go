// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

// vmTemplateManager implements a template registry that
// can return a proper VMware template given a series and
// image metadata.
type vmTemplateManager struct {
	imageMetadata    []*imagemetadata.ImageMetadata
	env              environs.Environ
	client           Client
	azPoolRef        types.ManagedObjectReference
	datastore        *object.Datastore
	statusUpdateArgs vsphereclient.StatusUpdateParams

	vmFolder       string
	controllerUUID string
}

// EnsureTemplate will return a virtual machine template for the requested series.
// If image metadata is supplied, this function will first look for "image-ids" entries
// describing a template already available in the vsphere deployment. If none is found
// or if no "image-ids" entries exist, it will then try to find a previously imported
// template via "image-download" simplestreams entries. As a last resort, it will try
// to import a new template from simplestreams.
func (v *vmTemplateManager) EnsureTemplate(ctx context.Context, series string, arches []string) (*object.VirtualMachine, string, error) {
	// Attempt to find image in image-metadata
	logger.Debugf("looking for local templates")
	tpl, arch, err := v.findTemplate(ctx)
	if err == nil {
		logger.Debugf("Found requested template for series %s", series)
		return tpl, arch, nil
	}
	if !errors.IsNotFound(err) {
		return nil, "", errors.Annotate(err, "searching for template")
	}

	logger.Debugf("Looking for already imported templates for series %q", series)
	// Attempt to find a previously imported instance template
	importedTemplate, arch, err := v.getImportedTemplate(ctx, series, arches)
	if err == nil {
		logger.Debugf("Using already imported template for series %s", series)
		return importedTemplate, arch, nil
	}
	logger.Debugf("could not find cached image: %s", err)
	// Exit here if we do not have a Not Found error. A Not Found error means we we have
	// not imported a template yet, keep going
	if !errors.IsNotFound(err) {
		return nil, "", errors.Trace(err)
	}
	logger.Debugf("Downloading and importing template from simplestreams")
	// Last resort, download and import a template.
	return v.downloadAndImportTemplate(ctx, series, arches)
}

// findTemplate uses the imageMetadata provided to find a local template.
// The imageMetadata parameter holds a list of already filtered templates,
// that should match the series that was requested.
func (v *vmTemplateManager) findTemplate(ctx context.Context) (*object.VirtualMachine, string, error) {
	if len(v.imageMetadata) == 0 {
		return nil, "", errors.NotFoundf("image metadata")
	}

	for _, img := range v.imageMetadata {
		vms, err := v.client.ListVMTemplates(ctx, img.Id)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		switch len(vms) {
		case 1:
			// Simplestreams image-id entries should point to only one template,
			// and not to a folder with multiple templates.
			//
			// Trust that due diligence was made when generating simplestreams
			// and the img.Arch, reflects the architecture of the OS running inside
			// the VM generated from the found template.
			return vms[0], img.Arch, nil
		default:
			continue
		}
	}
	return nil, "", errors.NotFoundf("suitable template")
}

func (v *vmTemplateManager) controllerTemplatesFolder() string {
	templateFolder := templateDirectoryName(controllerFolderName(v.controllerUUID))
	return path.Join(v.vmFolder, templateFolder)
}

func (v *vmTemplateManager) seriesTemplateFolder(series string) string {
	templatesPath := v.controllerTemplatesFolder()
	return path.Join(templatesPath, series)
}

func (v *vmTemplateManager) getVMArch(ctx context.Context, vmObj *object.VirtualMachine) (string, error) {
	vmMo, err := v.client.VirtualMachineObjectToManagedObject(ctx, vmObj)
	if err != nil {
		return "", errors.Trace(err)
	}
	for _, item := range vmMo.Config.ExtraConfig {
		value := item.GetOptionValue()
		if value.Key == vsphereclient.ArchTag {
			arch, ok := value.Value.(string)
			if !ok {
				return "", errors.Errorf("invalid arch tag for VM")
			}
			return arch, nil
		}
	}
	return "", errors.NotFoundf("arch tag")
}

func (v *vmTemplateManager) getImportedTemplate(ctx context.Context, series string, arches []string) (*object.VirtualMachine, string, error) {
	seriesTemplatesFolder := v.seriesTemplateFolder(series)
	seriesTemplates, err := v.client.ListVMTemplates(ctx, path.Join(seriesTemplatesFolder, "*"))
	if err != nil {
		logger.Tracef("failed to fetch templates: %v", err)
		return nil, "", errors.Trace(err)
	}
	logger.Tracef("Series templates: %v", seriesTemplates)
	if len(seriesTemplates) == 0 {
		return nil, "", errors.NotFoundf("valid template")
	}
	var vmTpl *object.VirtualMachine
	var arch string
	if len(arches) > 0 {
		for _, item := range seriesTemplates {
			arch, err = v.getVMArch(ctx, item)
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				}
			}
			if !set.NewStrings(arches...).Contains(arch) {
				continue
			}

			vmTpl = item
			break
		}
		if vmTpl == nil {
			return nil, "", errors.NotFoundf("valid template")
		}
	} else {
		vmTpl = seriesTemplates[0]
	}

	return vmTpl, arch, nil
}

func (v *vmTemplateManager) downloadAndImportTemplate(
	ctx context.Context,
	series string, arches []string,
) (*object.VirtualMachine, string, error) {

	seriesTemplateFolder := v.seriesTemplateFolder(series)
	if len(v.vmFolder) > 0 && strings.HasPrefix(seriesTemplateFolder, v.vmFolder) {
		seriesTemplateFolder = seriesTemplateFolder[len(v.vmFolder)+1:]
	}

	vmFolder, err := v.client.EnsureVMFolder(
		ctx, v.vmFolder, seriesTemplateFolder)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	img, err := findImageMetadata(v.env, arches, series)
	if err != nil {
		return nil, "", common.ZoneIndependentError(err)
	}

	readOVA := func() (string, io.ReadCloser, error) {
		resp, err := http.Get(img.URL)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		return img.URL, resp.Body, nil
	}

	ovaArgs := vsphereclient.ImportOVAParameters{
		ReadOVA:            readOVA,
		OVASHA256:          img.Sha256,
		TemplateName:       "juju-template-" + img.Sha256,
		ResourcePool:       v.azPoolRef,
		DestinationFolder:  vmFolder,
		StatusUpdateParams: v.statusUpdateArgs,
		Datastore:          v.datastore,
		Arch:               img.Arch,
		Series:             series,
	}
	vmTpl, err := v.client.CreateTemplateVM(ctx, ovaArgs)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return vmTpl, img.Arch, nil
}
