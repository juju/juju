// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8sstorage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/mongo"
)

const (
	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "juju-controller"

	portMongoDB             = 37017
	portAPIServer           = 17070
	fileNameSharedSecret    = "shared-secret"
	fileNameSSLKey          = "server.pem"
	fileNameBootstrapParams = "bootstrap-params"
	fileNameAgentConf       = "agent.conf"

	storageSizeControllerRaw = "20Gi" // TODO(caas): parse from constrains?
)

var (
	stackLabelsGetter                       = func(stackName string) map[string]string { return map[string]string{labelApplication: stackName} }
	resourceNameGetterStatefulSet           = func(stackName string) string { return stackName }
	resourceNameGetterService               = resourceNameGetter("service")
	resourceNameGetterVolumeSharedSecret    = resourceNameGetter(fileNameSharedSecret)
	resourceNameGetterVolumeSSLKey          = resourceNameGetter(fileNameSSLKey)
	resourceNameGetterVolumeBootstrapParams = resourceNameGetter(fileNameBootstrapParams)
	resourceNameGetterVolumeAgentConf       = resourceNameGetter(fileNameAgentConf)
	resourceNameGetterConfigMap             = resourceNameGetter("configmap")
	resourceNameGetterSecret                = resourceNameGetter("secret")
	pvcNameGetterLogDirStorage              = resourceNameGetter("jujud-log-storage")
	pvcNameGetterControllerPodStorage       = resourceNameGetter("juju-controller-storage")
)

func resourceNameGetter(name string) func(string) string {
	return func(stackName string) string {
		return stackName + "-" + strings.Replace(name, ".", "-", -1)
	}
}

func createControllerService(client bootstrapBroker, cleanUps *failHooks) error {
	svcName := resourceNameGetterService(JujuControllerStackName)
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      svcName,
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: client.GetCurrentNamespace(),
		},
		Spec: core.ServiceSpec{
			Selector: stackLabelsGetter(JujuControllerStackName),
			Type:     core.ServiceType("NodePort"), // TODO(caas): NodePort works for single node only like microk8s.
			Ports: []core.ServicePort{
				{
					Name:       "mongodb",
					TargetPort: intstr.FromInt(portMongoDB),
					Port:       portMongoDB,
					Protocol:   "TCP",
				},
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(portAPIServer),
					Port:       portAPIServer,
				},
			},
		},
	}
	logger.Debugf("ensuring controller service: \n%+v", spec)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q", svcName)
		client.deleteService(svcName)
	})
	return errors.Trace(client.ensureService(spec))
}

func getControllerSecret(broker bootstrapBroker) (secret *core.Secret, err error) {
	defer func() {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
	}()

	secretName := resourceNameGetterSecret(JujuControllerStackName)
	secret, err = broker.getSecret(secretName)
	if err == nil {
		return secret, nil
	}
	if errors.IsNotFound(err) {
		err = broker.createSecret(&core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretName,
				Labels:    stackLabelsGetter(JujuControllerStackName),
				Namespace: broker.GetCurrentNamespace(),
			},
			Type: core.SecretTypeOpaque,
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return broker.getSecret(secretName)
}

func createControllerSecretSharedSecret(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, cleanUps *failHooks) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSharedSecret] = []byte(si.SharedSecret)
	logger.Debugf("ensuring shared secret: \n%+v", secret)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q shared-secret", secret.Name)
		client.deleteSecret(secret.Name)
	})
	return client.ensureSecret(secret)
}

