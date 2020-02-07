package provider

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"

	"github.com/juju/juju/caas/kubernetes/provider/mocks"
)

type RBACMapperSuite struct {
	ctrl                *gomock.Controller
	k8sClient           *mocks.MockInterface
	mockCoreV1          *mocks.MockCoreV1Interface
	mockServiceAccounts *mocks.MockServiceAccountInterface
}

var _ = gc.Suite(&RBACMapperSuite{})

func (k *RBACMapperSuite) SetUpTest(c *gc.C) {
	k.ctrl = gomock.NewController(c)
	k.k8sClient = mocks.NewMockInterface(k.ctrl)
	k.mockCoreV1 = mocks.NewMockCoreV1Interface(k.ctrl)
	k.mockServiceAccounts = mocks.NewMockServiceAccountInterface(k.ctrl)
	k.mockCoreV1.EXPECT().ServiceAccounts(gomock.Any()).AnyTimes().Return(k.mockServiceAccounts)
	k.k8sClient.EXPECT().CoreV1().AnyTimes().Return(k.mockCoreV1)
}

func (k *RBACMapperSuite) TestMapping(c *gc.C) {
	sync := make(chan struct{}, 1)

	saList := corev1.ServiceAccountList{
		Items: []corev1.ServiceAccount{
			corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getRBACLabels2("test", "k8s-model", false),
					Name:   "test",
					UID:    types.UID("123"),
				},
			},
		},
	}

	saWatcher := watch.NewRaceFreeFake()
	gomock.InOrder(
		k.mockServiceAccounts.EXPECT().List(gomock.Any()).
			DoAndReturn(func(...interface{}) (*corev1.ServiceAccountList, error) {
				sync <- struct{}{}
				return &saList, nil
			}),
		k.mockServiceAccounts.EXPECT().Watch(gomock.Any()).Return(saWatcher, nil),
	)

	factory := informers.NewSharedInformerFactory(k.k8sClient, 0)
	rbacMapper := newRBACMapper(factory.Core().V1().ServiceAccounts())

	go rbacMapper.Wait()
	defer rbacMapper.Kill()
	<-sync

	_, err := rbacMapper.AppNameForServiceAccount(saList.Items[0].UID)
	c.Assert(err, jc.ErrorIsNil)
}
