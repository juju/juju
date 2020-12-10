// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/tags"
	jujustorage "github.com/juju/juju/storage"
)

var (
	jujuDataDir = paths.DataDir(paths.OSUnixLike)
)

type app struct {
	name           string
	clusterName    string
	controllerUUID string
	modelUUID      string
	modelName      string
	deploymentType caas.DeploymentType
	client         ecsiface.ECSAPI
	clock          clock.Clock
}

func newApplication(
	name string,
	clusterName string,
	controllerUUID string,
	modelUUID string,
	modelName string,
	deploymentType caas.DeploymentType,
	client ecsiface.ECSAPI,
	clock clock.Clock,
) caas.Application {
	return &app{
		name:           name,
		clusterName:    clusterName,
		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
		modelName:      modelName,
		deploymentType: deploymentType,
		client:         client,
		clock:          clock,
	}
}

func (a *app) labels(extra map[string]string) map[string]*string {
	tags := tags.ResourceTags(
		names.NewModelTag(a.modelUUID),
		names.NewControllerTag(a.controllerUUID),
		// TODO(ecs): support model config.ResourceTags().
	)
	for k, v := range extra {
		tags[k] = v
	}
	return aws.StringMap(tags)
}

func (a *app) resourceName() string {
	return fmt.Sprintf("%s-%s", a.modelName, a.name)
}

// Delete deletes the specified application.
func (a *app) Delete() error {
	if err := a.deleteService(); err != nil {
		return errors.Trace(err)
	}
	if err := a.deleteTaskDefinitions(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *app) deleteService() error {
	_, err := a.client.DeleteService(&ecs.DeleteServiceInput{
		Cluster: aws.String(a.clusterName),
		Service: aws.String(a.resourceName()),
		Force:   aws.Bool(true),
	})
	err = a.handleErr(err)
	if errors.IsNotFound(err) {
		logger.Tracef("deleting service %q in cluster %q, err -> %v", a.resourceName(), a.clusterName, err)
		return nil
	}
	return errors.Trace(err)
}

const deleteTaskDefinitionTimeout = 30 * time.Second

func (a *app) deleteTaskDefinitions() error {
	ctx, cancel := context.WithTimeout(context.Background(), deleteTaskDefinitionTimeout)
	defer cancel()

	result, err := a.client.ListTaskDefinitionsWithContext(ctx,
		&ecs.ListTaskDefinitionsInput{
			FamilyPrefix: aws.String(a.resourceName()),
		},
	)
	err = a.handleErr(err)
	if errors.IsNotFound(err) {
		logger.Tracef("listing task definitions for family %q in cluster %q, err: %v", a.resourceName(), a.clusterName, err)
		return nil
	}
	if err != nil {
		errors.Trace(err)
	}
	for _, arn := range result.TaskDefinitionArns {
		// Unfortunately, no bulk deregistering API available.
		// And we can't do it concurrently to avoid hitting the API rate limit.
		if err = a.deregisterTaskDefinition(arn); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// deregisterTaskDefinition deregisters the task definition.
// taskDefinitionID can be "family:revision" or full Amazon Resource Name (ARN)
// of the task definition.
func (a *app) deregisterTaskDefinition(taskDefinitionID *string) error {
	if taskDefinitionID == nil {
		return nil
	}
	_, err := a.client.DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: taskDefinitionID,
	})
	err = a.handleErr(err)
	if errors.IsNotFound(err) {
		logger.Tracef("deregistering task definition %q in cluster %q, err: %v", aws.StringValue(taskDefinitionID), a.clusterName, err)
		return nil
	}
	return errors.Trace(err)
}

func strPtrSlice(in ...string) (out []*string) {
	for _, v := range in {
		out = append(out, aws.String(v))
	}
	return out
}

func (a *app) volumeName(storageName string) string {
	// ecs.Volume is not really a resource needs to be created, so we don't need use model name (resourceName()) to prefix it.
	return fmt.Sprintf("%s-%s", a.name, storageName)
}

// getMountPathForFilesystem returns mount path.
func getMountPathForFilesystem(idx int, appName string, fs jujustorage.KubernetesFilesystemParams) string {
	if fs.Attachment != nil {
		return fs.Attachment.Path
	}
	return fmt.Sprintf(
		"%s/fs/%s/%s/%d",
		jujuDataDir,
		appName, fs.StorageName, idx,
	)
}

func (a *app) handleFileSystems(filesystems []jujustorage.KubernetesFilesystemParams) (vols []*ecs.Volume, mounts []*ecs.MountPoint, err error) {
	vols = make([]*ecs.Volume, len(filesystems))
	mounts = make([]*ecs.MountPoint, len(filesystems))

	volNames := set.NewStrings()
	for idx, fs := range filesystems {
		if volNames.Contains(fs.StorageName) {
			return nil, nil, errors.NotValidf("duplicated volume %q", fs.StorageName)
		}
		volNames.Add(fs.StorageName)

		ebsCfg, err := newEbsConfig(fs.Attributes)
		if err != nil {
			// This should never happen because it's been validated `storageProvider.ValidateConfig`.
			return nil, nil, errors.NotValidf("storage attribute for %q", fs.StorageName)
		}
		vol := &ecs.Volume{
			Name: aws.String(a.volumeName(fs.StorageName)),
			DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
				Scope:         aws.String("shared"),
				Autoprovision: aws.Bool(true),
				Driver:        aws.String(ebsCfg.driver),
				Labels:        a.labels(fs.ResourceTags),
				DriverOpts: map[string]*string{
					"volumetype": aws.String(ebsCfg.volumeType),
					"size":       aws.String(strconv.FormatUint(fs.Size/1024, 10)), // unit of size here should be `Gi`
				},
			},
		}
		vols[idx] = vol

		readOnly := false
		if fs.Attachment != nil {
			readOnly = fs.Attachment.ReadOnly
		}
		mounts[idx] = &ecs.MountPoint{
			ContainerPath: aws.String(getMountPathForFilesystem(
				idx, a.name, fs,
			)),
			SourceVolume: vol.Name,
			ReadOnly:     aws.Bool(readOnly),
		}
	}
	return vols, mounts, nil
}

