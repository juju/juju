// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/storage"
)

type applicationSuite struct {
	baseSuite

	appName string
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.appName = "gitlab"
}

func (s *applicationSuite) getApp(c *gc.C, deploymentType caas.DeploymentType) (caas.Application, *gomock.Controller) {
	ctrl := s.setupController(c)
	return s.environ.Application(s.appName, deploymentType), ctrl
}

func strPtrSlice(in ...string) (out []*string) {
	for _, v := range in {
		out = append(out, aws.String(v))
	}
	return out
}

func (s *applicationSuite) assertEnsure(c *gc.C, app caas.Application, assertCalls ...*gomock.Call) {
	registerTaskDefinitionInput := &ecs.RegisterTaskDefinitionInput{
		Family:      aws.String("test-gitlab"),
		TaskRoleArn: aws.String(""),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			// init container
			{
				Name:             aws.String("charm-init"),
				Image:            aws.String("operator/image-path"),
				WorkingDirectory: aws.String("/var/lib/juju"),
				Cpu:              aws.Int64(10),
				Memory:           aws.Int64(512),
				Essential:        aws.Bool(false),
				EntryPoint:       strPtrSlice("/opt/k8sagent"),
				DockerLabels: map[string]*string{
					"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
					"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
				},
				Command: strPtrSlice(
					"init",
					"--data-dir",
					"/var/lib/juju",
					"--bin-dir",
					"/charm/bin",
				),
				Environment: []*ecs.KeyValuePair{
					{
						Name:  aws.String("JUJU_CONTAINER_NAMES"),
						Value: aws.String("gitlab"),
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
						Value: aws.String(s.appName),
					},
					{
						Name:  aws.String("JUJU_K8S_MODEL"),
						Value: aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
					},
					{
						Name:  aws.String("JUJU_K8S_APPLICATION_PASSWORD"),
						Value: aws.String(""),
					},
					{
						Name:  aws.String("JUJU_K8S_CONTROLLER_ADDRESSES"),
						Value: aws.String(""),
					},
					{
						Name:  aws.String("JUJU_K8S_CONTROLLER_CA_CERT"),
						Value: aws.String(""),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						ContainerPath: aws.String("/var/lib/juju"),
						SourceVolume:  aws.String("var-lib-juju"),
					},
					{
						ContainerPath: aws.String("/charm/bin"),
						SourceVolume:  aws.String("charm-data-bin"),
					},
				},
			},
			{
				Name:  aws.String("gitlab"),
				Image: aws.String("gitlab-image:latest"),
				DependsOn: []*ecs.ContainerDependency{
					{
						ContainerName: aws.String("charm-init"),
						Condition:     aws.String("SUCCESS"),
					},
				},
				Cpu:        aws.Int64(10),
				Memory:     aws.Int64(512),
				Essential:  aws.Bool(true),
				EntryPoint: strPtrSlice("/charm/bin/pebble"),
				DockerLabels: map[string]*string{
					"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
					"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
				},
				Command: strPtrSlice(
					"listen",
					"--socket", "/charm/container/pebble.sock",
					"--append-env", "PATH=$PATH:/charm/bin",
				),
				Environment: []*ecs.KeyValuePair{
					{
						Name:  aws.String("JUJU_CONTAINER_NAME"),
						Value: aws.String("gitlab"),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						ContainerPath: aws.String("path/to/here"),
						SourceVolume:  aws.String("gitlab-database"),
						ReadOnly:      aws.Bool(false),
					},
					{
						ContainerPath: aws.String("/charm/bin"),
						SourceVolume:  aws.String("charm-data-bin"),
						ReadOnly:      aws.Bool(true),
					},
					{
						ContainerPath: aws.String("/charm/container"),
						SourceVolume:  aws.String("charm-data-container-gitlab"),
					},
				},
			},
			{
				Name:             aws.String("charm"),
				Image:            aws.String("operator/image-path"),
				WorkingDirectory: aws.String("/var/lib/juju"),
				Cpu:              aws.Int64(10),
				Memory:           aws.Int64(512),
				DependsOn: []*ecs.ContainerDependency{
					{
						ContainerName: aws.String("charm-init"),
						Condition:     aws.String("SUCCESS"),
					},
					{
						ContainerName: aws.String("gitlab"),
						Condition:     aws.String("START"),
					},
				},
				Essential:  aws.Bool(true),
				EntryPoint: strPtrSlice("/charm/bin/k8sagent"),
				DockerLabels: map[string]*string{
					"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
					"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
				},
				Command: strPtrSlice(
					"unit",
					"--data-dir", "/var/lib/juju",
					"--charm-modified-version", "9001",
					"--append-env",
					"PATH=$PATH:/charm/bin",
				),
				Environment: []*ecs.KeyValuePair{
					{
						Name:  aws.String("JUJU_CONTAINER_NAMES"),
						Value: aws.String("gitlab"),
					},
					{
						Name: aws.String(
							"HTTP_PROBE_PORT",
						),
						Value: aws.String(
							"3856",
						),
					},
				},
				MountPoints: []*ecs.MountPoint{
					{
						ContainerPath: aws.String("/var/lib/juju"),
						SourceVolume:  aws.String("var-lib-juju"),
					},
					{
						ContainerPath: aws.String("/charm/bin"),
						SourceVolume:  aws.String("charm-data-bin"),
					},
					{
						ContainerPath: aws.String("/charm/containers/gitlab"),
						SourceVolume:  aws.String("charm-data-container-gitlab"),
					},
				},
			},
		},
		Volumes: []*ecs.Volume{
			{
				Name: aws.String("gitlab-database"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Autoprovision: aws.Bool(true),
					Scope:         aws.String("shared"),
					Driver:        aws.String("rexray/ebs"),
					DriverOpts: map[string]*string{
						"volumetype": aws.String("gp2"),
						"size":       aws.String("0"),
					},
					Labels: map[string]*string{
						"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
						"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
						"foo":                  aws.String("bar"),
					},
				},
			},
			{
				Name: aws.String("var-lib-juju"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: map[string]*string{
						"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
						"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
					},
				},
			},
			{
				Name: aws.String("charm-data-bin"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: map[string]*string{
						"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
						"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
					},
				},
			},
			{
				Name: aws.String("charm-data-container-gitlab"),
				DockerVolumeConfiguration: &ecs.DockerVolumeConfiguration{
					Scope:  aws.String("task"),
					Driver: aws.String("local"),
					Labels: map[string]*string{
						"juju-model-uuid":      aws.String("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
						"juju-controller-uuid": aws.String("deadbeef-1bad-500d-9000-4b1d0d06f00d"),
					},
				},
			},
		},
	}
	gomock.InOrder(
		append(
			[]*gomock.Call{
				s.ecsClient.EXPECT().RegisterTaskDefinition(registerTaskDefinitionInput).Return(&ecs.RegisterTaskDefinitionOutput{
					TaskDefinition: &ecs.TaskDefinition{
						Family:   aws.String(s.appName),
						Revision: aws.Int64(1),
					},
				}, nil),
			}, assertCalls...,
		)...,
	)

	c.Assert(app.Ensure(
		caas.ApplicationConfig{
			AgentImagePath: "operator/image-path",
			CharmBaseImage: coreresources.DockerImageDetails{
				RegistryPath: "ubuntu:20.04",
			},
			CharmModifiedVersion: 9001,
			Filesystems: []storage.KubernetesFilesystemParams{
				{
					StorageName: "database",
					Size:        100,
					Provider:    "kubernetes",
					Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
					Attachment: &storage.KubernetesFilesystemAttachmentParams{
						Path: "path/to/here",
					},
					ResourceTags: map[string]string{"foo": "bar"},
				},
			},
			Containers: map[string]caas.ContainerConfig{
				"gitlab": {
					Name: "gitlab",
					Image: coreresources.DockerImageDetails{
						RegistryPath: "gitlab-image:latest",
					},
					Mounts: []caas.MountConfig{
						{
							StorageName: "database",
							Path:        "path/to/here",
						},
					},
				},
			},
		},
	), jc.ErrorIsNil)
}

