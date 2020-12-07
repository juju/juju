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
	"github.com/juju/errors"
	"github.com/kr/pretty"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	jujustorage "github.com/juju/juju/storage"
)

var (
	jujuDataDir = paths.DataDir(paths.OSUnixLike)
)

type app struct {
	name           string
	clusterName    string
	modelUUID      string
	modelName      string
	deploymentType caas.DeploymentType
	client         ecsiface.ECSAPI
	clock          clock.Clock
}

func newApplication(
	name string,
	clusterName string,
	modelUUID string,
	modelName string,
	deploymentType caas.DeploymentType,
	client ecsiface.ECSAPI,
	clock clock.Clock,
) caas.Application {
	// TODO: prefix modelName to all resource names?
	// Because ecs doesnot have namespace!!!
	// name = modelName + "-" + name
	return &app{
		name:           name,
		clusterName:    clusterName,
		modelUUID:      modelUUID,
		modelName:      modelName,
		deploymentType: deploymentType,
		client:         client,
		clock:          clock,
	}
}

func (a *app) labels() map[string]*string {
	// TODO
	return map[string]*string{
		"App":       aws.String(a.name),
		"ModelName": aws.String(a.modelName),
		"ModelUUID": aws.String(a.modelUUID),
	}
}

// Delete deletes the specified application.
func (a *app) Delete() error {
	return nil
}

func strPtrSlice(in ...string) (out []*string) {
	for _, v := range in {
		out = append(out, aws.String(v))
	}
	return out
}

