// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/errors"

	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
)

func (env *environ) resolveImageIDMetadata(ctx context.Context, args environs.StartInstanceParams) ([]*imagemetadata.ImageMetadata, error) {
	// Should not happen - caller checks image id value.
	if args.Constraints.ImageID == nil {
		return nil, errors.Errorf("image ID is required")
	}
	imageID := *args.Constraints.ImageID
	project, imageName, err := env.parseImageIDReference(args.InstanceConfig.Base.OS, imageID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	image, err := env.gce.ImageByProject(ctx, project, imageName)
	if err != nil {
		return nil, errors.Annotatef(err, "getting image %q", imageID)
	}

	imageArch, err := convertImageArch(image.GetArchitecture())
	if err != nil {
		return nil, errors.Annotatef(err, "getting image %q", imageID)
	}
	requiredArch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if imageArch != requiredArch {
		return nil, errors.Errorf("image %q has architecture %q but agent requires %q", imageID, imageArch, requiredArch)
	}

	return []*imagemetadata.ImageMetadata{{
		Id:       imageRef(project, imageName),
		Arch:     imageArch,
		VirtType: virtType,
	}}, nil
}

func (env *environ) parseImageIDReference(os string, imageID string) (string, string, error) {
	if project, imageName, ok := splitProjectImagePath(imageID); ok && imageName != "" {
		return project, imageName, nil
	}

	imageURLBase, err := env.imageURLBase(ostype.OSTypeForName(os))
	if err != nil {
		return "", "", errors.Trace(err)
	}
	project, _, ok := splitProjectImagePath(imageURLBase)
	if !ok || project == "" {
		return "", "", errors.Errorf("invalid image base path %q", imageURLBase)
	}
	return project, imageID, nil
}

func splitProjectImagePath(imagePath string) (string, string, bool) {
	normalizedPath := strings.TrimPrefix(imagePath, "/")
	if parsedURL, err := url.Parse(imagePath); err == nil && parsedURL.Scheme != "" {
		normalizedPath = strings.TrimPrefix(parsedURL.Path, "/")
	}

	parts := strings.Split(normalizedPath, "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "projects" || parts[i+2] != "global" || parts[i+3] != "images" {
			continue
		}
		project := parts[i+1]
		imageName := ""
		if i+4 < len(parts) {
			imageName = parts[i+4]
		}
		return project, imageName, project != ""
	}
	return "", "", false
}

func imageRef(project, imageName string) string {
	return fmt.Sprintf("projects/%s/global/images/%s", project, imageName)
}

func convertImageArch(gceArch string) (string, error) {
	switch strings.ToUpper(gceArch) {
	case "ARM64":
		return corearch.ARM64, nil
	case "X86_64":
		return corearch.AMD64, nil
	default:
		return "", errors.Errorf("unsupported image architecture %q", gceArch)
	}
}

func (env *environ) imageURL(os ostype.OSType, imageID string) (string, error) {
	if strings.HasPrefix(imageID, "projects/") ||
		strings.HasPrefix(imageID, "https://") {
		return imageID, nil
	}

	imageURLBase, err := env.imageURLBase(os)
	if err != nil {
		return "", errors.Trace(err)
	}
	return imageURLBase + imageID, nil
}
