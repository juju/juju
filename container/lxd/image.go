// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"path"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"

	jujuarch "github.com/juju/juju/core/arch"
	jujubase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
)

// SourcedImage is the result of a successful image acquisition.
// It includes the relevant data that located the image.
type SourcedImage struct {
	// Image is the actual image data that was located.
	Image *api.Image
	// LXDServer is the image server that supplied the image.
	LXDServer lxd.ImageServer
}

// FindImage searches the input sources in supplied order, looking for an OS
// image matching the supplied base and architecture.
// If found, the image and the server from which it was acquired are returned.
// If the server is remote the image will be cached by LXD when used to create
// a container.
// Supplying true for copyLocal will copy the image to the local cache.
// Copied images will have the juju/series/arch alias added to them.
// The callback argument is used to report copy progress.
func (s *Server) FindImage(
	base jujubase.Base,
	arch string,
	virtType instance.VirtType,
	sources []ServerSpec,
	copyLocal bool,
	callback environs.StatusCallbackFunc,
) (SourcedImage, error) {
	if callback != nil {
		_ = callback(status.Provisioning, "acquiring LXD image", nil)
	}

	// First we check if we have the image locally.
	localAlias := baseLocalAlias(base.DisplayString(), arch, virtType)
	var target string
	entry, _, err := s.GetImageAlias(localAlias)
	if err != nil && !IsLXDNotFound(err) {
		return SourcedImage{}, errors.Trace(err)
	}

	if entry != nil {
		// We already have an image with the given alias, so just use that.
		target = entry.Target
		image, _, err := s.GetImage(target)
		if isCompatibleVirtType(virtType, image.Type) && err == nil {
			logger.Debugf("Found image locally - %q %q", image.Filename, target)
			return SourcedImage{
				Image:     image,
				LXDServer: s.InstanceServer,
			}, nil
		}
	}

	sourced := SourcedImage{}
	lastErr := fmt.Errorf("no matching image found")

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of the aliases
	// that might identify the image we want.
	aliases, err := baseRemoteAliases(base, arch)
	if err != nil {
		return sourced, errors.Trace(err)
	}
	for _, remote := range sources {
		source, err := ConnectImageRemote(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = errors.Trace(err)
			continue
		}
		for _, alias := range aliases {
			if res, _, err := source.GetImageAliasType(string(virtType), alias); err == nil && res != nil && res.Target != "" {
				target = res.Target
				break
			}
		}
		if target != "" {
			image, _, err := source.GetImage(target)
			if err == nil {
				logger.Debugf("Found image remotely - %q %q %q", remote.Name, image.Filename, target)
				sourced.Image = image
				sourced.LXDServer = source
				break
			} else {
				lastErr = errors.Trace(err)
			}
		}
	}

	if sourced.Image == nil {
		return sourced, lastErr
	}

	// If requested, copy the image to the local cache, adding the local alias.
	if copyLocal {
		if err := s.CopyRemoteImage(sourced, []string{localAlias}, callback); err != nil {
			return sourced, errors.Trace(err)
		}

		// Now that we have the image cached locally, we indicate in the return
		// that the source is local instead of the remote where we found it.
		sourced.LXDServer = s.InstanceServer
	}

	return sourced, nil
}

// CopyRemoteImage accepts an image sourced from a remote server and copies it
// to the local cache
func (s *Server) CopyRemoteImage(
	sourced SourcedImage, aliases []string, callback environs.StatusCallbackFunc,
) error {
	logger.Debugf("Copying image from remote server")

	newAliases := make([]api.ImageAlias, len(aliases))
	for i, a := range aliases {
		newAliases[i] = api.ImageAlias{Name: a}
	}

	req := &lxd.ImageCopyArgs{Aliases: newAliases}
	op, err := s.CopyImage(sourced.LXDServer, *sourced.Image, req)
	if err != nil {
		return errors.Trace(err)
	}

	// Report progress via callback if supplied.
	if callback != nil {
		progress := func(op api.Operation) {
			if op.Metadata == nil {
				return
			}
			for _, key := range []string{"fs_progress", "download_progress"} {
				if value, ok := op.Metadata[key]; ok {
					_ = callback(status.Provisioning, fmt.Sprintf("Retrieving image: %s", value.(string)), nil)
					return
				}
			}
		}
		_, err = op.AddHandler(progress)
		if err != nil {
			return errors.Trace(err)
		}
	}

	if err := op.Wait(); err != nil {
		return errors.Trace(err)
	}
	opInfo, err := op.GetTarget()
	if err != nil {
		return errors.Trace(err)
	}
	if opInfo.StatusCode != api.Success {
		return fmt.Errorf("image copy failed: %s", opInfo.Err)
	}
	return nil
}

// baseLocalAlias returns the alias to assign to images for the
// specified corebase. The alias is juju-specific, to support the
// user supplying a customised image (e.g. CentOS with cloud-init).
func baseLocalAlias(base, arch string, virtType instance.VirtType) string {
	// We use a different alias for VMs, so that we can distinguish between
	// a VM image and a container image. We don't add anything to the alias
	// for containers to keep backwards compatibility with older versions
	// of the image aliases.
	switch virtType {
	case api.InstanceTypeVM:
		return fmt.Sprintf("juju/%s/%s/vm", base, arch)
	default:
		return fmt.Sprintf("juju/%s/%s", base, arch)
	}
}

// baseRemoteAliases returns the aliases to look for in remotes.
func baseRemoteAliases(base jujubase.Base, arch string) ([]string, error) {
	alias, err := constructBaseRemoteAlias(base, arch)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []string{
		alias,
	}, nil
}

func isCompatibleVirtType(virtType instance.VirtType, instanceType string) bool {
	if instanceType == "" && (virtType == api.InstanceTypeAny || virtType == api.InstanceTypeContainer) {
		return true
	}
	return string(virtType) == instanceType
}

func constructBaseRemoteAlias(base jujubase.Base, arch string) (string, error) {
	seriesOS := jujuos.OSTypeForName(base.OS)
	switch seriesOS {
	case jujuos.Ubuntu:
		return path.Join(base.Channel.Track, arch), nil
	case jujuos.CentOS:
		if arch == jujuarch.AMD64 {
			switch base.Channel.Track {
			case "7", "8":
				return fmt.Sprintf("centos/%s/cloud/amd64", base.Channel.Track), nil
			case "9":
				return "centos/9-Stream/cloud/amd64", nil
			}
		}
	case jujuos.OpenSUSE:
		if base.Channel.Track == "opensuse42" && arch == jujuarch.AMD64 {
			return "opensuse/42.2/amd64", nil
		}
	}
	return "", errors.NotSupportedf("base %q", base.DisplayString())
}
