// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package provider_test

import (
	// "strings"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apps "k8s.io/api/apps/v1"
	k8sstorage "k8s.io/api/storage/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var (
	// TODO(bootstraping): fix me.
	sharedSecret = []byte(`
n7i/pelnObS6ukP/onkSjUtYIL0fBQdPPqH/ckQSK1ykVwneSQDQIw3SN4x0JP55dDmYfKGkq86joT4LbgdojTvDEx7Ki5WKUFBzolYwjQa2oL39nFzWHC41d8MgpUvDRX6xoZX2NZnGY5LlVLw3SPO7KtdLSmZ5MGcUwkIDB9I2nTEHbk3099LsR2SiUX/12pCWszukOmfcZGMFtlxPkjtC1i1O4FRyI8uWabDYm5kbNNzXpewuuFmkAAr4BlQjmZUhWzULSCF62DUKDaruL4I6+vtWldYi4E4jXHGppxSUehox/jG3d4vSdr6E/fpLMlyic4SibOnXoiPIn68/XwOguTKWHjIaBu615VPkiTlAUOVPHFG6ItyvmVjKSnpU5/aAwG9hIbObqcN6+9mTc2KpBaRqtFBpso/dT1edVzRyRki2zcBH1zopNXlVU4MYmNrMTXfGEJ6wmzq2F7AT50mmePhBbGvZFLkRraHGB+bdanhg5XffwvcmXUsIwMylT7m1O4qJlmuQYECWIbzJISmOjmiTAqL26FcAJ295lxv01V6V6x8bOTpMPxDKRUfoGGqId7pGWfhGKl8RvXsu3ofPmfiEA0gHQn4BEJ1f2GlXkLhPjb4Cm4t/NL6EBvOANXtWfGri4CsVA0WVp9N3eeFce0Io96CUn0vmQnmDHMZzjiHM/q+G8kr6SVcrdbgRvWd918MkaHOU/id4coBDlndJXKVB+bi17OEGEtEaSGV3I/f37rRotEd7JzKTjTzImsWMyAVB1mFgU5nIdnqCIWrPQSxxD9q+p4GoqSxzm9oH/wi9JS4qkgWwSaMG5LS1zVBdtULqxOFFWpbdNhCc4WCPDIyia4jOhnkQc+35jWYCTSoYCY6b/Er+uGdo/0+Z1exNoaSZeYdDEj5FkY2sGqWk+fkn7XD3ymzbPIC1Efs5BrTTr2w1X9RvVMvw4JgywwxEskB1UYGmyA+R9+F4kQ9hcTnwLT38r9za7sydbrU/BXr1Ww4yDXhCc1bsPsq3`[1:])

	serverPem = []byte(`
-----BEGIN CERTIFICATE-----
MIIDtDCCApygAwIBAgIUWVpWywFVInsZEFBprPbrHpFXDwIwDQYJKoZIhvcNAQEL
BQAwbjENMAsGA1UEChMEanVqdTEuMCwGA1UEAwwlanVqdS1nZW5lcmF0ZWQgQ0Eg
Zm9yIG1vZGVsICJqdWp1LWNhIjEtMCsGA1UEBRMkZjU5OWNlNDAtNjkyYS00NzAw
LTg2ZmYtYzkyN2E1ZTlhOTNmMB4XDTE4MDgyNzAyMTUzOFoXDTI4MDkwMzAyMTUz
OFowGzENMAsGA1UEChMEanVqdTEKMAgGA1UEAwwBKjCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBALbyAb+z/v8TuAA0IvJjpzpnld7gUyqFvgZ2FAzQjXmC
i4Kzyt9aN35NR5MEMPWFUFWkNN3ndaOOCqzOkhGY0p4RCXEKBzkF9tGsn6ksp6J5
fIq0tcqlZVqtupwGAnNa4gj4NsNPUUmFB5mgNQdadGCoIdB+oZ10xp9noMlcO7JU
t4unyBiVZKyX6CCB96EPQYRYHOqI5oD6cfYeYR3AALqI80TDUp6R+jAirzG5wy66
PlkABKOZncoqCZWWSYdgnJJn+0vjFIwpIG7MEfvtZY1FhT47NCGloOTgrz2K+9qX
CD6YYzO6xW8dvaC/sa4Vsao/n+8AOiLfG7Xqnrgv6xMCAwEAAaOBnDCBmTAOBgNV
HQ8BAf8EBAMCA6gwEwYDVR0lBAwwCgYIKwYBBQUHAwEwHQYDVR0OBBYEFN4dOffD
oTewv2tVoGHHmtjO6LNDMB8GA1UdIwQYMBaAFBGQY4mX+bE0wCpF2gTC23JxG8PB
MDIGA1UdEQQrMCmCCWxvY2FsaG9zdIIOanVqdS1hcGlzZXJ2ZXKCDGp1anUtbW9u
Z29kYjANBgkqhkiG9w0BAQsFAAOCAQEAfJu6/G9fh//qAmUv0reHQhd/jOKX9xPE
fDMNf2EmeznGfwikXtsNII9SyhnOTCK0Q307Fw4TgewJFnA3Sz75kCWq5G+dplgK
aK2NHLk/bwmvIZ6GEa3LwFwcIT6Ux8DsGdHIERXEpAdG3ylfPoLasjKb5FDNgNxX
po1cBBAPK0gZkrV3O9dVzrUkqLlzdsmt1Kqr3AazN6djNXX52FRzqMi6oRevkLOJ
KMNfwPKiDYBnAtJZOnAv+QsYqDKsFprtJsOmkxCUhErDY4Xm7P+aeWRgd1HaHK75
4Ctms2Uy/XA5961Eke6ifQ6ds/0bvVYmEEU8hm5HlDHt4lfyzs90Nw==
-----END CERTIFICATE-----

-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAtvIBv7P+/xO4ADQi8mOnOmeV3uBTKoW+BnYUDNCNeYKLgrPK
31o3fk1HkwQw9YVQVaQ03ed1o44KrM6SEZjSnhEJcQoHOQX20ayfqSynonl8irS1
yqVlWq26nAYCc1riCPg2w09RSYUHmaA1B1p0YKgh0H6hnXTGn2egyVw7slS3i6fI
GJVkrJfoIIH3oQ9BhFgc6ojmgPpx9h5hHcAAuojzRMNSnpH6MCKvMbnDLro+WQAE
o5mdyioJlZZJh2Cckmf7S+MUjCkgbswR++1ljUWFPjs0IaWg5OCvPYr72pcIPphj
M7rFbx29oL+xrhWxqj+f7wA6It8bteqeuC/rEwIDAQABAoIBACF+t6FAtFxBYPvw
j8FvS2vfEUqIKdHsQLlwHwWlnXF03FQm1OsF2okuXv9k0g3xxZ6YfPFv8lLqq7ut
6oJ8R3uXRPJEUsQ2+lSzVVwlB+AwfAPtSCd9Fsx+aF8unn4+Uoov397sg8aBK74N
3geloQ8dWWuR88cfXUpML90OHQPuPT21nqNVBxEYUaU0zIVVMxxTkwqD91vWSxUU
EOpNEH3Egt7JpEqT8ohsFcA4iUCF40doES+HbGFP5J8tZwdSCvWT/nRtJq7RRxK4
y+wxJV5OCfA2RWl27Oy+UstXqXWdJ+VxMX9Ri3DcQY+6YsvqvZck0QNz0bF/EV72
cK3J2TECgYEA7grcBrTmu1FztLL13wA5TXtFo9FxCwKa7siyzg+lKRFa+uDw8Ii8
b4J27WIFPIbjM9tDXjtowmsSPHhffH9uCXx6jm3d+GD94h6EGO705r7FCd/iNG5G
cz94PJ1AA2NKa7YD5T9nkHmmjkavQ+dezoyKmOfW9RdAOiR1AZwNjLsCgYEAxL8Y
8D4IbmIWoyYQrawrsIqPyaLaleyOFrOoVkN24vNiDpfpRicnNcyoXHET7TDfWDVs
wjyRoopVWrwudFjOXOcOIZv/BvZSm+kmZiMoYXYUmzzjxToNmxow7B2Ko4ZpqLP+
vf3ReSMhEUUHZJMHgHGRGIRb9XVtMcmeEp5qoYkCgYEAkyd/cV3vrSjjQHfJazw2
MGHeYTEektHfeXH0p1Igpcym06SvDeNZqg2a+5C27/3rAqmvcdeEIXwTX/KCBPK5
0X90PAxLRjqfeGOpAcjm+KZCJKKUshjh0GkSKVaEthNxdDinG9cgbL3natjjjDTB
9SoInBHmXskq2UakVoRkE/UCgYAwCsXJLCyc36DNd+cMsYT9l+gigXzErT3I91e8
sL6gDnQ8QgX5Vmgxr+bQo+AMxClVfb4v8+BQA11ySY9CY8kIUHdX56KvjYiAf78b
o6whmFbRzV2E9HcMD6owjcojwhec1U74D7mNzfEuKV/zxB9J0vFuPivCVUkzphrO
SxaYmQKBgQDFDr7iv1KxDRj+IzBAZrRRMIORrvNZYtVpnzGf2nPNsvK4Ei1Uf5+2
liAle2zQUVLIRX6RGm0xsmr0mz5gWaumi4eex3l7Yec1CFxri93SC1DlMfpdkwH6
FOsMQt6rKnDmZ2ytfKpf8wQwGxcBw0o7Df/ZujbWHx6O6UoVM3cpFA==
-----END RSA PRIVATE KEY-----
`[1:])
)

