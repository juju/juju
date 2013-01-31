package environs

// BoilerPlateConfig returns the content which is written to a boilerplate environments.yaml file.
func BoilerPlateConfig() string {
	return `## This is the Juju config file, which you can use to specify multiple environments in which to deploy.
## By default we ship local (default), AWS, HP Cloud, OpenStack, and MAAS
## See http://juju.ubuntu.com/docs for more information

## An environment configuration must always specify at least the following information:
##
## - name (to identify the environment)
## - type (to specify the provider)
## - admin-secret (a "password" identifying an client with administrative-level access to system state)

## Values in <brackets> below need to be filled in by the user.

default: local
environments:

## https://juju.ubuntu.com/get-started/local/
  local:
    type: local
    admin-secret: <some made up number>
    # globally unique bucket name
    control-bucket: <juju-some-randomnumbers>
    # override if your workstation is running a different series to which you are deploying
    # default-series: precise
    data-dir: /home/<your username>/juju-data

## https://juju.ubuntu.com/get-started/amazon/
  amazon:
    type: ec2
    admin-secret: <some made up number>
    # globally unique S3 bucket name
    control-bucket: <juju-some-randomnumbers>
    # override if your workstation is running a different series to which you are deploying
    # default-series: precise
    ssl-hostname-verification: true
    # region defaults to us-east-1, override if required
    # region: us-east-1
    # Not required if env variable AWS_ACCESS_KEY_ID is set
    access-key: <secret>
    # Not required if env variable AWS_SECRET_ACCESS_KEY is set
    secret-key: <secret>

## https://juju.ubuntu.com/get-started/openstack/
  openstack:
    type: openstack
    admin-secret: <some made up number>
    # globally unique swift bucket name
    control-bucket: <juju-some-randomnumbers>
    # Not required if env variable OS_AUTH_URL is set
    auth-url: https://yourkeystoneurl:443/v2.0/
    # override if your workstation is running a different series to which you are deploying
    # default-series: precise
    ssl-hostname-verification: True
    default-image-id: <nova server id>
    # userpass authentication is currently supported
    # Not required if env variable OS_USERNAME is set
    username: <your username>
    # Not required if env variable OS_PASSWORD is set
    password: <secret>
    # Not required if env variable OS_TENANT_NAME is set, also known as project name
    tenant-name: <your tenant name>
    # Not required if env variable OS_REGION_NAME is set
    region: <your region>

## https://juju.ubuntu.com/get-started/hp-cloud/
  hpcloud:
    type: openstack
    admin-secret: <some made up number>
    # globally unique swift bucket name
    control-bucket: <juju-some-randomnumbers>
    # Not required if env variable OS_AUTH_URL is set
    auth-url: https://yourkeystoneurl:35357/v2.0/
    # override if your workstation is running a different series to which you are deploying
    # default-series: precise
    default-image-id: <nova server id>
    # userpass authentication is currently supported
    # Not required if env variable OS_USERNAME is set
    username: <your username>
    # Not required if env variable OS_PASSWORD is set
    password: <secret>
    # Not required if env variable OS_TENANT_NAME is set, also known as project name
    tenant-name: <your tenant name>
    # Not required if env variable OS_REGION_NAME is set
    region: <your region>
    # The following are only required for keypair authentication which is not yet supported.
    # access-key: <secret>
    # secret-key: <secret>

## https://maas.ubuntu.com/docs/juju-quick-start.html
  maas:
    type: maas
    admin-secret: nothing
    maas-server: http://localhost:5240
    maas-oauth: '${maas-api-key}'

`
}