func createControllerSecretServerPem(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, cleanUps *failHooks) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	// secret.Data[fileNameSSLKey] = []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey))
	// TODO(bootstrap): remove me.
	secret.Data[fileNameSSLKey] = []byte(`
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

	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q server.pem", secret.Name)
		client.deleteSecret(secret.Name)
	})
	return client.ensureSecret(secret)
}

func createControllerSecretMongoAdmin(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, cleanUps *failHooks) error {
	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(caas): for mongo side car container, it's currently disabled.
	// secret.Data[mongoAdmin] = []byte("xxxx")
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q mongo admin secret", secret.Name)
		client.deleteSecret(secret.Name)
	})
	return nil
}

func getControllerConfigMap(broker bootstrapBroker) (cm *core.ConfigMap, err error) {
	defer func() {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
	}()

	cmName := resourceNameGetterConfigMap(JujuControllerStackName)
	cm, err = broker.getConfigMap(cmName)
	if err == nil {
		return cm, nil
	}
	if errors.IsNotFound(err) {
		err = broker.createConfigMap(&core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      cmName,
				Labels:    stackLabelsGetter(JujuControllerStackName),
				Namespace: broker.GetCurrentNamespace(),
			},
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return broker.getConfigMap(cmName)
}

func ensureControllerConfigmapBootstrapParams(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig, cleanUps *failHooks) error {
	bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

	cm, err := getControllerConfigMap(client)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameBootstrapParams] = string(bootstrapParamsFileContent)

	logger.Debugf("creating bootstrap-params configmap: \n%+v", cm)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q bootstrap-params", cm.Name)
		client.deleteConfigMap(cm.Name)
	})
	return client.ensureConfigMap(cm)
}

func ensureControllerConfigmapAgentConf(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, cleanUps *failHooks) error {
	agentConfigFileContent, err := agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	cm, err := getControllerConfigMap(client)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameAgentConf] = string(agentConfigFileContent)

	logger.Debugf("ensuring agent.conf configmap: \n%+v", cm)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q template-agent.conf", cm.Name)
		client.deleteConfigMap(cm.Name)
	})
	return client.ensureConfigMap(cm)
}

type bootstrapBroker interface {
	createConfigMap(configMap *core.ConfigMap) error
	getConfigMap(cmName string) (*core.ConfigMap, error)
	ensureConfigMap(configMap *core.ConfigMap) error
	deleteConfigMap(configMapName string) error

	createSecret(Secret *core.Secret) error
	getSecret(secretName string) (*core.Secret, error)
	ensureSecret(sec *core.Secret) error
	deleteSecret(secretName string) error

	ensureService(spec *core.Service) error
	deleteService(deploymentName string) error

	createStatefulSet(spec *apps.StatefulSet) error
	deleteStatefulSet(name string) error

	EnsureNamespace() error
	GetCurrentNamespace() string

	getDefaultStorageClass() (*k8sstorage.StorageClass, error)
}

type failHooks []func()

func createControllerStack(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig) error {
	// TODO(caas): we'll need a different tag type other than machine tag.
	var agentConfig agent.ConfigSetterWriter
	agentConfig, err := pcfg.AgentConfig(names.NewMachineTag(pcfg.MachineId))
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(bootstrap): remove me.
	agentConfig.SetMongoMemoryProfile(mongo.MemoryProfileDefault)
	agentConfig.SetMongoVersion(mongo.Mongo36wt)
	agentConfig.SetOldPassword("dbacffbe75cd8c70d81fe7738d9e8493")
	agentConfig.SetPassword("izREP7cxnryLX2gwEUe3zl40")

	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}

	// TODO(bootstrap): remove me.
	si.Cert = `
-----BEGIN CERTIFICATE-----
MIIDyzCCArOgAwIBAgIVAMaum/bXkVMByKDmsJZKQ4O23ElWMA0GCSqGSIb3DQEB
CwUAMG4xDTALBgNVBAoTBGp1anUxLjAsBgNVBAMMJWp1anUtZ2VuZXJhdGVkIENB
IGZvciBtb2RlbCAianVqdS1jYSIxLTArBgNVBAUTJGY1OTljZTQwLTY5MmEtNDcw
MC04NmZmLWM5MjdhNWU5YTkzZjAeFw0xODA4MjcwMjE1NDJaFw0yODA5MDMwMjE1
NDFaMBsxDTALBgNVBAoTBGp1anUxCjAIBgNVBAMMASowggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQCph6PF2WZD/lYNVDqa0iVBplpMfymNrMwpLgEIGVYx
KsNSMSPuUKhtNVJTRj6yesZDJS6cDwo6TSsBdDCZGcJuR+1H6FMyIAJpg1Pi2D+X
yCBh9v2QXJftqN7xCGoXQx50GEmHs5aN87U3VaPVE6Ezl2k/Bb5pPZYftNWvHD2e
ATQ4lG6bIMePYx55g3inNpzTZzM1oakX9BmBakOuBS0SD3fCbUHuo5OSGRByrJ8w
3oPMV+/s8npjcy9sYhprYXl5hEGonyEyl1yX5+DjEwhD2ZUaNrJ1ScviRhxrOVmE
/U4LoqNJFZCqoHtXAf+af8VLVULz4MxkdUxHxLGRU1gfAgMBAAGjgbIwga8wDgYD
VR0PAQH/BAQDAgOoMBMGA1UdJQQMMAoGCCsGAQUFBwMBMB0GA1UdDgQWBBQopMWN
ZZjJGgG1cZsU/jiKdAaV7DAfBgNVHSMEGDAWgBQRkGOJl/mxNMAqRdoEwttycRvD
wTBIBgNVHREEQTA/gghhbnl0aGluZ4IOanVqdS1hcGlzZXJ2ZXKCDGp1anUtbW9u
Z29kYoIJbG9jYWxob3N0hwQN7nT1hwSsHwbJMA0GCSqGSIb3DQEBCwUAA4IBAQA6
0n/7B4Yqzg5YpbB+yDOV5dbmdqj2Gi2/p0YTUtELTT5N7MDJbki/hjAN3YKiuCnO
fZBNvZVszzFUJgEYabqfCtNhZMTOTAcjwhcY+J9jNTZJEROccbSg/KvfFJTkRhjj
h3t6C4n4PPHQDhGBTTBUTdsc44GmEBSR0sqgykxquwOrSxVleqkw2dl0MV41MqaK
RuP2uoV/Px0rij/lNb+lCF697m6phruy95ZJdx4E9vZiSSrlOHONWR6yCaQ3Hvw3
BHIl0tbNZZqh3XIzTFv/VRecYz5tE/OsTptYkmc+glw3Zp5pWSOcGacb06Alm4Bj
YILHEY4tAouuw0cijCAP
-----END CERTIFICATE-----
`[1:]
	si.PrivateKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIEoQIBAAKCAQEAqYejxdlmQ/5WDVQ6mtIlQaZaTH8pjazMKS4BCBlWMSrDUjEj
7lCobTVSU0Y+snrGQyUunA8KOk0rAXQwmRnCbkftR+hTMiACaYNT4tg/l8ggYfb9
kFyX7aje8QhqF0MedBhJh7OWjfO1N1Wj1ROhM5dpPwW+aT2WH7TVrxw9ngE0OJRu
myDHj2MeeYN4pzac02czNaGpF/QZgWpDrgUtEg93wm1B7qOTkhkQcqyfMN6DzFfv
7PJ6Y3MvbGIaa2F5eYRBqJ8hMpdcl+fg4xMIQ9mVGjaydUnL4kYcazlZhP1OC6Kj
SRWQqqB7VwH/mn/FS1VC8+DMZHVMR8SxkVNYHwIDAQABAoIBAG9tnBO7JSCj12PD
bRG99ocEFG4bVvCsFzUp67urC6Adf2xSqE9H7Kx7U7Uwgp1FXXNcyRoCOLLBbfby
q861w7pAxJFy/tv/dhZsH4MGqCXXgJFjip6Mfb/UM1UyNqk7kJS2Mf5j6B09hmrs
e1beJCKI7sBhwhniRP5qGdmTMlzbU0J31UaZ72DJegC09qpe1GGKGpjx9FjHOVR6
+c9SfAHZ8WlBEgbc8cvVDKhs0XbXeMtLKoIKht5dLFyRtpFE3Eyl0K8WtliJHjWU
PgSt0i8myMIgPsLIjXNmKTOq8C8CmFFnCZcwYueFotrQqNUKcYQYD4cYn6ovL6fj
ph4lVAECgYEA3r7F9ijELsQ8pD9+QtqOFCILi82CGvQusYRgUu+GKXG4b5AWCfos
VrzyvqiqXl8y0RxJ/ltjXrG6SMxoenhtse59OOPf41scWKtqeQbaDQHcxHo7jtPf
yg2nFbicxibQxQhsXiTVfEaIibWbm6kjD0PGcOIWcDVbDvs17i3fPZ8CgYEAwtb/
d0qMY/TFF0xzIqSqQMG43OY00sz1Y6PSKGeDTA6zlJ9L6Euq17PByPkLcRP9JJAI
regPF0RxQoRQ5Y8vUaATrEpTPLSD48SiLuvFOAltqS6PI1PHbfITsfzHfCGQEoJr
YfChMwiXlcXkjNge44mm/7d0ydnc993hMUBw1YECgYAUhLRNoaG4wSDo7GRgGive
VOiFX0/t1bJ7bbtFyISuSqh3tmkhUCdHci5WO8k92j0fICD8ykRUE8EaNaImLfPE
4Tgtxmf4VIs+68NqFKR/cD1659uWo5PI1AshKBlg83Blxgndfj0gLosjTFRiOWle
XZrpCRqSCYgy0Bc+soEO9wJ/LCSJvH0nUX4jKSQo8bBc4k8BkwqU7S2CaxCyjHTn
SlQKW6G7kOWTz0rqnJ3P+c6Ni7sWPFBXGu5muqs/qMLH9bZOvroYIajEONZT6E2C
YS/BqJLj2x0gEcjGrYyXpYf1HDxwF8BsxSMtNMGhBkfwt5x4OXdW/mRdq8qZOo9f
AQKBgQCJlpkSmjoFxoabNKUWNmpamXqc01XhmhZMUQJWvvoSbOl/OqeQvb1NkEWf
ntiELCYCLE3GL4ytEt3C1mJTY3EmcnN/Jf7HXcskTsA1hqaB3S37MVT1Lss//nnF
Ywhm2dDF4r/Rf2yCJ2mipjkgOumk8lCh8PLlY7TiDzkHWlI3qw==
-----END RSA PRIVATE KEY-----
`[1:]

	// ensures shared-secret content.
	if si.SharedSecret == "" {
		// Generate a shared secret for the Mongo replica set.
		sharedSecret, err := mongo.GenerateSharedSecret()
		if err != nil {
			return errors.Trace(err)
		}
		si.SharedSecret = sharedSecret
	}
	// TODO(bootstrap): remove me.
	si.SharedSecret = `
n7i/pelnObS6ukP/onkSjUtYIL0fBQdPPqH/ckQSK1ykVwneSQDQIw3SN4x0JP55dDmYfKGkq86joT4LbgdojTvDEx7Ki5WKUFBzolYwjQa2oL39nFzWHC41d8MgpUvDRX6xoZX2NZnGY5LlVLw3SPO7KtdLSmZ5MGcUwkIDB9I2nTEHbk3099LsR2SiUX/12pCWszukOmfcZGMFtlxPkjtC1i1O4FRyI8uWabDYm5kbNNzXpewuuFmkAAr4BlQjmZUhWzULSCF62DUKDaruL4I6+vtWldYi4E4jXHGppxSUehox/jG3d4vSdr6E/fpLMlyic4SibOnXoiPIn68/XwOguTKWHjIaBu615VPkiTlAUOVPHFG6ItyvmVjKSnpU5/aAwG9hIbObqcN6+9mTc2KpBaRqtFBpso/dT1edVzRyRki2zcBH1zopNXlVU4MYmNrMTXfGEJ6wmzq2F7AT50mmePhBbGvZFLkRraHGB+bdanhg5XffwvcmXUsIwMylT7m1O4qJlmuQYECWIbzJISmOjmiTAqL26FcAJ295lxv01V6V6x8bOTpMPxDKRUfoGGqId7pGWfhGKl8RvXsu3ofPmfiEA0gHQn4BEJ1f2GlXkLhPjb4Cm4t/NL6EBvOANXtWfGri4CsVA0WVp9N3eeFce0Io96CUn0vmQnmDHMZzjiHM/q+G8kr6SVcrdbgRvWd918MkaHOU/id4coBDlndJXKVB+bi17OEGEtEaSGV3I/f37rRotEd7JzKTjTzImsWMyAVB1mFgU5nIdnqCIWrPQSxxD9q+p4GoqSxzm9oH/wi9JS4qkgWwSaMG5LS1zVBdtULqxOFFWpbdNhCc4WCPDIyia4jOhnkQc+35jWYCTSoYCY6b/Er+uGdo/0+Z1exNoaSZeYdDEj5FkY2sGqWk+fkn7XD3ymzbPIC1Efs5BrTTr2w1X9RvVMvw4JgywwxEskB1UYGmyA+R9+F4kQ9hcTnwLT38r9za7sydbrU/BXr1Ww4yDXhCc1bsPsq3`[1:]

	agentConfig.SetStateServingInfo(si)
	pcfg.Bootstrap.StateServingInfo = si

	// ensuring namespace for controller stack, this namespace will be removed by broker.DestroyController if bootstrap failed.
	if err := client.EnsureNamespace(); err != nil {
		return errors.Annotate(err, "ensuring namespace for controller stack")
	}

	cleanups := &failHooks{}
	defer func() {
		if err == nil {
			return
		}
		logger.Debugf("bootstrap failed, cleaning up %d resources have already created.", len(*cleanups))
		for _, f := range *cleanups {
			f()
		}
	}()
	// create service for controller pod.
	if err = createControllerService(client, cleanups); err != nil {
		return errors.Annotate(err, "creating service for controller")
	}

	// create shared-secret secret for controller pod.
	if err = createControllerSecretSharedSecret(client, agentConfig, cleanups); err != nil {
		return errors.Annotate(err, "creating shared-secret secret for controller")
	}

	// create server.pem secret for controller pod.
	if err = createControllerSecretServerPem(client, agentConfig, cleanups); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}

	// create mongo admin account secret for controller pod.
	if err = createControllerSecretMongoAdmin(client, agentConfig, cleanups); err != nil {
		return errors.Annotate(err, "creating mongo admin account secret for controller")
	}

	// create bootstrap-params configmap for controller pod.
	if err = ensureControllerConfigmapBootstrapParams(client, pcfg, cleanups); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err = ensureControllerConfigmapAgentConf(client, agentConfig, cleanups); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}

	// create statefulset to ensure controller stack.
	if err = createControllerStatefulset(client, pcfg, agentConfig, cleanups); err != nil {
		return errors.Annotate(err, "creating statefulset for controller")
	}

	return nil
}

