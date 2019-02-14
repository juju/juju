// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	// "github.com/juju/utils/ssh"
	"gopkg.in/juju/names.v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8sstorage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/podcfg"
	// config "github.com/juju/juju/environs/config"
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

	storageSizeControllerRaw = "20Gi" // TODO: parse from constrains?
)

var (
	stackLabelsGetter                       = func(stackName string) map[string]string { return map[string]string{labelApplication: stackName} }
	resourceNameGetterService               = func(stackName string) string { return stackName }
	resourceNameGetterStatefulSet           = resourceNameGetterService
	resourceNameGetterVolumeSharedSecret    = resourceNameGetter(fileNameSharedSecret)
	resourceNameGetterVolumeSSLKey          = resourceNameGetter(fileNameSSLKey)
	resourceNameGetterVolumeBootstrapParams = resourceNameGetter(fileNameBootstrapParams)
	resourceNameGetterVolumeAgentConf       = resourceNameGetter(fileNameAgentConf)
	resourceNameGetterVolumeSystemIdentity  = resourceNameGetter(agent.SystemIdentity)
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

func createControllerService(client bootstrapBroker) error {
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterService(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: client.GetCurrentNamespace(),
		},
		Spec: core.ServiceSpec{
			Selector: stackLabelsGetter(JujuControllerStackName),
			Type:     core.ServiceType("NodePort"), // TODO: NodePort works for single node only like microk8s.
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

func createControllerSecretSharedSecret(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}
	// if si.SharedSecret == "" {
	// 	// Generate a shared secret for the Mongo replica set, and write it out.
	// 	sharedSecret, err := mongo.GenerateSharedSecret()
	// 	if err != nil {
	// 		return errors.Trace(err)
	// 	}
	// 	si.SharedSecret = sharedSecret
	// 	agentConfig.SetStateServingInfo(si)
	// }

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSharedSecret] = []byte(si.SharedSecret)
	logger.Debugf("ensuring shared secret: \n%+v", secret)
	return client.ensureSecret(secret)
}

func createControllerSecretServerPem(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
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
	return client.ensureSecret(secret)
}

func createControllerSecretSystemIdentity(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, pcfg *podcfg.ControllerPodConfig) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "StateServingInfo is empty")
	}
	// privateKey, _, err := ssh.GenerateKey(config.JujuSystemKey)
	// // TODO: append this _ publickey into authorized-keys.
	// if err != nil {
	// 	return errors.Trace(err)
	// }
	// si.SystemIdentity = privateKey
	// agentConfig.SetStateServingInfo(si)

	// // TODO: should we set to `default` rather than low.?
	// mmprof, err := mongo.NewMemoryProfile(pcfg.Controller.Config.MongoMemoryProfile())
	// if err != nil {
	// 	logger.Errorf("could not set requested memory profile: %v", err)
	// } else {
	// 	agentConfig.SetMongoMemoryProfile(mmprof)
	// }

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[agent.SystemIdentity] = []byte(si.SystemIdentity)
	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	return client.ensureSecret(secret)
}