func (s *applicationSuite) TestEnsureDeploymentStatelessCreate(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless)
	defer ctrl.Finish()
	s.assertEnsure(c, app,
		s.ecsClient.EXPECT().UpdateService(&ecs.UpdateServiceInput{
			Cluster:        aws.String("test-cluster"),
			DesiredCount:   aws.Int64(1),
			Service:        aws.String("test-gitlab"),
			TaskDefinition: aws.String("gitlab:1"),
		}).Return(nil, &ecs.ServiceNotFoundException{}),
		s.ecsClient.EXPECT().CreateService(&ecs.CreateServiceInput{
			Cluster:        aws.String("test-cluster"),
			DesiredCount:   aws.Int64(1),
			ServiceName:    aws.String("test-gitlab"),
			TaskDefinition: aws.String("gitlab:1"),
		}).Return(nil, nil),
	)
}

func (s *applicationSuite) TestEnsureDeploymentStatelessUpdate(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless)
	defer ctrl.Finish()
	s.assertEnsure(c, app,
		s.ecsClient.EXPECT().UpdateService(&ecs.UpdateServiceInput{
			Cluster:        aws.String("test-cluster"),
			DesiredCount:   aws.Int64(1),
			Service:        aws.String("test-gitlab"),
			TaskDefinition: aws.String("gitlab:1"),
		}).Return(nil, nil),
	)
}

func (s *applicationSuite) TestDelete(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless)
	defer ctrl.Finish()

	gomock.InOrder(
		s.ecsClient.EXPECT().DeleteService(&ecs.DeleteServiceInput{
			Cluster: aws.String("test-cluster"),
			Service: aws.String("test-gitlab"),
			Force:   aws.Bool(true),
		}).Return(nil, nil),

		s.ecsClient.EXPECT().ListTaskDefinitionsWithContext(gomock.Any(), &ecs.ListTaskDefinitionsInput{
			FamilyPrefix: aws.String("test-gitlab"),
		}).Return(&ecs.ListTaskDefinitionsOutput{
			TaskDefinitionArns: []*string{
				aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:1"),
				aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:2"),
				aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:3"),
			},
		}, nil),

		s.ecsClient.EXPECT().DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:1"),
		}).Return(nil, nil),
		s.ecsClient.EXPECT().DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:2"),
		}).Return(nil, nil),
		s.ecsClient.EXPECT().DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String("arn:aws:ecs:us-east-1:012345678910:task-definition/test-gitlab:3"),
		}).Return(nil, nil),
	)

	c.Assert(app.Delete(), jc.ErrorIsNil)
}
