// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"

	jujubase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
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
	// Fingerprint is the fingerprint of the image.
	// This is the original fingerprint of the image, this is used to indicate
	// that an auto-update is available for the image.
	Fingerprint string
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
	ctx context.Context,
	base jujubase.Base,
	arch string,
	virtType instance.VirtType,
	sources []ServerSpec,
	copyLocal bool,
	callback environs.StatusCallbackFunc,
) (SourcedImage, error) {
	if callback != nil {
		_ = callback(ctx, status.Provisioning, "acquiring LXD image", nil)
	}

	// First we check if we have the image locally.
	localAlias := baseLocalAlias(base.DisplayString(), arch, virtType)
	entry, _, err := s.GetImageAlias(localAlias)
	if err != nil && !IsLXDNotFound(err) {
		return SourcedImage{}, errors.Trace(err)
	}

	var target string
	if entry != nil {
		// We already have an image with the given alias, so just use that.
		target = entry.Target
		image, _, err := s.GetImage(target)
		if err != nil {
			logger.Warningf(ctx, "failed to get local image %s: %s", target, err)
		}

		if err == nil && isCompatibleVirtType(virtType, image.Type) {
			logger.Debugf(ctx, "found image locally - %q %q", image.Filename, target)
			return SourcedImage{
				Image:     image,
				LXDServer: s.InstanceServer,
			}, nil
		}
	}

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of the aliases
	// that might identify the image we want.
	alias, err := constructBaseRemoteAlias(base, arch)
	if err != nil {
		return SourcedImage{}, errors.Trace(err)
	}

	var sourced SourcedImage
	lastErr := fmt.Errorf("no matching image found")

	for _, remote := range sources {
		source, err := ConnectImageRemote(ctx, remote)
		if err != nil {
			logger.Infof(ctx, "failed to connect to %q: %s", remote.Host, err)
			lastErr = errors.Trace(err)
			continue
		}

		// Locate the image using the aliases. This will return the first
		// image found that matches the alias.
		res, _, err := source.GetImageAliasType(string(virtType), alias)
		if err != nil {
			logger.Debugf(ctx, "failed to get alias %q from %q: %s", alias, remote.Name, err)
		} else if res != nil && res.Target != "" {
			// If the image is found by an alias prefer that over the one
			// from the local alias.
			target = res.Target
		}

		// If the target is empty, this is because the local target was empty
		// and the alias was not found.
		if target == "" {
			continue
		}

		// If we've found a target for the image we want, get the image.
		// NOTE: If the get image fails for any reason, we'll never retry the
		// same source again.
		image, _, err := source.GetImage(target)
		if err != nil {
			lastErr = errors.Trace(err)
			continue
		}

		logger.Debugf(ctx, "found image remotely - %q %q %q", remote.Name, image.Filename, target)

		// In order to support auto-update, we need to set the
		// fingerprint of the image to the alias that was used to
		// find it.
		// There is no LXD API to do this natively, and this is a copy
		// direct from the LXD source that enables the same feature.

		// Copy the image to ensure we don't modify the original. This
		// can cause issues if the image is used in multiple places.
		imageRef := *image
		imageRef.AutoUpdate = true

		// If dealing with an alias, set the img fingerprint to match
		// the provided targetAlias (needed for auto-update)
		if imageRef.Public && !strings.HasPrefix(imageRef.Fingerprint, alias) {
			imageRef.Fingerprint = alias
		}

		// Set the image copy reference to the original image.
		logger.Debugf(ctx, "found image remotely - %q %q %q %q", remote.Name, image.Filename, imageRef.Fingerprint, target)
		sourced.Image = &imageRef
		sourced.LXDServer = source
		sourced.Fingerprint = image.Fingerprint
		break
	}

	// We use the absence of a sourced image to indicate that we didn't find
	// the image we were looking for and return the last error we encountered.
	if sourced.Image == nil {
		return SourcedImage{}, lastErr
	}

	// If requested, copy the image to the local cache, adding the local alias.
	if copyLocal {
		if err := s.CopyRemoteImage(ctx, sourced, []string{alias, localAlias}, callback); err != nil {
			return sourced, errors.Trace(err)
		}

		// Now that we have the image cached locally, we indicate in the return
		// that the source is local instead of the remote where we found it.
		sourced.LXDServer = s.InstanceServer
	}

	// If we had to override Fingerprint to track an upstream alias, restore
	// the Fingerprint attribute to ensure the alias is correctly recorded.
	sourced.Image.Fingerprint = sourced.Fingerprint

	// Delete the LXD lookup alias, as we have now copied the image locally.
	// This ensures that if you mix both containers and virtual machines, they
	// can both exist.
	if err := s.InstanceServer.DeleteImageAlias(alias); err != nil && !IsLXDNotFound(err) {
		return sourced, errors.Trace(err)
	}

	return sourced, nil
}