type bootstrapSuite struct {
	BaseSuite

	controllerCfg controller.Config
	pcfg          *podcfg.ControllerPodConfig

	controllerStackerGetter func() provider.ControllerStackerForTest
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.controllerCfg = testing.FakeControllerConfig()
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(s.controllerCfg, "bionic")
	c.Assert(err, jc.ErrorIsNil)

	pcfg.JujuVersion = jujuversion.Current
	pcfg.APIInfo = &api.Info{
		Password: "password",
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	}
	pcfg.Controller.MongoInfo = &mongo.MongoInfo{
		Password: "password", Info: mongo.Info{CACert: coretesting.CACert},
	}
	pcfg.Bootstrap.ControllerModelConfig = s.cfg
	pcfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
	pcfg.Bootstrap.HostedModelConfig = map[string]interface{}{
		"name": "hosted-model",
	}
	pcfg.Bootstrap.StateServingInfo = params.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.StateServingInfo = params.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	s.pcfg = pcfg
	s.controllerStackerGetter = func() provider.ControllerStackerForTest {
		controllerStacker, err := provider.NewcontrollerStackForTest("juju-controller-test", s.broker, s.pcfg)
		c.Assert(err, jc.ErrorIsNil)
		return controllerStacker
		// return controllerStacker.(provider.ControllerStackerForTest)
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	controllerStacker := s.controllerStackerGetter()

	sc := k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "default-storageclass",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
	}
	scs := &k8sstorage.StorageClassList{Items: []k8sstorage.StorageClass{sc}}

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: s.namespace}}
	svc := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-service",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{provider.LabelApplication: "juju-controller-test"},
			Type:     core.ServiceType("NodePort"), // TODO(caas): NodePort works for single node only like microk8s.
			Ports: []core.ServicePort{
				{
					Name:       "mongodb",
					TargetPort: intstr.FromInt(37017),
					Port:       37017,
					Protocol:   "TCP",
				},
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(17070),
					Port:       17070,
				},
			},
		},
	}

	emptySecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
	}
	secretWithSharedSecretAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": sharedSecret,
		},
	}
	secretWithServerPEMAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": sharedSecret,
			"server.pem":    serverPem,
		},
	}

	emptyConfigMap := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
	}
	bootstrapParamsContent, err := s.pcfg.Bootstrap.StateInitializationParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	configMapWithBootstrapParamsAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Data: map[string]string{
			"bootstrap-params": string(bootstrapParamsContent),
		},
	}
	configMapWithAgentConfAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Data: map[string]string{
			"bootstrap-params": string(bootstrapParamsContent),
			"agent.conf":       controllerStacker.GetAgentConfigContent(c),
		},
	}

	numberOfPods := int32(1)
	statefulSetSpec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-statefulset",
			Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
			Namespace: s.namespace,
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: "juju-controller-test-service",
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{provider.LabelApplication: "juju-controller-test"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    map[string]string{provider.LabelApplication: "juju-controller-test"},
					Namespace: s.namespace,
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
				},
			},
		},
	}

	gomock.InOrder(

		// create namespace.
		s.mockNamespaces.EXPECT().Create(ns).Times(1).
			Return(ns, nil),

		// ensure service
		s.mockServices.EXPECT().Get("juju-controller-test-service", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(svc).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(svc).Times(1).
			Return(svc, nil),

		// ensure shared-secret secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(emptySecret).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Update(secretWithSharedSecretAdded).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),

		// ensure server.pem secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),
		s.mockSecrets.EXPECT().Update(secretWithServerPEMAdded).AnyTimes().
			Return(secretWithServerPEMAdded, nil),

		// ensure bootstrap-params configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(emptyConfigMap).AnyTimes().
			Return(emptyConfigMap, nil),
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(emptyConfigMap, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithBootstrapParamsAdded).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),

		// ensure agent.conf configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithAgentConfAdded).AnyTimes().
			Return(configMapWithAgentConfAdded, nil),

		// find storageclass to use.
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
			Return(scs, nil),

		// ensure statefulset.
		s.mockStatefulSets.EXPECT().Create(statefulSetSpec).Times(1).
			Return(statefulSetSpec, nil),
	)
	c.Assert(controllerStacker.Deploy(), jc.ErrorIsNil)
}
