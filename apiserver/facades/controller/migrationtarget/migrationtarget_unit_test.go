// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	stdctx "context"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// caasSuite is specifically used for testing a caas model without mongo.
type caasSuite struct {
	state      *mocks.MockMigrationState
	context    *mocks.MockContext
	authorizer *mocks.MockAuthorizer
	resources  *mocks.MockResources
	presence   *mocks.MockPresence
}

var _ = gc.Suite(&caasSuite{})

func (s *caasSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = mocks.NewMockMigrationState(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.authorizer = mocks.NewMockAuthorizer(ctrl)
	s.resources = mocks.NewMockResources(ctrl)
	s.presence = mocks.NewMockPresence(ctrl)

	s.context.EXPECT().StatePool().Return(&state.StatePool{})
	s.context.EXPECT().Resources().Return(s.resources)
	s.context.EXPECT().Presence().Return(s.presence)

	s.authorizer.EXPECT().AuthClient().Return(true)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any())

	s.state.EXPECT().ControllerTag().Return(testing.ControllerTag)
	s.state.EXPECT().Cloud("k8s").Return(cloud.Cloud{Name: "k8s"}, nil)
	s.state.EXPECT().CloudCredential(gomock.Any()).Return(
		state.Credential{}, nil)
	s.state.EXPECT().Close()

	return ctrl
}

// TestImportPopulateStorageUniqueID tests that a model hosting an app
// with an empty storage unique ID will have its storage unique ID backfilled
// when we import the model. The storage unique ID will be fetched from annotations
// in the k8s resource.
func (s *caasSuite) TestImportPopulateStorageUniqueID(c *gc.C) {
	// Add a statefulset in k8s. The `app.juju.is/uuid` entry in annotation
	// will be used to backfill the missing storage unique ID.
	k8sClient := fake.NewSimpleClientset()
	_, err := k8sClient.AppsV1().StatefulSets("testmodel").
		Create(stdctx.Background(), &v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ubuntu",
				Annotations: map[string]string{
					"app.juju.is/uuid": "uniqueid",
				},
			},
		}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Create a model with a missing storage unique id.
	serializedModel := s.makeSerializedModel(c)

	// Create the migrationtarget API.
	api, err := migrationtarget.NewAPI(
		s.context, nil,
		nil, facades.FacadeVersions{},
		func(cloudSpec cloudspec.CloudSpec) (kubernetes.Interface, *rest.Config, error) {
			return k8sClient, nil, nil
		},
		// We assert the storage unique ID value in [importModel].
		importModel(c, s.state),
		nil,
		s.state,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Do the import.
	err = api.Import(params.SerializedModel{Bytes: serializedModel})
	c.Assert(err, jc.ErrorIsNil)
}

// makeSerializedModel creates a description model and returns it in a slice of
// bytes format ready for import.
// We populate the fields that allows it to be successfully deserialized.
func (s *caasSuite) makeSerializedModel(c *gc.C) []byte {
	model := description.NewModel(description.ModelArgs{
		AgentVersion:       "3.6.9",
		Type:               "caas",
		Owner:              names.NewUserTag("admin"),
		LatestToolsVersion: version.MustParse("3.6.9"),
		Cloud:              "k8s",
		Config: map[string]interface{}{
			"name": "testmodel",
		},
	})
	model.SetStatus(description.StatusArgs{})

	// Minimal arguments to add an application.
	// Be aware we are not setting a StorageUniqueID value. This is so we can
	// simulate a backfill.
	app := model.AddApplication(description.ApplicationArgs{
		Tag:                  names.NewApplicationTag("ubuntu"),
		Type:                 "caas",
		CharmURL:             "local:trusty/ubuntu",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		CharmConfig: map[string]interface{}{
			"key": "value",
		},
		Leader: "ubuntu/0",
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
	})
	app.SetStatus(description.StatusArgs{
		Value:   "running",
		Updated: time.Date(2025, 11, 16, 11, 50, 0, 0, time.UTC),
	})
	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner:    names.NewUserTag("test-admin"),
		Cloud:    names.NewCloudTag("dummy"),
		Name:     "kubernetes",
		AuthType: string(cloud.EmptyAuthType),
	})

	// For this test case we want to simulate an application with a missing storage
	// unique ID to be eventually backfilled, hence we assert here it's empty
	// as a pre-condition.
	c.Assert(model.Applications(), gc.HasLen, 1)
	c.Assert(model.Applications()[0].StorageUniqueID(), gc.Equals, "")

	bytes, err := description.Serialize(model)
	c.Assert(err, jc.ErrorIsNil)
	return bytes
}

// importModel performs the assertion that it receives a model s.t. the application
// storage unique ID has a non-empty value.
func importModel(c *gc.C, mockState *mocks.MockMigrationState) func(
	importer migration.StateImporter,
	getClaimer migration.ClaimerFunc,
	model description.Model,
) (*state.Model, migrationtarget.MigrationState, error) {
	return func(
		importer migration.StateImporter,
		getClaimer migration.ClaimerFunc,
		model description.Model,
	) (*state.Model, migrationtarget.MigrationState, error) {
		// Make sure that the storage unique ID for this application is indeed
		// backfilled. It was initially empty when exported in [makeSerializedModel].
		c.Assert(model.Applications()[0].StorageUniqueID(), gc.Equals, "uniqueid")
		return nil, mockState, nil
	}
}
