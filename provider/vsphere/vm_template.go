// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
	"github.com/vmware/govmomi/object"
)

// vmTemplateManager implements a template registry that
// can return a proper VMware template given a series and
// image metadata.
type vmTemplateManager struct {
	imageMetadata    []*imagemetadata.ImageMetadata
	env              *sessionEnviron
	az               *vmwareAvailZone
	datastore        *object.Datastore
	controllerUUID   string
	statusUpdateArgs vsphereclient.StatusUpdateParams
}

// EnsureTemplate will return a virtual machine template, given the template
// constraints. If the template does not exist, this function will attempt
// to create a new template with the characteristics described by the givem
// constraints.
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

	logger.Debugf("Looking for already imported templates for arches %q", arches)
	// Attempt to find a previously imported instance template
	importedTemplate, arch, err := v.getImportedTemplate(ctx, series, arches)
	if err == nil {
		logger.Debugf("Found already imported template for series %s", series)
		return importedTemplate, arch, nil
	}

	// Ignore not found errors. It means we have not imported a template yet.
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
		return nil, "", errors.NotFoundf("no image metadata was supplied")
	}

	for _, img := range v.imageMetadata {
		vms, err := v.env.client.ListVMTemplates(ctx, img.Id)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		switch len(vms) {
		case 1:
			// trust that due diligence was made when generating simplestreams
			// and the img.Arch, reflects the architecture of the OS running insude
			// the VM generated from the found template.
			return vms[0], img.Arch, nil
		default:
			continue
		}
	}
	return nil, "", errors.NotFoundf("could not find a suitable template")
}

func (v *vmTemplateManager) seriesTemplateFolder(series string) string {
	templatesPath := v.env.controllerTemplatesFolder(v.controllerUUID)
	return path.Join(templatesPath, series)
}

func (v *vmTemplateManager) getVMArch(ctx context.Context, vmObj *object.VirtualMachine) (string, error) {
	vmMo, err := v.env.client.VirtualMachineObjectToManagedObject(ctx, vmObj)
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
	return "", errors.NotFoundf("arch tag not found for VM")
}

func (v *vmTemplateManager) isValidArch(arch string, desiredArches []string) bool {
	for _, item := range desiredArches {
		if item == arch {
			return true
		}
	}
	return false
}

func (v *vmTemplateManager) getImportedTemplate(ctx context.Context, series string, arches []string) (*object.VirtualMachine, string, error) {
	seriesTemplatesFolder := v.seriesTemplateFolder(series)
	seriesTemplates, err := v.env.client.ListVMTemplates(ctx, path.Join(seriesTemplatesFolder, "*"))
	if err != nil {
		logger.Errorf("failed to fetch templates: %v", err)
		return nil, "", errors.Trace(err)
	}
	logger.Debugf("Series templates: %v", seriesTemplates)
	if len(seriesTemplates) == 0 {
		return nil, "", errors.NotFoundf("no valid templates found")
	}
	var vmTpl *object.VirtualMachine
	if len(arches) > 0 {
		for _, item := range seriesTemplates {
			arch, err := v.getVMArch(ctx, item)
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				}
			}
			if !v.isValidArch(arch, arches) {
				continue
			}

			vmTpl = item
			break
		}
		if vmTpl == nil {
			return nil, "", errors.NotFoundf("no valid templates found")
		}
	} else {
		vmTpl = seriesTemplates[0]
	}

	return vmTpl, "", nil
}

func (v *vmTemplateManager) downloadAndImportTemplate(
	ctx context.Context,
	series string, arches []string,
) (*object.VirtualMachine, string, error) {

	baseFolder := v.env.getVMFolder()
	seriesTemplateFolder := v.seriesTemplateFolder(series)
	relativePath := seriesTemplateFolder
	if len(baseFolder) > 0 && strings.HasPrefix(relativePath, baseFolder) {
		relativePath = relativePath[len(baseFolder)+1:]
	}

	vmFolder, err := v.env.client.EnsureVMFolder(
		ctx, baseFolder, relativePath)
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
		ResourcePool:       v.az.pool.Reference(),
		DestinationFolder:  vmFolder,
		StatusUpdateParams: v.statusUpdateArgs,
		Datastore:          v.datastore,
		Arch:               img.Arch,
		Series:             series,
	}
	vmTpl, err := v.env.client.CreateTemplateVM(ctx, ovaArgs)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return vmTpl, img.Arch, nil
}