func createControllerStatefulset(
	client bootstrapBroker,
	pcfg *podcfg.ControllerPodConfig,
	agentConfig agent.ConfigSetterWriter,
	cleanUps *failHooks,
) error {
	numberOfPods := int32(1) // TODO: HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterStatefulSet(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: client.GetCurrentNamespace(),
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: resourceNameGetterService(JujuControllerStackName),
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: stackLabelsGetter(JujuControllerStackName),
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    stackLabelsGetter(JujuControllerStackName),
					Namespace: client.GetCurrentNamespace(),
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
				},
			},
		},
	}

	storageclass, err := client.getDefaultStorageClass()
	if err != nil {
		return errors.Trace(err)
	}
	if err := buildStorageSpecForController(spec, storageclass.GetName()); err != nil {
		return errors.Trace(err)
	}

	if err := buildContainerSpecForController(spec, *pcfg, agentConfig); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("creating controller statefulset: \n%+v", spec)
	*cleanUps = append(*cleanUps, func() {
		logger.Debugf("deleting %q statefulset", spec.Name)
		client.deleteStatefulSet(spec.Name)
	})
	return errors.Trace(client.createStatefulSet(spec))
}

func buildStorageSpecForController(statefulset *apps.StatefulSet, storageClassName string) error {
	storageSizeController, err := resource.ParseQuantity(storageSizeControllerRaw)
	if err != nil {
		return errors.Trace(err)
	}

	// build persistent volume claim.
	statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:   pvcNameGetterControllerPodStorage(JujuControllerStackName),
				Labels: stackLabelsGetter(JujuControllerStackName),
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: storageSizeController,
					},
				},
			},
		},
	}

	fileMode := int32(256)
	var vols []core.Volume

	// add volume log dir.
	vols = append(vols, core.Volume{
		Name: pvcNameGetterLogDirStorage(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	})
	secretName := resourceNameGetterSecret(JujuControllerStackName)
	// add volume server.pem secret.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSSLKey(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  fileNameSSLKey,
						Path: fileNameSSLKey,
					},
				},
			},
		},
	})
	// add volume shared secret.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  fileNameSharedSecret,
						Path: fileNameSharedSecret,
					},
				},
			},
		},
	})
	cmName := resourceNameGetterConfigMap(JujuControllerStackName)
	// add volume agent.conf comfigmap.
	volAgentConf := core.Volume{
		Name: resourceNameGetterVolumeAgentConf(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  fileNameAgentConf,
						Path: "template" + "-" + fileNameAgentConf,
					},
				},
			},
		},
	}
	volAgentConf.VolumeSource.ConfigMap.Name = cmName
	vols = append(vols, volAgentConf)
	// add volume bootstrap-params comfigmap.
	volBootstrapParams := core.Volume{
		Name: resourceNameGetterVolumeBootstrapParams(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  fileNameBootstrapParams,
						Path: fileNameBootstrapParams,
					},
				},
			},
		},
	}
	volBootstrapParams.VolumeSource.ConfigMap.Name = cmName
	vols = append(vols, volBootstrapParams)

	statefulset.Spec.Template.Spec.Volumes = vols
	return nil
}