func (a *app) applicationTaskDefinition(config caas.ApplicationConfig) (*ecs.RegisterTaskDefinitionInput, error) {

	var containerNames []string
	var containers []caas.ContainerConfig
	for _, v := range config.Containers {
		containerNames = append(containerNames, v.Name)
		containers = append(containers, v)
	}
	sort.Strings(containerNames)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	volumes, volumeMounts, err := a.handleFileSystems(config.Filesystems)
	if err != nil {
		return nil, errors.Trace(err)
	}
	input := &ecs.RegisterTaskDefinitionInput{
		Family:      aws.String(a.resourceName()),
		TaskRoleArn: aws.String(""),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			// init container
			{
				Name:             aws.String("charm-init"),
				Image:            aws.String(config.AgentImagePath),
				WorkingDirectory: aws.String(jujuDataDir),
				Cpu:              aws.Int64(10),
				Memory:           aws.Int64(512),
				Essential:        aws.Bool(false),
				EntryPoint:       strPtrSlice("/opt/k8sagent"),
				DockerLabels:     a.labels(nil),
				Command: strPtrSlice(
					"init",
					"--data-dir",
					jujuDataDir,
					"--bin-dir",
					"/charm/bin",
				),
				Environment: []*ecs.KeyValuePair{
					{
						Name:  aws.String("JUJU_CONTAINER_NAMES"),
						Value: aws.String(strings.Join(containerNames, ",")),
					},
					{
						// TODO(ecs): remove me when we solve the ECS unit's identity issue.
						Name:  aws.String("JUJU_K8S_POD_NAME"),
						Value: aws.String("cockroachdb-0"),
					},
					{
						// TODO(ecs): remove me when we solve the ECS unit's identity issue.
						Name:  aws.String("JUJU_K8S_POD_UUID"),
						Value: aws.String("c83b286e-8f45-4dbf-b2a6-0c393d93bd6a"),
					},
					// appSecret
					{
						Name:  aws.String("JUJU_K8S_APPLICATION"),
						Value: aws.String(a.name),
					},
					{
						Name:  aws.String("JUJU_K8S_MODEL"),
						Value: aws.String(a.modelUUID),
					},
					{
						Name:  aws.String("JUJU_K8S_APPLICATION_PASSWORD"),
						Value: aws.String(config.IntroductionSecret),
					},
					{
						Name:  aws.String("JUJU_K8S_CONTROLLER_ADDRESSES"),
						Value: aws.String(config.ControllerAddresses),
					},
					{
						Name:  aws.String("JUJU_K8S_CONTROLLER_CA_CERT"),
						Value: aws.String(config.ControllerCertBundle),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						ContainerPath: aws.String(jujuDataDir),
						SourceVolume:  aws.String("var-lib-juju"),
					},
					{
						ContainerPath: aws.String("/charm/bin"),
						SourceVolume:  aws.String("charm-data-bin"),
					},
					// DO we need this in init container?
					// {
					// 	ContainerPath: aws.String("/charm/containers"),
					// 	SourceVolume:  aws.String("charm-data-containers"),
					// },
				},
			},
		},
		Volumes: append(volumes, []*ecs.Volume{
			// TODO(ecs): ensure no vol.Name conflict.
			{
				Name: aws.String("var-lib-juju"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: a.labels(nil),
				},
			},
			{
				Name: aws.String("charm-data-bin"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: a.labels(nil),
				},
			},
		}...),
	}
	// container agent.
	charmContainerDefinition := &ecs.ContainerDefinition{
		Name:             aws.String("charm"),
		Image:            aws.String(config.AgentImagePath),
		WorkingDirectory: aws.String(jujuDataDir),
		Cpu:              aws.Int64(10),
		Memory:           aws.Int64(512),
		DependsOn: []*ecs.ContainerDependency{
			{
				ContainerName: aws.String("charm-init"),
				Condition:     aws.String("SUCCESS"),
			},
		},
		Essential:    aws.Bool(true),
		EntryPoint:   strPtrSlice("/charm/bin/k8sagent"),
		DockerLabels: a.labels(nil),
		Command: strPtrSlice(
			"unit",
			"--data-dir", jujuDataDir,
			"--charm-modified-version", strconv.Itoa(config.CharmModifiedVersion),
			"--append-env",
			"PATH=$PATH:/charm/bin",
		),
		// TODO(ecs): Health check/prob
		Environment: []*ecs.KeyValuePair{
			{
				Name:  aws.String("JUJU_CONTAINER_NAMES"),
				Value: aws.String(strings.Join(containerNames, ",")),
			},
			{
				Name: aws.String(
					"HTTP_PROBE_PORT", // constants.EnvAgentHTTPProbePort
				),
				Value: aws.String(
					"3856", // constants.AgentHTTPProbePort
				),
			},
		},
		MountPoints: []*ecs.MountPoint{
			{
				ContainerPath: aws.String(jujuDataDir),
				SourceVolume:  aws.String("var-lib-juju"),
			},
			{
				ContainerPath: aws.String("/charm/bin"),
				SourceVolume:  aws.String("charm-data-bin"),
			},
		},
	}

	for _, v := range containers {
		// TODO(ecs): https://aws.amazon.com/blogs/compute/amazon-ecs-and-docker-volume-drivers-amazon-ebs/
		// to use EBS volumes, it requires some docker storage plugin installed in the
		// container instance!
		// Add precheck for storage support!
		container := &ecs.ContainerDefinition{
			Name:  aws.String(v.Name),
			Image: aws.String(v.Image.RegistryPath),
			DependsOn: []*ecs.ContainerDependency{
				{
					ContainerName: aws.String("charm-init"),
					Condition:     aws.String("SUCCESS"),
				},
			},
			Cpu:          aws.Int64(10),
			Memory:       aws.Int64(512),
			Essential:    aws.Bool(true),
			EntryPoint:   strPtrSlice("/charm/bin/pebble"),
			DockerLabels: a.labels(nil),
			Command: strPtrSlice(
				"listen",
				"--socket", "/charm/container/pebble.sock",
				"--append-env", "PATH=$PATH:/charm/bin",
			),
			// TODO: Health check/prob
			Environment: []*ecs.KeyValuePair{
				{
					Name:  aws.String("JUJU_CONTAINER_NAME"),
					Value: aws.String(v.Name),
				},
			},
			MountPoints: append(volumeMounts,
				// TODO: ensure no vol.Name conflict.
				&ecs.MountPoint{
					ContainerPath: aws.String("/charm/bin"),
					SourceVolume:  aws.String("charm-data-bin"),
					ReadOnly:      aws.Bool(true),
				},
			),
		}
		charmContainerDefinition.DependsOn = append(charmContainerDefinition.DependsOn, &ecs.ContainerDependency{
			ContainerName: container.Name,
			Condition:     aws.String("START"),
		})
		volume := &ecs.Volume{
			Name: aws.String(fmt.Sprintf("charm-data-container-%s", v.Name)),
			DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
				Scope:  aws.String("task"),
				Driver: aws.String("local"),
				Labels: a.labels(nil),
			},
		}
		input.Volumes = append(input.Volumes, volume)
		container.MountPoints = append(container.MountPoints, &ecs.MountPoint{
			ContainerPath: aws.String("/charm/container"),
			SourceVolume:  volume.Name,
		})
		input.ContainerDefinitions = append(input.ContainerDefinitions, container)
		charmContainerDefinition.MountPoints = append(charmContainerDefinition.MountPoints, &ecs.MountPoint{
			ContainerPath: aws.String(fmt.Sprintf("/charm/containers/%s", v.Name)),
			SourceVolume:  volume.Name,
		})
	}
	input.ContainerDefinitions = append(input.ContainerDefinitions, charmContainerDefinition)
	return input, nil
}