func (a *app) volumeName(storageName string) string {
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
	for idx, fs := range filesystems {
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
				Driver:        aws.String(ebsCfg.driver), // TODO: fs.Attributes["Driver"] ?????
				Labels:        a.labels(),                // TODO: merge with fs.ResourceTags !!!
				DriverOpts: map[string]*string{
					"volumetype": aws.String(ebsCfg.volumeType),                    // TODO!!!
					"size":       aws.String(strconv.FormatUint(fs.Size/1024, 10)), // unit of size here should be `Gi`
				},
			},
		}
		vols = append(vols, vol)

		readOnly := false
		if fs.Attachment != nil {
			readOnly = fs.Attachment.ReadOnly
		}
		mounts = append(mounts, &ecs.MountPoint{
			ContainerPath: aws.String(getMountPathForFilesystem(
				idx, a.name, fs,
			)),
			SourceVolume: vol.Name,
			ReadOnly:     aws.Bool(readOnly),
		})
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
		Family:      aws.String(a.name),
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
						Name:  aws.String("JUJU_K8S_POD_NAME"),
						Value: aws.String("cockroachdb-0"),
					},
					{
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
			// TODO: ensure no vol.Name conflict.
			{
				Name: aws.String("var-lib-juju"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: a.labels(),
				},
			},
			{
				Name: aws.String("charm-data-bin"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: a.labels(),
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
		Essential:  aws.Bool(true),
		EntryPoint: strPtrSlice("/charm/bin/k8sagent"),
		Command: strPtrSlice(
			"unit",
			"--data-dir", jujuDataDir,
			"--charm-modified-version", strconv.Itoa(config.CharmModifiedVersion),
			"--append-env",
			"PATH=$PATH:/charm/bin",
		),
		// TODO: Health check/prob
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
		// TODO: https://aws.amazon.com/blogs/compute/amazon-ecs-and-docker-volume-drivers-amazon-ebs/
		// to use EBS volumes, it requires some docker storage plugin installed in the
		// container instance!!!
		// disable persistence storage or Juju have to manage those plugins????
		container := &ecs.ContainerDefinition{
			Name:  aws.String(v.Name),
			Image: aws.String(v.Image.RegistryPath),
			DependsOn: []*ecs.ContainerDependency{
				// {
				// 	ContainerName: aws.String("charm"),
				// 	Condition:     aws.String("START"),
				// },
				{
					ContainerName: aws.String("charm-init"),
					Condition:     aws.String("SUCCESS"),
				},
			},
			Cpu:        aws.Int64(10),
			Memory:     aws.Int64(512),
			Essential:  aws.Bool(true),
			EntryPoint: strPtrSlice("/charm/bin/pebble"),
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
				Labels: a.labels(),
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
	logger.Criticalf("app.Ensure config -> %s", pretty.Sprint(config))
	logger.Criticalf("app.Ensure a.labels() -> %s", pretty.Sprint(a.labels()))
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
	return caas.DeploymentState{}, nil
}

func (a *app) State() (caas.ApplicationState, error) {
	return caas.ApplicationState{}, nil
}

// !!!
func computeStatus(ctx context.Context, t *ecs.Task) (statusMessage string, jujuStatus status.Status, since time.Time) {
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
		// Family:      aws.String(a.name), // TODO: model prefixing????
		Cluster:     aws.String(a.clusterName),
		ServiceName: aws.String(a.name), // TODO: model prefixing????
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
		logger.Warningf("a.client.DescribeTasks(%#v), tasks.Failures -> %q", result.TaskArns, failures)
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
	return nil
}

// UpdateService updates the default service with specific service type and port mappings.
func (a *app) UpdateService(param caas.ServiceParam) error {
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
			Services: []*string{aws.String(a.name)}, // TODO: model prefixing????
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
		logger.Warningf("lastestEvent -> %s", svc.Events[0])
		if lastEventID != lastestEventID {
			lastEventID = lastestEventID
			return true, nil
		}
		return false, nil
	}
	return newNotifyWatcher(a.name, a.clock, hasNewEvents)
}

func (a *app) WatchReplicas() (watcher.NotifyWatcher, error) {
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
	logger.Criticalf("app.Ensure err -> %#v result -> %s", err, pretty.Sprint(result))
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ecs.ErrCodeServerException:
				logger.Errorf(ecs.ErrCodeServerException + " -> " + aerr.Error())
			case ecs.ErrCodeClientException:
				logger.Errorf(ecs.ErrCodeClientException + " -> " + aerr.Error())
			case ecs.ErrCodeInvalidParameterException:
				logger.Errorf(ecs.ErrCodeInvalidParameterException + " -> " + aerr.Error())
			default:
				logger.Errorf(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			logger.Errorf(err.Error())
		}
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (a *app) ensureECSService(taskDefinitionID string) (err error) {
	handleErr := func(err error) error {
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case ecs.ErrCodeServerException:
					logger.Errorf(ecs.ErrCodeServerException + " -> " + aerr.Error())
				case ecs.ErrCodeClientException:
					logger.Errorf(ecs.ErrCodeClientException + " -> " + aerr.Error())
				case ecs.ErrCodeInvalidParameterException:
					logger.Errorf(ecs.ErrCodeInvalidParameterException + " -> " + aerr.Error())
				case ecs.ErrCodeClusterNotFoundException:
					logger.Errorf(ecs.ErrCodeClusterNotFoundException + " -> " + aerr.Error())
				case ecs.ErrCodeUnsupportedFeatureException:
					logger.Errorf(ecs.ErrCodeUnsupportedFeatureException + " -> " + aerr.Error())
				case ecs.ErrCodePlatformUnknownException:
					logger.Errorf(ecs.ErrCodePlatformUnknownException + " -> " + aerr.Error())
				case ecs.ErrCodePlatformTaskDefinitionIncompatibilityException:
					logger.Errorf(ecs.ErrCodePlatformTaskDefinitionIncompatibilityException + " -> " + aerr.Error())
				case ecs.ErrCodeAccessDeniedException:
					logger.Errorf(ecs.ErrCodeAccessDeniedException + " -> " + aerr.Error())
				case ecs.ErrCodeServiceNotFoundException, ecs.ErrCodeServiceNotActiveException:
					logger.Errorf(aerr.Code() + " -> " + aerr.Error())
					return errors.NewNotFound(aerr, aerr.Error())
				default:
					logger.Errorf(aerr.Error())
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				logger.Errorf(err.Error())
			}
		}
		return err
	}

	updateInput := &ecs.UpdateServiceInput{
		Cluster:        aws.String(a.clusterName),
		DesiredCount:   aws.Int64(1),
		Service:        aws.String(a.name), // TODO: model prefixing????
		TaskDefinition: aws.String(taskDefinitionID),
	}
	result, err := a.client.UpdateService(updateInput)
	logger.Criticalf("app.ensureECSService UpdateService %q err -> %#v result -> %s", taskDefinitionID, err, pretty.Sprint(result))
	err = handleErr(err)
	if errors.IsNotFound(err) {
		createInput := &ecs.CreateServiceInput{
			Cluster:        aws.String(a.clusterName),
			DesiredCount:   aws.Int64(1),
			ServiceName:    aws.String(a.name), // TODO: model prefixing????
			TaskDefinition: aws.String(taskDefinitionID),
		}
		result, err := a.client.CreateService(createInput)
		logger.Criticalf("app.ensureECSService CreateService %q err -> %#v result -> %s", taskDefinitionID, err, pretty.Sprint(result))
		err = handleErr(err)
	}
	return errors.Trace(err)
}