// CopyRemoteImage accepts an image sourced from a remote server and copies it
// to the local cache
func (s *Server) CopyRemoteImage(
	ctx context.Context, sourced SourcedImage, aliases []string,
	callback environs.StatusCallbackFunc,
) error {
	logger.Debugf(ctx, "Copying image from remote server")

	newAliases := make([]api.ImageAlias, len(aliases))
	for i, a := range aliases {
		newAliases[i] = api.ImageAlias{Name: a}
	}
	req := &lxd.ImageCopyArgs{
		AutoUpdate: true,
		Type:       sourced.Image.Type,
		Aliases:    newAliases,
	}
	progress := func(op api.Operation) {
		if op.Metadata == nil {
			return
		}
		for _, key := range []string{"fs_progress", "download_progress"} {
			if value, ok := op.Metadata[key]; ok {
				_ = callback(ctx, status.Provisioning, fmt.Sprintf("Retrieving image: %s", value.(string)), nil)
				return
			}
		}
	}

	var op lxd.RemoteOperation
	attemptDownload := func() error {
		var err error
		op, err = s.CopyImage(sourced.LXDServer, *sourced.Image, req)
		if err != nil {
			return err
		}
		// Report progress via callback if supplied.
		if callback != nil {
			_, err = op.AddHandler(progress)
			if err != nil {
				return err
			}
		}

		// Prevent the operation from blocking indefinitely.
		done := make(chan error)
		go func() {
			done <- op.Wait()
		}()
		select {
		case err := <-done:
			return errors.Trace(err)
		case <-ctx.Done():
			return op.CancelTarget()
		}
	}
	// NOTE(jack-w-shaw) We wish to retry downloading images because we have been seeing
	// some flakey performance from the ubuntu cloud-images archive. This has lead to rare
	// but disruptive failures to bootstrap due to these transient failures.
	// Ideally this should be handled at lxd's end. However, image download is handled by
	// the lxd server/agent, this needs to be handled by lxd.
	// TODO(jack-s-shaw) Remove retries here once it's been implemented in lxd. See this bug:
	// https://github.com/canonical/lxd/issues/12672
	err := retry.Call(retry.CallArgs{
		Clock:       s.clock,
		Attempts:    3,
		Delay:       15 * time.Second,
		BackoffFunc: retry.DoubleDelay,
		Stop:        ctx.Done(),
		Func:        attemptDownload,
		IsFatalError: func(err error) bool {
			// unfortunately the LXD client currently does not
			// provide a way to differentiate between errors
			return !strings.HasPrefix(err.Error(), "Failed remote image download")
		},
		NotifyFunc: func(_ error, attempt int) {
			if callback != nil {
				_ = callback(ctx, status.Provisioning, fmt.Sprintf("Failed remote LXD image download. Retrying. Attempt number %d", attempt+1), nil)
			}
		},
	})
	if err != nil {
		return errors.Trace(err)
	}
	opInfo, err := op.GetTarget()
	if err != nil {
		return errors.Trace(err)
	}
	if opInfo.StatusCode != api.Success {
		return fmt.Errorf("image copy failed: %s", opInfo.Err)
	}
	if err := ensureImageAliases(s.InstanceServer, newAliases, sourced.Fingerprint); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Create the specified image aliases, updating those that already exist.
func ensureImageAliases(client lxd.InstanceServer, aliases []api.ImageAlias, fingerprint string) error {
	if len(aliases) == 0 {
		return nil
	}

	names := set.NewStrings()
	for _, alias := range aliases {
		names.Add(alias.Name)
	}

	resp, err := client.GetImageAliases()
	if err != nil {
		return err
	}

	// Delete existing aliases that match provided ones
	for _, alias := range getExistingAliases(names, resp) {
		err := client.DeleteImageAlias(alias.Name)
		if err != nil {
			return fmt.Errorf("failed to remove alias %s: %w", alias.Name, err)
		}
	}

	// Create new aliases.
	for _, alias := range aliases {
		var aliasPost api.ImageAliasesPost
		aliasPost.Name = alias.Name
		aliasPost.Target = fingerprint
		err := client.CreateImageAlias(aliasPost)
		if err != nil {
			return fmt.Errorf("failed to create alias %s: %w", alias.Name, err)
		}
	}

	return nil
}

// getExistingAliases returns the intersection between a list of aliases and
// all the existing ones.
func getExistingAliases(aliases set.Strings, allAliases []api.ImageAliasesEntry) []api.ImageAliasesEntry {
	existing := []api.ImageAliasesEntry{}
	for _, alias := range allAliases {
		if aliases.Contains(alias.Name) {
			existing = append(existing, alias)
		}
	}
	return existing
}

// baseLocalAlias returns the alias to assign to images for the
// specified series. The alias is juju-specific, to support the
// user supplying a customised image (e.g. CentOS with cloud-init).
func baseLocalAlias(base, arch string, virtType instance.VirtType) string {
	// We use a different alias for VMs, so that we can distinguish between
	// a VM image and a container image. We don't add anything to the alias
	// for containers to keep backwards compatibility with older versions
	// of the image aliases.
	switch virtType {
	case instance.InstanceTypeVM:
		return fmt.Sprintf("juju/%s/%s/vm", base, arch)
	default:
		return fmt.Sprintf("juju/%s/%s", base, arch)
	}
}

func isCompatibleVirtType(virtType instance.VirtType, instanceType string) bool {
	if instanceType == "" && (virtType.IsAny() || virtType == instance.InstanceTypeContainer) {
		return true
	}
	return string(virtType) == instanceType
}

func constructBaseRemoteAlias(base jujubase.Base, arch string) (string, error) {
	if ostype.OSTypeForName(base.OS) != ostype.Ubuntu {
		return "", errors.NotSupportedf("base %q", base.DisplayString())
	}
	return path.Join(base.Channel.Track, arch), nil
}