// Ensure creates or updates an application pod with the given application
// name, agent path, and application config.
func (a *app) Ensure(config caas.ApplicationConfig) (err error) {
	result, err := a.registerTaskDefinition(config)
	if err != nil {
		return errors.Trace(err)
	}
	taskDefinitionID := fmt.Sprintf(
		"%s:%s",
		aws.StringValue(result.TaskDefinition.Family),
		strconv.FormatInt(aws.Int64Value(result.TaskDefinition.Revision), 10),
	)
	return errors.Trace(a.ensureECSService(taskDefinitionID))
}

// Exists indicates if the application for the specified
// application exists, and whether the application is terminating.
func (a *app) Exists() (caas.DeploymentState, error) {
	// TODO(ecs)
	return caas.DeploymentState{}, nil
}

func (a *app) State() (caas.ApplicationState, error) {
	// TODO(ecs)
	return caas.ApplicationState{}, nil
}

func computeStatus(ctx context.Context, t *ecs.Task) (statusMessage string, jujuStatus status.Status, since time.Time) {
	// TODO(ecs)
	if t.StoppedAt != nil || t.StoppingAt != nil {
		since = aws.TimeValue(t.StoppedAt)
		if t.StoppedAt == nil {
			since = aws.TimeValue(t.StoppingAt)
		}
		return "", status.Terminated, since
	}
	jujuStatus = status.Unknown
	healthStatus := aws.StringValue(t.HealthStatus)
	switch healthStatus {
	case "UNKNOWN":
	case "RUNNING":
		jujuStatus = status.Running
	case "UNHEALTHY":
		jujuStatus = status.Error
	case "PENDING":
		jujuStatus = status.Allocating
	}
	statusMessage = aws.StringValue(t.StoppedReason)
	// since = now ??
	return statusMessage, jujuStatus, since
}