func buildContainerSpecForController(statefulset *apps.StatefulSet, pcfg podcfg.ControllerPodConfig, agentConfig agent.ConfigSetterWriter) error {
	probCmds := &core.ExecAction{
		Command: []string{
			"mongo",
			fmt.Sprintf("--port=%d", portMongoDB),
			"--ssl",
			"--sslAllowInvalidHostnames",
			"--sslAllowInvalidCertificates",
			fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	var containerSpec []core.Container
	// add container mongoDB.
	// TODO(caas): refactor mongo package to make it usable for IAAS and CAAS,
	// then generate mongo config from EnsureServerParams.
	containerSpec = append(containerSpec, core.Container{
		Name:            "mongodb",
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           "mongo:3.6.6",
		Command: []string{
			"mongod",
		},
		Args: []string{
			fmt.Sprintf("--dbpath=%s/db", pcfg.DataDir),
			fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
			"--sslPEMKeyPassword=ignored",
			"--sslMode=requireSSL",
			fmt.Sprintf("--port=%d", portMongoDB),
			"--journal",
			fmt.Sprintf("--replSet=%s", mongo.ReplicaSetName),
			"--quiet",
			"--oplogSize=1024",
			"--ipv6",
			"--auth",
			fmt.Sprintf("--keyFile=%s/shared-secret", pcfg.DataDir),
			"--storageEngine=wiredTiger",
			"--wiredTigerCacheSizeGB=0.25",
			"--bind_ip_all",
		},
		Ports: []core.ContainerPort{
			{
				Name:          "mongodb",
				ContainerPort: portMongoDB,
				Protocol:      "TCP",
			},
		},
		ReadinessProbe: &core.Probe{
			Handler: core.Handler{
				Exec: probCmds,
			},
			FailureThreshold:    3,
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
		},
		LivenessProbe: &core.Probe{
			Handler: core.Handler{
				Exec: probCmds,
			},
			FailureThreshold:    3,
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			TimeoutSeconds:      5,
		},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      pvcNameGetterLogDirStorage(JujuControllerStackName),
				MountPath: pcfg.LogDir,
			},
			{
				Name:      pvcNameGetterControllerPodStorage(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "db"),
				SubPath:   "db",
			},
			{
				Name:      resourceNameGetterVolumeSSLKey(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret),
				SubPath:   fileNameSharedSecret,
				ReadOnly:  true,
			},
		},
	})

	// add container API server.
	containerSpec = append(containerSpec, core.Container{
		Name: "api-server",
		// ImagePullPolicy: core.PullIfNotPresent,
		ImagePullPolicy: core.PullAlways, // TODO(bootstrap): for debug
		Image:           pcfg.GetControllerImagePath(),
		VolumeMounts: []core.VolumeMount{
			{
				Name:      pvcNameGetterControllerPodStorage(JujuControllerStackName),
				MountPath: pcfg.DataDir,
			},
			{
				Name:      pvcNameGetterLogDirStorage(JujuControllerStackName),
				MountPath: pcfg.LogDir,
			},
			{
				Name:      resourceNameGetterVolumeAgentConf(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "agents", ("machine-" + pcfg.MachineId), "template-agent.conf"),
				SubPath:   "template-agent.conf",
			},
			{
				Name:      resourceNameGetterVolumeSSLKey(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret),
				SubPath:   fileNameSharedSecret,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeBootstrapParams(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameBootstrapParams),
				SubPath:   fileNameBootstrapParams,
				ReadOnly:  true,
			},
		},
	})
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