func createControllerSecretMongoAdmin(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	// TODO: for mongo side car container, it's currently disabled.
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

func ensureControllerConfigmapBootstrapParams(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig) error {
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
	return client.ensureConfigMap(cm)
}

func ensureControllerConfigmapAgentConf(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
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
	return client.ensureConfigMap(cm)
}

type bootstrapBroker interface {
	createConfigMap(configMap *core.ConfigMap) error
	getConfigMap(cmName string) (*core.ConfigMap, error)
	ensureConfigMap(configMap *core.ConfigMap) error

	createSecret(Secret *core.Secret) error
	getSecret(secretName string) (*core.Secret, error)
	ensureSecret(sec *core.Secret) error

	ensureService(spec *core.Service) error

	createStatefulSet(spec *apps.StatefulSet) error

	GetCurrentNamespace() string
	EnsureNamespace() error
	getDefaultStorageClass() (*k8sstorage.StorageClass, error)
}

func createControllerStack(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig) error {
	// TODO(caas): we'll need a different tag type other than machine tag.
	var agentConfig agent.ConfigSetterWriter
	agentConfig, err := pcfg.AgentConfig(names.NewMachineTag(pcfg.MachineId))
	if err != nil {
		return errors.Trace(err)
	}

	// TODO:
	agentConfig.SetMongoMemoryProfile(mongo.MemoryProfileDefault)
	agentConfig.SetMongoVersion(mongo.Mongo36wt)
	agentConfig.SetOldPassword("dbacffbe75cd8c70d81fe7738d9e8493")
	agentConfig.SetPassword("izREP7cxnryLX2gwEUe3zl40")
	// 	agentConfig.SetCACert(`
	// -----BEGIN CERTIFICATE-----
	// MIIDrDCCApSgAwIBAgIUcxEbwMv177lg2pTqqfkuBvE+6rEwDQYJKoZIhvcNAQEL
	// BQAwbjENMAsGA1UEChMEanVqdTEuMCwGA1UEAwwlanVqdS1nZW5lcmF0ZWQgQ0Eg
	// Zm9yIG1vZGVsICJqdWp1LWNhIjEtMCsGA1UEBRMkZjU5OWNlNDAtNjkyYS00NzAw
	// LTg2ZmYtYzkyN2E1ZTlhOTNmMB4XDTE4MDgyNzAyMTE1M1oXDTI4MDkwMzAyMTE1
	// MlowbjENMAsGA1UEChMEanVqdTEuMCwGA1UEAwwlanVqdS1nZW5lcmF0ZWQgQ0Eg
	// Zm9yIG1vZGVsICJqdWp1LWNhIjEtMCsGA1UEBRMkZjU5OWNlNDAtNjkyYS00NzAw
	// LTg2ZmYtYzkyN2E1ZTlhOTNmMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
	// AQEAr/wI4stwkPIaw+2EVQwHbqeGX4LQPaEJGbwubmAuQPKAIZGqKqWGXdY6+Ixd
	// GFLapXORWImw9w+XP7Fc562TXGVB8VNVGGUQRzfUs0FMvjKQ78CM3kxNA0r+mA6P
	// IDBU/mrRR2C9U2/PDDMf4zBRUUWbjQQfqGNEm5/F8eGOJDwOompXXnh1puXhucDZ
	// Of+xdaatlb9HKwwfK8INboPmkAL0JpF1LXVnOn8AMYnFFYohHBTC+3Wta2Rn4tNX
	// Qq6APCSfHYCopVlG0GsrDLeRLHYoreQmiyw1+YatS31MaGiGqnnkeVJPMa9x0Sy5
	// SdUoKSDYK3dYo/GzTilVbNyRdQIDAQABo0IwQDAOBgNVHQ8BAf8EBAMCAqQwDwYD
	// VR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUEZBjiZf5sTTAKkXaBMLbcnEbw8EwDQYJ
	// KoZIhvcNAQELBQADggEBAImvg8SmBeFf5u33U+LvfClY63YDwGCT+pzpzUrVI3iJ
	// UIqUD+c+7oGSkDbIYESepBqYzDPyhT7zlUJKPaQVwAGqz6BT7MNgImPuAuLFk0ur
	// up054Jr+8ozybqKFW89/eHahjdufrNRplHZfcn5cXqbjflxsntk7ptIhy2l3plet
	// iDELkvN8bvQ1L9RyFdKxaQnXGLMp8GJ6xPhEuooyWjmfQUZf2QB5+tUfeMfYaD6i
	// VR2mo3QJx3nC/Q0BRCfAjpRXxzlX6ws5ynx9H9mbEcCJVIwklkd8/A16A6K+g073
	// UyXxQANHHzmA8BaTB7d3p/nYH2xidgWNMtz6rmGrZX8=
	// -----END CERTIFICATE-----
	// `[1:])

	// TODO:
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}
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
	// 	si.CAPrivateKey = `
	// -----BEGIN RSA PRIVATE KEY-----
	// MIIEowIBAAKCAQEAr/wI4stwkPIaw+2EVQwHbqeGX4LQPaEJGbwubmAuQPKAIZGq
	// KqWGXdY6+IxdGFLapXORWImw9w+XP7Fc562TXGVB8VNVGGUQRzfUs0FMvjKQ78CM
	// 3kxNA0r+mA6PIDBU/mrRR2C9U2/PDDMf4zBRUUWbjQQfqGNEm5/F8eGOJDwOompX
	// Xnh1puXhucDZOf+xdaatlb9HKwwfK8INboPmkAL0JpF1LXVnOn8AMYnFFYohHBTC
	// +3Wta2Rn4tNXQq6APCSfHYCopVlG0GsrDLeRLHYoreQmiyw1+YatS31MaGiGqnnk
	// eVJPMa9x0Sy5SdUoKSDYK3dYo/GzTilVbNyRdQIDAQABAoIBAGH/uLcK0Ql2OJ9o
	// kauGglEFaxee0fWvylCRcU23s6opIF8RLbCH8nYoyTgFegYEhYti+spSCsDZ5sDq
	// NLEzAH+QR5Nqc1WdWd4+4excba7wm7NXB1r3JF+0EGh+mwcywvHWa+oSnftrpOHH
	// SneKPY5Dc+aoKDTt6pO6+lDC6ROVjXhKYmKmbvJyKh/1Lotm31ekwYZLvyOi5t9z
	// Kskmi6FeuAKDZcSMS8/nlBl4M13rFFQyQ+aq4qNwFngPD8B0QLz5BYJi86Mn+yrF
	// doCT4c6BMsXCdQt7T/7lVptWRpXpSwzIwMCS1wUy4bY5WDp/abJc8FqgrUKE2V/V
	// JSQjYaECgYEAy2X3evFO1hw8rFPhjrQCQcPx/OfeMgvO7dr1ntZfZZWa/Uo4sXL4
	// h6BVT99zLNqtAj3pTyi94rnAFUJnKOTTJagwkSC+tEZSe0w9+OfyRigDCTKrmRAE
	// OyWocCxrAHn/2otgh/kF06cPFu/RoK/+ACtbfgmT8vb1JTsPTtAkIg0CgYEA3X8h
	// jKQ1OKT6rD8X0Zf1aAQ5wANNRghV/GM2Nh/C/px9zFt2qoSmWwmqRkI4EA9++aAF
	// z6NymNalaSK67dQBNekl54mA6E/vBSEjoD7VntcAbmyy7olNXyD2wiuy3NpTplHf
	// X9hJV8YDX+NVZaMUzoOMzxOpjhmPBZXqFnwAGwkCgYBA3uaNeYTxWNQpCh+4ScUm
	// gH4fcTw2rflzdxA7dpe6aHqkKhXm0opdh09uSBAN0Di5rFFLA+178E5I+YK5UjHd
	// osTKpKzuBjesR2bEigWFRqGhP13nVWpkCuCr1h7Sahal9yn0dAHdvTxczmQHYdoa
	// 57koe5mKNiV9mFaLhmrfyQKBgQCQe6No2JyW7JdP0IA7CkLcrRT2ubCoZDuivRzZ
	// xXIvH+m3alpH9OuHKxDVb9CeOV18e/QOc/IG3M1dfXguN0Lq5cEB/eIGqE2kLO/O
	// Ue6LBHiVj3ZQv2OnEBumoVa1Vf2G2pU5Mh71kIcW/3XvLKgf5hPt6EeMGAQBgr8G
	// F7EB8QKBgGGhHmRO5QDJhL6fqnc6z0DL78O/hGGYPqN7pjCwgyvPWagMrZKENXY5
	// gd4hJmPxCBthMswdF9qKm8LRnb/3FrmgBTsbFoDs1qZpuvfkbqFGk/QlvOrAylWA
	// ICxEI9P1aFYRyHHMmFC/FQRWx/VniQQAhj854NHoNPy90zYkFd6c
	// -----END RSA PRIVATE KEY-----
	// `[1:]
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
	si.SystemIdentity = `
-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEA1Dm6Al9LAUtksxc+AIBiQjFnJpkLNq/3B+X5UQcaLzAtGvFG
SLBIYEEI/V7zweP2kpHFGTEPuQoRVzdseWAN0+JBgSoG2JUd87sIwBU7Y/c+KhMx
u8q+do6NpPqsCjV2QoqeYV1IxASf/KxqAigE51KWUl5oQ6kZD+rlFJ0D8Bx1EBw0
Xx9mRoGlLUKpsuDwqdOKgdJP6OgryCD1G8Jc1/lkdpTVhpSmd2iTbfqqmG0wPQq6
IvdqvEwjiSdrz+/4RPO4umEgMFDQlgiZYncwWc+CT/rbWwHNWWMVHFp0GFA2xrOB
zHakH3kB2yxwBOedCVRnyWYPSj5kXvUQm3hdmQIDAQABAoIBAHMtoTYIYbyiHlTU
GGJNSwaBqWnZRay4c2ll9plzMVLK4q/soihxA9a5dReNoN1pyzhgxIeXiOD0BdU/
zy9QYjDMaqCfHngM9eSBbY5R95mZZbOQFz3EGvpdA6K2KQihWz1h3fMZnZRErk+D
g0UIUyD4QX0Sn6OY8nEhGpLFZI265msb1y2lEJfPWmTxQA6ay6e/8oqUycpZ8K+x
gDz+w+ujO4sGIqRVtYTC7vnLLgCCFbNdkHan/tpflA/2sepqzXGiT3OB9SiXUPt1
10pd73FvJJV5KMN1v4fcL7LaIlyKiuHrxDST5doZkJpo+Gda6l5Kfh8AZQ0pk4pY
PG+piMECgYEA2TWMshgRGtWFeHFLzR8LHuSKrKjWCfvwI6xAAZl0VL/mAf8lllDt
kSy+oQPcNs+rYEpLgpTRgFrYAy0JeEWA8lcLpS8JcB4yGeQkcRXAmeZxZd3nDSEh
bFBORvsWhpnKVAssfwfXR1eguzDsLrf9/mIm8+Eey0V+UejbhdMcif8CgYEA+iBU
wtBabY0Vm/KU0yKrmN0KB84bkKulf3XNfV2w8nv7nCyRlNAjCgu5z+Pig1yfGsNH
E/DHcegWKte4mObuN9GxtpGNDztfXU+cZKRP6OjXNz6eAohtFSmWTjHHDpy3lMt8
mjNZCjm98U2X/Iw5/SIEmM4l1xhWWbeKdwiTKGcCgYA7gGjfbKpa4H0kplyufz+L
oe2/KK0hpQt+qjQKfCAbC0qV53BDgj3iFBDQiP8tYKxAv3l59wyBDeG41QCQGvIc
8O12vbDnLs5ou0+kTuIpBrCvyB8AQMAoLMOUvDnKe5yqczkoP1yg5YdZYCiDD9Ib
eoXTLytBYfMdux1Pxqo9vwKBgDnzY6//NfRLy8Xl3jVMwxUXoUtNpXVPT3jIgmOZ
YXXM4+67JL+luXiKXvKbic+FlhdNRxqHnq31Z61lbY9/cZHdM59o+ZWd2+pyl3l5
2EnOKI7UIyfTE/LjP7++KLBp/t6qhqPzYZ3M4wUVRTFuC8FqMEZ2/K1pJhiDPcF2
ayHhAoGAIaRxScWB7te3Y5sIqXrjQ0EBe7O+V1WwwgmGLEG95Rq1haafu6kDvLWD
v4+3QCEVEw2lKj8IHFb8QPq2rz6Zy+mAM8nVMtS/jf01rh+LN1K1K2xnDcFyQDhy
9YxZn4X4ZH1/4RJD+sQ6Raz1ln8Xtw1G9AR87qWdSdMU8K4n4Es=
-----END RSA PRIVATE KEY-----
`[1:]
	si.SharedSecret = `
n7i/pelnObS6ukP/onkSjUtYIL0fBQdPPqH/ckQSK1ykVwneSQDQIw3SN4x0JP55dDmYfKGkq86joT4LbgdojTvDEx7Ki5WKUFBzolYwjQa2oL39nFzWHC41d8MgpUvDRX6xoZX2NZnGY5LlVLw3SPO7KtdLSmZ5MGcUwkIDB9I2nTEHbk3099LsR2SiUX/12pCWszukOmfcZGMFtlxPkjtC1i1O4FRyI8uWabDYm5kbNNzXpewuuFmkAAr4BlQjmZUhWzULSCF62DUKDaruL4I6+vtWldYi4E4jXHGppxSUehox/jG3d4vSdr6E/fpLMlyic4SibOnXoiPIn68/XwOguTKWHjIaBu615VPkiTlAUOVPHFG6ItyvmVjKSnpU5/aAwG9hIbObqcN6+9mTc2KpBaRqtFBpso/dT1edVzRyRki2zcBH1zopNXlVU4MYmNrMTXfGEJ6wmzq2F7AT50mmePhBbGvZFLkRraHGB+bdanhg5XffwvcmXUsIwMylT7m1O4qJlmuQYECWIbzJISmOjmiTAqL26FcAJ295lxv01V6V6x8bOTpMPxDKRUfoGGqId7pGWfhGKl8RvXsu3ofPmfiEA0gHQn4BEJ1f2GlXkLhPjb4Cm4t/NL6EBvOANXtWfGri4CsVA0WVp9N3eeFce0Io96CUn0vmQnmDHMZzjiHM/q+G8kr6SVcrdbgRvWd918MkaHOU/id4coBDlndJXKVB+bi17OEGEtEaSGV3I/f37rRotEd7JzKTjTzImsWMyAVB1mFgU5nIdnqCIWrPQSxxD9q+p4GoqSxzm9oH/wi9JS4qkgWwSaMG5LS1zVBdtULqxOFFWpbdNhCc4WCPDIyia4jOhnkQc+35jWYCTSoYCY6b/Er+uGdo/0+Z1exNoaSZeYdDEj5FkY2sGqWk+fkn7XD3ymzbPIC1Efs5BrTTr2w1X9RvVMvw4JgywwxEskB1UYGmyA+R9+F4kQ9hcTnwLT38r9za7sydbrU/BXr1Ww4yDXhCc1bsPsq3`[1:]
	agentConfig.SetStateServingInfo(si)

	// pcfg.Bootstrap.StateServingInfo = si

	// ensuring namespace for controller stack.
	if err := client.EnsureNamespace(); err != nil {
		return errors.Annotate(err, "ensuring namespace for controller stack")
	}

	// create service for controller pod.
	if err := createControllerService(client); err != nil {
		return errors.Annotate(err, "creating service for controller")
	}

	// create shared-secret secret for controller pod.
	if err := createControllerSecretSharedSecret(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating shared-secret secret for controller")
	}

	// create server.pem secret for controller pod.
	if err := createControllerSecretServerPem(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}

	// create system-identity secret for controller pod.
	if err := createControllerSecretSystemIdentity(client, agentConfig, pcfg); err != nil {
		return errors.Annotate(err, "creating system-identity secret for controller")
	}

	// create mongo admin account secret for controller pod.
	if err := createControllerSecretMongoAdmin(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating mongo admin account secret for controller")
	}

	// create bootstrap-params configmap for controller pod.
	if err := ensureControllerConfigmapBootstrapParams(client, pcfg); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err := ensureControllerConfigmapAgentConf(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}

	// create statefulset to ensure controller stack.
	return errors.Annotate(
		createControllerStatefulset(client, pcfg, agentConfig),
		"creating statefulset for controller",
	)
}