// Units of the application fetched from kubernetes by matching pod labels.
func (a *app) Units() (units []caas.Unit, err error) {
	ctx := context.Background()

	result, err := a.client.ListTasks(&ecs.ListTasksInput{
		Cluster:     aws.String(a.clusterName),
		ServiceName: aws.String(a.resourceName()),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	tasks, err := a.client.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: aws.String(a.clusterName),
		Tasks:   result.TaskArns,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(tasks.Failures) > 0 {
		failures := ""
		for _, failure := range tasks.Failures {
			failures = " | " + failure.String()
		}
		logger.Warningf("a.client.DescribeTasks(%#v), tasks.Failures: %q", result.TaskArns, failures)
	}
	for _, t := range tasks.Tasks {
		logger.Warningf("Units() task -> %s", pretty.Sprint(t))
		statusMessage, unitStatus, since := computeStatus(ctx, t)
		unitInfo := caas.Unit{
			// Id:       aws.StringValue(t.TaskArn),
			Id:       "cockroachdb-0", // !!!
			Address:  "",
			Ports:    nil,
			Dying:    t.StoppedAt != nil || t.StoppingAt != nil,
			Stateful: a.deploymentType == caas.DeploymentStateful, // ??????????
			Status: status.StatusInfo{
				Status:  unitStatus,
				Message: statusMessage,
				Since:   &since,
			},
			FilesystemInfo: []caas.FilesystemInfo{
				{
					Size:         1,
					FilesystemId: "cockroachdb-0",
					MountPoint:   "/var/lib/juju/storage/database/0",
					ReadOnly:     false,
					Status: status.StatusInfo{
						Status: status.Attached,
						Since:  &since,
					},
					Volume: caas.VolumeInfo{
						VolumeId:   "cockroachdb-0",
						Size:       1,
						Persistent: false,
						Status: status.StatusInfo{
							Status: status.Attached,
							Since:  &since,
						},
					},
				},
			},
		}
		units = append(units, unitInfo)
	}
	return units, nil
}

// UpdatePorts updates port mappings on the specified service.
func (a *app) UpdatePorts(ports []caas.ServicePort, updateContainerPorts bool) error {
	// TODO(ecs)
	return nil
}

// UpdateService updates the default service with specific service type and port mappings.
func (a *app) UpdateService(param caas.ServiceParam) error {
	// TODO(ecs)
	return nil
}

func errorOrFailures(err error, failures []*ecs.Failure) error {
	if err != nil {
		return errors.Trace(err)
	}
	if len(failures) == 0 {
		return nil
	}
	var errStrs []string
	for _, failure := range failures {
		errStrs = append(errStrs, failure.String())
	}
	return errors.New(strings.Join(errStrs, ":"))
}

// Watch returns a watcher which notifies when there
// are changes to the application of the specified application.
func (a *app) Watch() (watcher.NotifyWatcher, error) {
	var lastEventID string
	hasNewEvents := func() (bool, error) {
		result, err := a.client.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  aws.String(a.clusterName),
			Services: []*string{aws.String(a.resourceName())},
		})
		err = errorOrFailures(err, result.Failures)
		if err != nil {
			return false, errors.Trace(err)
		}
		if len(result.Services) == 0 {
			return false, nil
		}
		svc := result.Services[0]
		if len(svc.Events) == 0 {
			return false, nil
		}
		lastestEventID := aws.StringValue(svc.Events[0].Id)
		logger.Tracef("lastestEvent -> %s", svc.Events[0])
		if lastEventID != lastestEventID {
			lastEventID = lastestEventID
			return true, nil
		}
		return false, nil
	}
	// Implement API result cache for better performance.
	return newNotifyWatcher(a.name, a.clock, hasNewEvents)
}

