// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/version"
)

const (
	msgUserRequestedBase = "with the user specified base %q"
	msgLatestLTSBase     = "with the latest LTS base %q"
)

// BaseSelector is a helper type that determines what base the charm should
// be deployed to.
type BaseSelector struct {
	requestedBase       corebase.Base
	defaultBase         corebase.Base
	explicitDefaultBase bool
	force               bool
	logger              SelectorLogger
	// supportedBases is the union of SupportedCharmBases and
	// SupportedJujuBases.
	supportedBases     []corebase.Base
	jujuSupportedBases set.Strings
	usingImageID       bool
}

type SelectorConfig struct {
	Config              SelectorModelConfig
	Force               bool
	Logger              SelectorLogger
	RequestedBase       corebase.Base
	SupportedCharmBases []corebase.Base
	WorkloadBases       []corebase.Base
	// usingImageID is true when the user is using the image-id constraint
	// when deploying the charm. This is needed to validate that in that
	// case the user is also explicitly providing a base.
	UsingImageID bool
}

type SelectorModelConfig interface {
	// DefaultBase returns the configured default base
	// for the environment, and whether the default base was
	// explicitly configured on the environment.
	DefaultBase() (string, bool)
}

// ConfigureBaseSelector returns a configured and validated BaseSelector
func ConfigureBaseSelector(cfg SelectorConfig) (BaseSelector, error) {
	// TODO (hml) 2023-05-16
	// Is there more we can do here and reduce the prep work
	// necessary for the callers?
	defaultBase, explicit := cfg.Config.DefaultBase()
	var (
		parsedDefaultBase corebase.Base
		err               error
	)
	if explicit {
		parsedDefaultBase, err = corebase.ParseBaseFromString(defaultBase)
		if err != nil {
			return BaseSelector{}, errors.Trace(err)
		}
	}
	bs := BaseSelector{
		requestedBase:       cfg.RequestedBase,
		defaultBase:         parsedDefaultBase,
		explicitDefaultBase: explicit,
		force:               cfg.Force,
		logger:              cfg.Logger,
		usingImageID:        cfg.UsingImageID,
		jujuSupportedBases:  set.NewStrings(),
	}
	bs.supportedBases, err = bs.validate(cfg.SupportedCharmBases, cfg.WorkloadBases)
	if err != nil {
		return BaseSelector{}, errors.Trace(err)
	}
	return bs, nil
}

// TODO(nvinuesa): The force flag is only valid if the requestedBase is specified
// or to force the deploy of a LXD profile that doesn't pass validation, this
// should be added to these validation checks.
func (s BaseSelector) validate(supportedCharmBases, supportedJujuBases []corebase.Base) ([]corebase.Base, error) {
	// If the image-id constraint is provided then base must be explicitly
	// provided either by flag either by model-config default base.
	if s.logger == nil {
		return nil, errors.NotValidf("empty Logger")
	}
	if s.usingImageID && s.requestedBase.Empty() && !s.explicitDefaultBase {
		return nil, errors.Forbiddenf("base must be explicitly provided when image-id constraint is used")
	}
	if len(supportedCharmBases) == 0 {
		return nil, errors.NotValidf("charm does not define any bases,")
	}
	if len(supportedJujuBases) == 0 {
		return nil, errors.NotValidf("no juju supported bases")
	}
	// Verify that the charm supported bases include at least one juju
	// supported base.
	var supportedBases []corebase.Base
	for _, charmBase := range supportedCharmBases {
		for _, jujuCharmBase := range supportedJujuBases {
			s.jujuSupportedBases.Add(jujuCharmBase.String())
			if jujuCharmBase.IsCompatible(charmBase) {
				supportedBases = append(supportedBases, charmBase)
				s.logger.Infof(msgUserRequestedBase, charmBase)
			}
		}
	}
	if len(supportedBases) == 0 {
		return nil, errors.NotSupportedf("the charm defined bases %q", printBases(supportedCharmBases))
	}
	return supportedBases, nil
}

// CharmBase determines what base to use with a charm.
// Order of preference is:
//   - user requested with --base or defined by bundle when deploying
//   - model default, if set, acts like --base
//   - juju default ubuntu LTS from charm manifest
//   - first base listed in the charm manifest
//   - in the case of local charms with no manifest nor base in metadata,
//     base must be provided by the user.
func (s BaseSelector) CharmBase() (selectedBase corebase.Base, err error) {
	// TODO(sidecar): handle systems

	// TODO (hml) 2023-05-16
	// BaseSelector needs refinement. It is currently a copy of
	// SeriesSelector, however it does too much for too many
	// cases.

	// User has requested a base with --base.
	if !s.requestedBase.Empty() {
		return s.userRequested(s.requestedBase)
	}

	// Use model default base, if explicitly set and supported by the charm.
	// Cannot guarantee that the requestedBase is either a user supplied base or
	// the DefaultBase model config if supplied.
	if s.explicitDefaultBase {
		return s.userRequested(s.defaultBase)
	}

	// Prefer latest Ubuntu LTS.
	preferredBase, err := BaseForCharm(corebase.LatestLTSBase(), s.supportedBases)
	if err == nil {
		s.logger.Infof(msgLatestLTSBase, corebase.LatestLTSBase())
		return preferredBase, nil
	} else if IsMissingBaseError(err) {
		return corebase.Base{}, err
	}

	// Try juju's current default supported Ubuntu LTS
	jujuDefaultBase, err := BaseForCharm(version.DefaultSupportedLTSBase(), s.supportedBases)
	if err == nil {
		s.logger.Infof(msgLatestLTSBase, version.DefaultSupportedLTSBase())
		return jujuDefaultBase, nil
	}

	// Last chance, the first base in the charm's manifest
	return BaseForCharm(corebase.Base{}, s.supportedBases)
}

// userRequested checks the base the user has requested, and returns it if it
// is supported, or if they used --force.
func (s BaseSelector) userRequested(requestedBase corebase.Base) (corebase.Base, error) {
	// TODO(sidecar): handle computed base
	base, err := BaseForCharm(requestedBase, s.supportedBases)
	if s.force && IsUnsupportedBaseError(err) && s.jujuSupportedBases.Contains(requestedBase.String()) {
		// If the base is unsupported by juju, using force will not
		// apply.
		base = requestedBase
	} else if err != nil {
		if !s.jujuSupportedBases.Contains(requestedBase.String()) {
			return corebase.Base{}, errors.NewNotSupported(nil, fmt.Sprintf("base: %s", requestedBase))
		}
		if IsUnsupportedBaseError(err) {
			return corebase.Base{}, errors.Errorf(
				"base %q is not supported, base series are: %s",
				requestedBase, printBases(s.supportedBases),
			)
		}
		return corebase.Base{}, err
	}
	s.logger.Infof(msgUserRequestedBase, base)
	return base, nil
}

func printBases(bases []corebase.Base) string {
	baseStrings := make([]string, len(bases))
	for i, base := range bases {
		baseStrings[i] = base.DisplayString()
	}
	return strings.Join(baseStrings, ", ")
}