func createControllerStatefulset(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig, agentConfig agent.ConfigSetterWriter) error {
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
			EmptyDir: &core.EmptyDirVolumeSource{}, // TODO: setup log dir.
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
	// add volume system-identity.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  agent.SystemIdentity,
						Path: agent.SystemIdentity,
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
		Image:           "mongo:3.6.6", // TODO:
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
			fmt.Sprintf("--replSet=%s", mongo.ReplicaSetName), // TODO
			"--quiet",
			"--oplogSize=1024", // TODO
			"--ipv6",
			"--auth",
			fmt.Sprintf("--keyFile=%s/shared-secret", pcfg.DataDir),
			"--storageEngine=wiredTiger",
			"--wiredTigerCacheSizeGB=0.25", // TODO
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
			// {
			// 	Name:      resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
			// 	MountPath: agentConfig.SystemIdentityPath(),
			// 	SubPath:   agent.SystemIdentity,
			// 	ReadOnly:  true,
			// },
		},
	})

	// add container API server.
	containerSpec = append(containerSpec, core.Container{
		Name: "api-server",
		// ImagePullPolicy: core.PullIfNotPresent,
		ImagePullPolicy: core.PullAlways, // TODO: for debug
		// Image:           pcfg.GetControllerImagePath(),
		Image: "ycliuhw/jujud-controller:2.5-beta1-bionic-amd64-2a3577c0b9",
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
			// {
			// 	Name:      resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
			// 	MountPath: agentConfig.SystemIdentityPath(),
			// 	SubPath:   agent.SystemIdentity,
			// 	ReadOnly:  true,
			// },
		},
	})
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