// WatchReplicas returns a watcher for watching the number of units changes.
func (a *app) WatchReplicas() (watcher.NotifyWatcher, error) {
	// TODO(ecs)
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return watchertest.NewMockNotifyWatcher(ch), nil
}

func (a *app) registerTaskDefinition(config caas.ApplicationConfig) (*ecs.RegisterTaskDefinitionOutput, error) {
	input, err := a.applicationTaskDefinition(config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := a.client.RegisterTaskDefinition(input)
	err = a.handleErr(err)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (a *app) ensureECSService(taskDefinitionID string) (err error) {
	updateInput := &ecs.UpdateServiceInput{
		Cluster:        aws.String(a.clusterName),
		DesiredCount:   aws.Int64(1),
		Service:        aws.String(a.resourceName()),
		TaskDefinition: aws.String(taskDefinitionID),
	}
	result, err := a.client.UpdateService(updateInput)
	logger.Tracef("ensuring service updating %q err: %v result: %s", taskDefinitionID, err, pretty.Sprint(result))
	err = a.handleErr(err)
	if errors.IsNotFound(err) {
		createInput := &ecs.CreateServiceInput{
			Cluster:        aws.String(a.clusterName),
			DesiredCount:   aws.Int64(1),
			ServiceName:    aws.String(a.resourceName()),
			TaskDefinition: aws.String(taskDefinitionID),
		}
		var createResult *ecs.CreateServiceOutput
		createResult, err = a.client.CreateService(createInput)
		logger.Tracef("ensuring service creating %q err: %v result: %s", taskDefinitionID, err, pretty.Sprint(createResult))
		err = a.handleErr(err)
	}
	return errors.Trace(err)
}

func (a *app) handleErr(err error) error {
	if err == nil {
		return nil
	}
	aerr, ok := err.(awserr.Error)
	if !ok {
		return err
	}

	switch aerr.Code() {
	case ecs.ErrCodeServerException:
	case ecs.ErrCodeClientException:
	case ecs.ErrCodeInvalidParameterException:
		return errors.NewNotValid(err, aerr.Message())
	case ecs.ErrCodeClusterNotFoundException:
		return errors.NewNotFound(err, fmt.Sprintf("cluster %q", a.clusterName))
	case ecs.ErrCodeUnsupportedFeatureException:
		return errors.NewNotSupported(err, aerr.Message())
	case ecs.ErrCodePlatformUnknownException:
	case ecs.ErrCodePlatformTaskDefinitionIncompatibilityException:
	case ecs.ErrCodeAccessDeniedException:
	case ecs.ErrCodeServiceNotFoundException, ecs.ErrCodeServiceNotActiveException:
		return errors.NewNotFound(err, aerr.Message())
	default:
		logger.Errorf("unknown error: %v", aerr.Error())
	}
	return err
}
