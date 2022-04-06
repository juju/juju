package grpcserver

import (
	"github.com/juju/charm/v8"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/application"
	apiapplication "github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	apiclient "github.com/juju/juju/api/client/client"
	apimodelconfig "github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/version"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/series"
	"github.com/juju/names/v4"
)

// deployCharmArgs is the set of arguments required to deploy a charm
type deployCharmArgs struct {
	CharmName       string            // The charm to deploy (required)
	ApplicationName string            // Name of the deployed application (defaults to CharmName)
	NumUnits        int               // Number of units to deploy (required)
	Channel         charm.Channel     // Charm channel
	Revision        int               // Charm revision (-1 means unspecified?)
	Series          string            // Charm series (optional)
	WorkloadSeries  set.Strings       // Series available for workloads (optional)
	Constraints     constraints.Value // Deployment constraints (optional)
	ImageStream     string            // Used to find workload series if WorkloadSeries not provided
	Force           bool              // Set to true, bypasses some checks
}

// deployCharm deploys a charm.
func deployCharm(conn api.Connection, args deployCharmArgs) error {

	// Get the required facade clients
	var (
		charmsAPIClient      = apicharms.NewClient(conn)
		clientAPIClient      = apiclient.NewClient(conn)
		applicationAPIClient = apiapplication.NewClient(conn)
		modelconfigAPIClient = apimodelconfig.NewClient(conn)
	)

	// Figure out the application name if not provided
	appName := args.ApplicationName
	if appName == "" {
		appName = args.CharmName
	}
	if err := names.ValidateApplicationName(appName); err != nil {
		return errors.Trace(err)
	}

	// Get the model config
	attrs, err := modelconfigAPIClient.ModelGet()
	if err != nil {
		return errors.Wrap(err, errors.New("cannot fetch model settings"))
	}
	modelConfig, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	// Get the charm URL
	defaultCharmSchema := charm.CharmHub
	if charmsAPIClient.BestAPIVersion() < 3 {
		defaultCharmSchema = charm.CharmStore
	}
	path, err := charm.EnsureSchema(args.CharmName, defaultCharmSchema)
	if err != nil {
		return errors.Trace(err)
	}
	charmURL, err := charm.ParseURL(path)
	if err != nil {
		return errors.Trace(err)
	}
	// To deploy by revision, the revision number must be in the origin for a
	// charmhub charm and in the url for a charmstore charm.
	if charm.CharmHub.Matches(charmURL.Schema) {
		if charmURL.Revision != -1 {
			return errors.Errorf("cannot specify revision in a charm or bundle name")
		}
		if args.Revision != -1 && args.Channel.Empty() {
			return errors.Errorf("specifying a revision requires a channel for future upgrades")
		}
	} else if charm.CharmStore.Matches(charmURL.Schema) {
		if charmURL.Revision != -1 && args.Revision != -1 && charmURL.Revision != args.Revision {
			return errors.Errorf("two different revisions to deploy: specified %d and %d, please choose one.", charmURL.Revision, args.Revision)
		}
		if charmURL.Revision == -1 && args.Revision != -1 {
			charmURL = charmURL.WithRevision(args.Revision)
		}
	}

	// Resolve the charm
	modelConstraints, err := clientAPIClient.GetModelConstraints()
	if err != nil {
		return errors.Trace(err)
	}
	platform, err := utils.DeducePlatform(args.Constraints, args.Series, modelConstraints)
	if err != nil {
		return errors.Trace(err)
	}
	urlForOrigin := charmURL
	if args.Revision != -1 {
		urlForOrigin = urlForOrigin.WithRevision(args.Revision)
	}
	origin, err := utils.DeduceOrigin(urlForOrigin, args.Channel, platform)
	if err != nil {
		return errors.Trace(err)
	}
	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store but pass in the origin command line
	// argument so users can target a specific origin.
	rev := -1
	origin.Revision = &rev
	resolved, err := charmsAPIClient.ResolveCharms([]apicharms.CharmToResolve{{URL: charmURL, Origin: origin}})
	if err != nil {
		return errors.Trace(err)
	}
	if len(resolved) != 1 {
		return errors.Errorf("expected only one resolution, received %d", len(resolved))
	}
	selectedCharm := resolved[0]

	// Figure out the workload series if not provided
	workloadSeries := args.WorkloadSeries
	if workloadSeries == nil {
		imageStream := args.ImageStream
		if imageStream == "" {
			imageStream = modelConfig.ImageStream()
		}
		workloadSeries, err = series.WorkloadSeries(clock.WallClock.Now(), charmURL.Series, imageStream)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Figure out the actual series of the charm
	var series string
	switch {
	case args.Series != "":
		// Explicitly request series.
		series = args.Series
	case charmURL.Series != "":
		// Series specified in charm URL.
		series = charmURL.Series
	default:
		// First try using the default model series if explicitly set, provided
		// it is supported by the charm.
		var explicit bool
		series, explicit = modelConfig.DefaultSeries()
		if explicit {
			_, err := charm.SeriesForCharm(series, selectedCharm.SupportedSeries)
			if err == nil {
				break
			}
		}
		// Else use the selected charm's list of series, filtering by what Juju
		// supports.
		var supportedSeries []string
		for _, s := range selectedCharm.SupportedSeries {
			if workloadSeries.Contains(s) {
				supportedSeries = append(supportedSeries, s)
			}
		}
		series, err = charm.SeriesForCharm("", supportedSeries)
		if err == nil {
			break
		}
		if !args.Force {
			return errors.Trace(err)
		}
		// Finally, because we are forced we choose LTS
		series = version.DefaultSupportedLTS()
	}

	// Select an actually supported series
	if !args.Force {
		series, err = charm.SeriesForCharm(series, selectedCharm.SupportedSeries)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Validate the series
	if !args.Force && !workloadSeries.Contains(series) {
		return errors.NotSupportedf("series: %s", series)
	}

	// Add the charm to the model
	origin = selectedCharm.Origin.WithSeries(series)
	if charm.CharmHub.Matches(charmURL.Schema) {
		charmURL = selectedCharm.URL.WithRevision(*origin.Revision).WithArchitecture(origin.Architecture).WithSeries(series)
	} else if charm.CharmStore.Matches(charmURL.Schema) {
		charmURL = selectedCharm.URL
		origin.Revision = &charmURL.Revision
	}
	resultOrigin, err := charmsAPIClient.AddCharm(charmURL, origin, args.Force)
	if err != nil {
		return errors.Trace(err)
	}

	// Finally, deploy the charm!
	return applicationAPIClient.Deploy(application.DeployArgs{
		CharmID: application.CharmID{
			URL:    charmURL,
			Origin: resultOrigin,
		},
		ApplicationName: appName,
		Series:          resultOrigin.Series,
		NumUnits:        args.NumUnits,
		Cons:            args.Constraints,
	})
}
