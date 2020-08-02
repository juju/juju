package utils_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

type LabelSuite struct {
	client *fake.Clientset
}

var _ = gc.Suite(&LabelSuite{})

func (l *LabelSuite) SetUpTest(c *gc.C) {
	l.client = fake.NewSimpleClientset()
}

func (l *LabelSuite) TestIsLegacyModelLabels(c *gc.C) {
	tests := []struct {
		IsLegacy  bool
		Model     string
		Namespace *core.Namespace
	}{
		{
			IsLegacy: false,
			Model:    "legacy-model-label-test-1",
			Namespace: &core.Namespace{
				ObjectMeta: meta.ObjectMeta{
					Name:   "legacy-model-label-test-1",
					Labels: utils.LabelsForModel("legacy-model-label-test-1", false),
				},
			},
		},
	}

	for _, test := range tests {
		_, err := l.client.CoreV1().Namespaces().Create(context.TODO(), test.Namespace, meta.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)

		legacy, err := utils.IsLegacyModelLabels(test.Model, l.client.CoreV1().Namespaces())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(legacy, gc.Equals, test.IsLegacy)
	}
}
