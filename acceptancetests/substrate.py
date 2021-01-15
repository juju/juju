from contextlib import (
    contextmanager,
)
import json
import logging
import os
import subprocess
from time import sleep
try:
    import urlparse
except ImportError:
    import urllib.parse as urlparse
from boto import ec2
from boto.exception import EC2ResponseError

from dateutil import parser as date_parser

import gce
import six
from utility import (
    temp_dir,
    until_timeout,
)
import winazurearm


__metaclass__ = type


log = logging.getLogger("substrate")


LIBVIRT_DOMAIN_RUNNING = 'running'
LIBVIRT_DOMAIN_SHUT_OFF = 'shut off'


class StillProvisioning(Exception):
    """Attempted to terminate instances still provisioning."""

    def __init__(self, instance_ids):
        super(StillProvisioning, self).__init__(
            'Still provisioning: {}'.format(', '.join(instance_ids)))
        self.instance_ids = instance_ids


def translate_to_env(current_env):
    """Translate openstack settings to environment variables."""
    if current_env['type'] not in ('openstack', 'rackspace'):
        raise Exception('Not an openstack environment. (type: %s)' %
                        current_env['type'])
    # Region doesn't follow the mapping for other vars.
    new_environ = {'OS_REGION_NAME': current_env['region']}
    for key in ['username', 'password', 'tenant-name', 'auth-url']:
        new_environ['OS_' + key.upper().replace('-', '_')] = current_env[key]
    return new_environ


def get_euca_env(current_env):
    """Translate openstack settings to environment variables."""
    # Region doesn't follow the mapping for other vars.
    new_environ = {
        'EC2_URL': 'https://%s.ec2.amazonaws.com' % current_env['region']}
    for key in ['access-key', 'secret-key']:
        env_key = key.upper().replace('-', '_')
        new_environ['EC2_' + env_key] = current_env[key]
        new_environ['AWS_' + env_key] = current_env[key]
    return new_environ


def terminate_instances(env, instance_ids):
    if len(instance_ids) == 0:
        log.info("No instances to delete.")
        return
    provider_type = env.provider
    environ = dict(os.environ)
    if provider_type == 'ec2':
        environ.update(get_euca_env(env.make_config_copy()))
        command_args = ['euca-terminate-instances'] + instance_ids
    elif provider_type in ('openstack', 'rackspace'):
        environ.update(translate_to_env(env.make_config_copy()))
        command_args = ['nova', 'delete'] + instance_ids
    elif provider_type == 'maas':
        with maas_account_from_boot_config(env) as substrate:
            substrate.terminate_instances(instance_ids)
        return
    else:
        with make_substrate_manager(env) as substrate:
            if substrate is None:
                raise ValueError(
                    "This test does not support the %s provider"
                    % provider_type)
            return substrate.terminate_instances(instance_ids)
    log.info("Deleting %s." % ', '.join(instance_ids))
    subprocess.check_call(command_args, env=environ)


def attempt_terminate_instances(account, instance_ids):
    """Initiate terminate instance method of specific handler

    :param account: Substrate account object.
    :param instance_ids: List of instance_ids to terminate
    :return: List of instance_ids failed to terminate
    """
    uncleaned_instances = []
    for instance_id in instance_ids:
        try:
            # We are calling terminate instances for each instances
            # individually so as to catch any error.
            account.terminate_instances([instance_id])
        except Exception as e:
            # Using too broad exception here because terminate_instances method
            # is handlers specific
            uncleaned_instances.append((instance_id, repr(e)))
    return uncleaned_instances


def contains_only_known_instances(known_instance_ids, possibly_known_ids):
    """Identify instance_id_list only contains ids we know about.

    :param known_instance_ids: The list of instance_ids (superset)
    :param possibly_known_ids: The list of instance_ids (subset)
    :return: True if known_instance_ids only contains
    possibly_known_ids
    """
    return set(possibly_known_ids).issubset(set(known_instance_ids))


class AWSAccount:
    """Represent the credentials of an AWS account."""

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config, region=None):
        """Create an AWSAccount from a JujuData object."""
        config = get_config(boot_config)
        euca_environ = get_euca_env(config)
        if region is None:
            region = config["region"]
        client = ec2.connect_to_region(
            region, aws_access_key_id=euca_environ['EC2_ACCESS_KEY'],
            aws_secret_access_key=euca_environ['EC2_SECRET_KEY'])
        # There is no point constructing a AWSAccount if client is None.
        # It can't do anything.
        if client is None:
            log.info(
                'Failed to create ec2 client for region: {}.'.format(region))
            yield None
        else:
            yield cls(euca_environ, region, client)

    def __init__(self, euca_environ, region, client):
        self.euca_environ = euca_environ
        self.region = region
        self.client = client

    def iter_security_groups(self):
        """Iterate through security groups created by juju in this account.

        :return: an iterator of (group-id, group-name) tuples.
        """
        groups = self.client.get_all_security_groups(
            filters={'description': 'juju group'})
        for group in groups:
            yield group.id, group.name

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an iterator of (group-id, group-name) tuples.
        """
        log.info('Listing security groups in use.')
        reservations = self.client.get_all_instances(instance_ids=instance_ids)
        for reservation in reservations:
            for instance in reservation.instances:
                for group in instance.groups:
                    yield group.id, group.name

    def destroy_security_groups(self, groups):
        """Destroy the specified security groups.

        :return: a list of groups that could not be destroyed.
        """
        failures = []
        for group in groups:
            deleted = self.client.delete_security_group(name=group)
            if not deleted:
                failures.append(group)
        return failures

    def delete_detached_interfaces(self, security_groups):
        """Delete detached network interfaces for supplied groups.

        :param security_groups: A collection of security_group ids.
        :return: A collection of security groups which still have interfaces in
            them.
        """
        interfaces = self.client.get_all_network_interfaces(
            filters={'status': 'available'})
        unclean = set()
        for interface in interfaces:
            for group in interface.groups:
                if group.id in security_groups:
                    try:
                        interface.delete()
                    except EC2ResponseError as e:
                        err_code = six.ensure_text(e.error_code)
                        if err_code not in (
                                'InvalidNetworkInterface.InUse',
                                'InvalidNetworkInterfaceID.NotFound'):
                            raise
                        log.info(
                            'Failed to delete interface {!r}. {}'.format(
                                interface.id, e.message))
                        unclean.update(g.id for g in interface.groups)
                    break
        return unclean

    def cleanup_security_groups(self, instances, secgroups):
        """Destroy any security groups used only by `instances`.

        :param instances: The list of instance_ids
        :param secgroups: dict of security groups
        :return: list of failed deleted security groups
        """
        failures = []
        for sg_id, sg_instances in secgroups:
            if contains_only_known_instances(instances, sg_instances):
                try:
                    deleted = self.client.delete_security_group(name=sg_id)
                    if not deleted:
                        failures.append((sg_id, "Failed to delete"))
                except EC2ResponseError as e:
                    err_code = six.ensure_text(e.error_code)
                    if err_code != 'InvalidGroup.NotFound':
                        failures.append((sg_id, repr(e)))

        return failures

    def get_security_groups(self, instances):
        """Get AWS configured security group
        If instances list is specified then get security groups mapped
        to those instances only.

        :param instances: list of instance names
        :return: list containing tuples; where each tuples contains security
        group id as first element and the list of instances mapped to that
        security group as second element. [(sg_id, [i_id, id2]),
         (sg_id2, [i_id1])]
        """
        group_ids = [sg[0] for sg in self.iter_instance_security_groups(
            instances)]
        all_groups = self.client.get_all_security_groups(
            group_ids=group_ids)
        secgroups = [(sg.id, [id for id in sg.instances()])
                     for sg in all_groups]
        return secgroups

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        return self.client.terminate_instances(instance_ids=instance_ids)

    def ensure_cleanup(self, resource_details):
        """
        Do AWS specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resources = []

        if not resource_details:
            return uncleaned_resources

        security_groups = self.get_security_groups(
            resource_details.get('instances', []))

        uncleaned_instances = attempt_terminate_instances(
            self, resource_details.get('instances', []))

        uncleaned_security_groups = self.cleanup_security_groups(
            resource_details.get('instances', []), security_groups)

        if uncleaned_instances:
            uncleaned_resources.append(
                {'resource': 'instances',
                 'errors': uncleaned_instances})
        if uncleaned_security_groups:
            uncleaned_resources.append(
                {'resource': 'security groups',
                 'errors': uncleaned_security_groups})
        return uncleaned_resources


class OpenStackAccount:
    """Represent the credentials/region of an OpenStack account."""

    def __init__(self, username, password, tenant_name, auth_url, region_name):
        self._username = username
        self._password = password
        self._tenant_name = tenant_name
        self._auth_url = auth_url
        self._region_name = region_name
        self._client = None

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config):
        """Create an OpenStackAccount from a JujuData object."""
        config = get_config(boot_config)
        yield cls(
            config['username'], config['password'], config['tenant-name'],
            config['auth-url'], config['region'])

    def get_client(self):
        """Return a novaclient Client for this account."""
        from novaclient import client
        return client.Client(
            '1.1', self._username, self._password, self._tenant_name,
            self._auth_url, region_name=self._region_name,
            service_type='compute', insecure=False)

    @property
    def client(self):
        """A novaclient Client for this account.  May come from cache."""
        if self._client is None:
            self._client = self.get_client()
        return self._client

    def iter_security_groups(self):
        """Iterate through security groups created by juju in this account.

        :return: an iterator of (group-id, group-name) tuples.
        """
        return ((g.id, g.name) for g in self.client.security_groups.list()
                if g.description == 'juju group')

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an iterator of (group-id, group-name) tuples.
        """
        group_names = set()
        for server in self.client.servers.list():
            if instance_ids is not None and server.id not in instance_ids:
                continue
            # A server that errors before security groups are assigned will
            # have no security_groups attribute.
            groups = (getattr(server, 'security_groups', []))
            group_names.update(group['name'] for group in groups)
        return ((k, v) for k, v in self.iter_security_groups()
                if v in group_names)

    def ensure_cleanup(self, resource_details):
        """
        Do OpenStack specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


def convert_to_azure_ids(client, instance_ids):
    """Return a list of ARM ids from a list juju machine instance-ids.

    The Juju 2 machine instance-id is not an ARM VM id, it is the non-unique
    machine name. For any juju controller, there are 2 or more machines named
    0. Using the client, the machine ids machine names can be found.

    See: https://bugs.launchpad.net/juju-core/+bug/1586089

    :param client: A ModelClient instance.
    :param instance_ids: a list of Juju machine instance-ids
    :return: A list of ARM VM instance ids.
    """
    with AzureARMAccount.from_boot_config(
            client.env) as substrate:
        return substrate.convert_to_azure_ids(client, instance_ids)


class GCEAccount:
    """Represent an Google Compute Engine Account."""

    def __init__(self, client):
        """Constructor.

        :param client: An instance of apache libcloud GCEClient retrieved
            via gce.get_client.
        """
        self.client = client

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config):
        """A context manager for a GCE account.

        This creates a temporary cert file from the private-key.
        """
        config = get_config(boot_config)
        with temp_dir() as cert_dir:
            cert_file = os.path.join(cert_dir, 'gce.pem')
            open(cert_file, 'w').write(config['private-key'])
            client = gce.get_client(
                config['client-email'], cert_file,
                config['project-id'])
            yield cls(client)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance_id in instance_ids:
            # Pass old_age=0 to mean delete now.
            count = gce.delete_instances(self.client, instance_id, old_age=0)
            if count != 1:
                raise Exception('Failed to delete {}: deleted {}'.format(
                    instance_id, count))

    def ensure_cleanup(self, resource_details):
        """
        Do GCE specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


class AzureARMAccount:
    """Represent an Azure ARM Account."""

    def __init__(self, arm_client):
        """Constructor.

        :param arm_client: An instance of winazurearm.ARMClient.
        """
        self.arm_client = arm_client

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config):
        """A context manager for a Azure RM account.

        In the case of the Juju 1x, the ARM keys must be in the boot_config's
        config.  subscription_id is the same. The PEM for the SMS is ignored.
        """
        credentials = boot_config.get_cloud_credentials()
        # The tenant-id is required by Azure storage, but forbidden to be in
        # Juju credentials, so we get it from the bootstrap model options.  It
        # is suppressed when actually bootstrapping.  (See
        # ModelClient.make_model_config)
        tenant_id = boot_config.get_option('tenant-id')
        arm_client = winazurearm.ARMClient(
            credentials['subscription-id'], credentials['application-id'],
            credentials['application-password'], tenant_id)
        arm_client.init_services()
        yield cls(arm_client)

    def convert_to_azure_ids(self, client, instance_ids):
        if not instance_ids[0].startswith('machine'):
            log.info('Bug Lp 1586089 is fixed in {}.'.format(client.version))
            log.info('AzureARMAccount.convert_to_azure_ids can be deleted.')
            return instance_ids

        models = client.get_models()['models']
        # 2.2-rc1 introduced new model listing output name/short-name.
        model = [
            m for m in models
            if m.get('short-name', m['name']) == client.model_name][0]
        resource_group = 'juju-{}-model-{}'.format(
            model.get('short-name', model['name']), model['model-uuid'])
        # NOTE(achilleasa): resources was not used in this func
        # resources = winazurearm.list_resources(
        #    self.arm_client, glob=resource_group, recursive=True)
        vm_ids = []
        for machine_name in instance_ids:
            rgd, vm = winazurearm.find_vm_deployment(
                resource_group, machine_name)
            vm_ids.append(vm.vm_id)
        return vm_ids

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance_id in instance_ids:
            winazurearm.delete_instance(
                self.arm_client, instance_id, resource_group=None)

    def ensure_cleanup(self, resource_details):
        """
        Do AzureARM specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


class AzureAccount:
    """Represent an Azure Account."""

    def __init__(self, service_client):
        """Constructor.

        :param service_client: An instance of
            azure.servicemanagement.ServiceManagementService.
        """
        self.service_client = service_client

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config):
        """A context manager for a AzureAccount.

        It writes the certificate to a temp file because the Azure client
        library requires it, then deletes the temp file when done.
        """
        from azure.servicemanagement import ServiceManagementService
        config = get_config(boot_config)
        with temp_dir() as cert_dir:
            cert_file = os.path.join(cert_dir, 'azure.pem')
            open(cert_file, 'w').write(config['management-certificate'])
            service_client = ServiceManagementService(
                config['management-subscription-id'], cert_file)
            yield cls(service_client)

    @staticmethod
    def convert_instance_ids(instance_ids):
        """Convert juju instance ids into Azure service/role names.

        Return a dict mapping service name to role names.
        """
        services = {}
        for instance_id in instance_ids:
            service, role = instance_id.rsplit('-', 1)
            services.setdefault(service, set()).add(role)
        return services

    @contextmanager
    def terminate_instances_cxt(self, instance_ids):
        """Terminate instances in a context.

        This context manager requests termination, then allows the "with"
        block to happen.  When the block is exited, it waits until the
        operations complete.

        The strategy for terminating instances varies depending on whether all
        roles are being terminated.  If all roles are being terminated, the
        deployment and hosted service are deleted.  If not all roles are being
        terminated, the roles themselves are deleted.
        """
        converted = self.convert_instance_ids(instance_ids)
        requests = set()
        services_to_delete = set(converted.keys())
        for service, roles in converted.items():
            properties = self.service_client.get_hosted_service_properties(
                service, embed_detail=True)
            for deployment in properties.deployments:
                role_names = set(
                    d_role.role_name for d_role in deployment.role_list)
                if role_names.difference(roles) == set():
                    requests.add(self.service_client.delete_deployment(
                        service, deployment.name))
                else:
                    services_to_delete.discard(service)
                    for role in roles:
                        requests.add(
                            self.service_client.delete_role(
                                service, deployment.name, role))
        yield
        self.block_on_requests(requests)
        for service in services_to_delete:
            self.service_client.delete_hosted_service(service)

    def block_on_requests(self, requests):
        """Wait until the requests complete."""
        requests = set(requests)
        while len(requests) > 0:
            for request in list(requests):
                op = self.service_client.get_operation_status(
                    request.request_id)
                if op.status == 'Succeeded':
                    requests.remove(request)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances.

        See terminate_instances_cxt for details.
        """
        with self.terminate_instances_cxt(instance_ids):
            return

    def ensure_cleanup(self, resource_details):
        """
        Do Azure specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


class MAASAccount:
    """Represent a MAAS 2.0 account."""

    _API_PATH = 'api/2.0/'

    STATUS_READY = 4

    SUBNET_CONNECTION_MODES = frozenset(('AUTO', 'DHCP', 'STATIC', 'LINK_UP'))

    ACQUIRING = 'User acquiring node'

    CREATED = 'created'

    NODE = 'node'

    def __init__(self, profile, url, oauth):
        self.profile = profile
        self.url = urlparse.urljoin(url, self._API_PATH)
        self.oauth = oauth

    def _maas(self, *args):
        """Call maas api with given arguments and parse json result."""
        try:
            command = ('maas',) + args
            output = subprocess.check_output(command, stderr=subprocess.STDOUT)
            if not output:
                return None
            return json.loads(output)
        except subprocess.CalledProcessError as e:
            raise Exception('%s failed:\n %s' % (command, e.output))

    def login(self):
        """Login with the maas cli."""
        subprocess.check_call([
            'maas', 'login', self.profile, self.url, self.oauth])

    def logout(self):
        """Logout with the maas cli."""
        subprocess.check_call(['maas', 'logout', self.profile])

    def _machine_release_args(self, machine_id):
        return (self.profile, 'machine', 'release', machine_id)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance in instance_ids:
            maas_system_id = instance.split('/')[5]
            log.info('Deleting %s.' % instance)
            self._maas(*self._machine_release_args(maas_system_id))

    def _list_allocated_args(self):
        return (self.profile, 'machines', 'list-allocated')

    def get_allocated_nodes(self):
        """Return a dict of allocated nodes with the hostname as keys."""
        nodes = self._maas(*self._list_allocated_args())
        allocated = {node['hostname']: node for node in nodes}
        return allocated

    def get_acquire_date(self, node):
        events = self._maas(
            self.profile, 'events', 'query', 'id={}'.format(node))
        for event in events['events']:
            if node != event[self.NODE]:
                raise ValueError(
                    'Node "{}" was not "{}".'.format(event[self.NODE], node))
            if event['type'] == self.ACQUIRING:
                return date_parser.parse(event[self.CREATED])
        raise LookupError('Unable to find acquire date for "{}".'.format(node))

    def get_allocated_ips(self):
        """Return a dict of allocated ips with the hostname as keys.

        A maas node may have many ips. The method selects the first ip which
        is the address used for virsh access and ssh.
        """
        allocated = self.get_allocated_nodes()
        ips = {k: v['ip_addresses'][0] for k, v in allocated.items()
               if v['ip_addresses']}
        return ips

    def machines(self):
        """Return list of all machines."""
        return self._maas(self.profile, 'machines', 'read')

    def fabrics(self):
        """Return list of all fabrics."""
        return self._maas(self.profile, 'fabrics', 'read')

    def create_fabric(self, name, class_type=None):
        """Create a new fabric."""
        args = [self.profile, 'fabrics', 'create', 'name=' + name]
        if class_type is not None:
            args.append('class_type=' + class_type)
        return self._maas(*args)

    def delete_fabric(self, fabric_id):
        """Delete a fabric with given id."""
        return self._maas(self.profile, 'fabric', 'delete', str(fabric_id))

    def spaces(self):
        """Return list of all spaces."""
        return self._maas(self.profile, 'spaces', 'read')

    def create_space(self, name):
        """Create a new space with given name."""
        return self._maas(self.profile, 'spaces', 'create', 'name=' + name)

    def delete_space(self, space_id):
        """Delete a space with given id."""
        return self._maas(self.profile, 'space', 'delete', str(space_id))

    def create_vlan(self, fabric_id, vid, name=None):
        """Create a new vlan on fabric with given fabric_id."""
        args = [
            self.profile, 'vlans', 'create', str(fabric_id), 'vid=' + str(vid),
        ]
        if name is not None:
            args.append('name=' + name)
        return self._maas(*args)

    def delete_vlan(self, fabric_id, vid):
        """Delete a vlan on given fabric_id with vid."""
        return self._maas(
            self.profile, 'vlan', 'delete', str(fabric_id), str(vid))

    def interfaces(self, system_id):
        """Return list of interfaces belonging to node with given system_id."""
        return self._maas(self.profile, 'interfaces', 'read', system_id)

    def interface_update(self, system_id, interface_id, name=None,
                         mac_address=None, tags=None, vlan_id=None):
        """Update fields of existing interface on node with given system_id."""
        args = [
            self.profile, 'interface', 'update', system_id, str(interface_id),
        ]
        if name is not None:
            args.append('name=' + name)
        if mac_address is not None:
            args.append('mac_address=' + mac_address)
        if tags is not None:
            args.append('tags=' + tags)
        if vlan_id is not None:
            args.append('vlan=' + str(vlan_id))
        return self._maas(*args)

    def interface_create_vlan(self, system_id, parent, vlan_id):
        """Create a vlan interface on machine with given system_id."""
        args = [
            self.profile, 'interfaces', 'create-vlan', system_id,
            'parent=' + str(parent), 'vlan=' + str(vlan_id),
        ]
        # TODO(gz): Add support for optional parameters as needed.
        return self._maas(*args)

    def delete_interface(self, system_id, interface_id):
        """Delete interface on node with given system_id with interface_id."""
        return self._maas(
            self.profile, 'interface', 'delete', system_id, str(interface_id))

    def interface_link_subnet(self, system_id, interface_id, mode, subnet_id,
                              ip_address=None, default_gateway=False):
        """Link interface from given system_id and interface_id to subnet."""
        if mode not in self.SUBNET_CONNECTION_MODES:
            raise ValueError('Invalid subnet connection mode: {}'.format(mode))
        if ip_address and mode != 'STATIC':
            raise ValueError('Must be mode STATIC for ip_address')
        if default_gateway and mode not in ('AUTO', 'STATIC'):
            raise ValueError('Must be mode AUTO or STATIC for default_gateway')
        args = [
            self.profile, 'interface', 'link-subnet', system_id,
            str(interface_id), 'mode=' + mode, 'subnet=' + str(subnet_id),
        ]
        if ip_address:
            args.append('ip_address=' + ip_address)
        if default_gateway:
            args.append('default_gateway=true')
        return self._maas(*args)

    def interface_unlink_subnet(self, system_id, interface_id, link_id):
        """Unlink subnet from interface."""
        return self._maas(
            self.profile, 'interface', 'unlink-subnet', system_id,
            str(interface_id), 'id=' + str(link_id))

    def subnets(self):
        """Return list of all subnets."""
        return self._maas(self.profile, 'subnets', 'read')

    def create_subnet(self, cidr, name=None, fabric_id=None, vlan_id=None,
                      vid=None, space=None, gateway_ip=None, dns_servers=None):
        """Create a subnet with given cidr."""
        if vlan_id and vid:
            raise ValueError('Must only give one of vlan_id and vid')
        args = [self.profile, 'subnets', 'create', 'cidr=' + cidr]
        if name is not None:
            # Defaults to cidr if none is given
            args.append('name=' + name)
        if fabric_id is not None:
            # Uses default fabric if none is given
            args.append('fabric=' + str(fabric_id))
        if vlan_id is not None:
            # Uses default vlan on fabric if none is given
            args.append('vlan=' + str(vlan_id))
        if vid is not None:
            args.append('vid=' + str(vid))
        if space is not None:
            # Uses default space if none is given
            args.append('space=' + str(space))
        if gateway_ip is not None:
            args.append('gateway_ip=' + str(gateway_ip))
        if dns_servers is not None:
            args.append('dns_servers=' + str(dns_servers))
        # TODO(gz): Add support for rdns_mode and allow_proxy from MAAS 2.0
        return self._maas(*args)

    def delete_subnet(self, subnet_id):
        """Delete subnet with given subnet_id."""
        return self._maas(
            self.profile, 'subnet', 'delete', str(subnet_id))

    def ensure_cleanup(self, resource_details):
        """
        Do MAAS specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


class MAAS1Account(MAASAccount):
    """Represent a MAAS 1.X account."""

    _API_PATH = 'api/1.0/'

    def _list_allocated_args(self):
        return (self.profile, 'nodes', 'list-allocated')

    def _machine_release_args(self, machine_id):
        return (self.profile, 'node', 'release', machine_id)


@contextmanager
def maas_account_from_boot_config(env):
    """Create a ContextManager for either a MAASAccount or a MAAS1Account.

    As it's not possible to tell from the maas config which version of the api
    to use, try 2.0 and if that fails on login fallback to 1.0 instead.
    """
    maas_oauth = env.get_cloud_credentials()['maas-oauth']
    args = (env.get_option('name'), env.get_option('maas-server'), maas_oauth)
    manager = MAASAccount(*args)
    try:
        manager.login()
    except subprocess.CalledProcessError as e:
        log.info("Could not login with MAAS 2.0 API, trying 1.0! err -> %s", e)
        manager = MAAS1Account(*args)
        manager.login()
    yield manager
    # We do not call manager.logout() because it can break concurrent procs.


class LXDAccount:
    """Represent a LXD account."""

    def __init__(self, remote=None):
        self.remote = remote

    @classmethod
    @contextmanager
    def from_boot_config(cls, boot_config):
        """Create a ContextManager for a LXDAccount."""
        config = get_config(boot_config)
        remote = config.get('region', None)
        yield cls(remote=remote)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance_id in instance_ids:
            subprocess.check_call(['lxc', 'stop', '--force', instance_id])
            if self.remote:
                instance_id = '{}:{}'.format(self.remote, instance_id)
            subprocess.check_call(['lxc', 'delete', '--force', instance_id])

    def ensure_cleanup(self, resource_details):
        """
        Do LXD specific clean-up activity.
        :param resource_details: The list of resource to be cleaned up
        :return: list of resources that were not cleaned up
        """
        uncleaned_resource = []
        return uncleaned_resource


def get_config(boot_config):
    config = boot_config.make_config_copy()
    if boot_config.provider not in ('lxd', 'manual', 'kubernetes'):
        config.update(boot_config.get_cloud_credentials())
    return config


@contextmanager
def make_substrate_manager(boot_config):
    """A ContextManager that returns an Account for the config's substrate.

    Returns None if the substrate is not supported.
    """
    config = get_config(boot_config)
    substrate_factory = {
        'ec2': AWSAccount.from_boot_config,
        'openstack': OpenStackAccount.from_boot_config,
        'rackspace': OpenStackAccount.from_boot_config,
        'azure': AzureAccount.from_boot_config,
        'azure-arm': AzureARMAccount.from_boot_config,
        'lxd': LXDAccount.from_boot_config,
        'gce': GCEAccount.from_boot_config,
    }
    substrate_type = config['type']
    if substrate_type == 'azure' and 'application-id' in config:
        substrate_type = 'azure-arm'
    factory = substrate_factory.get(substrate_type)
    if factory is None:
        yield None
    else:
        with factory(boot_config) as substrate:
            yield substrate


def start_libvirt_domain(uri, domain):
    """Call virsh to start the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'start', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'already active' in e.output.decode('utf-8'):
            return '%s is already running; nothing to do.' % domain
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(uri, domain, LIBVIRT_DOMAIN_RUNNING):
            return "%s is now running" % domain
        sleep(2)
    raise Exception('libvirt domain %s did not start.' % domain)


def stop_libvirt_domain(uri, domain):
    """Call virsh to shutdown the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'shutdown', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'domain is not running' in e.output.decode('utf-8'):
            return ('%s is not running; nothing to do.' % domain)
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(uri, domain, LIBVIRT_DOMAIN_SHUT_OFF):
            return "%s is now shut off" % domain
        sleep(2)
    raise Exception('libvirt domain %s is not shut off.' % domain)


def verify_libvirt_domain(uri, domain, state=LIBVIRT_DOMAIN_RUNNING):
    """Returns a bool based on if the domain is in the given state.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    @Parm state: The state to verify (e.g. "running or "shut off").
    """

    dom_status = get_libvirt_domstate(uri, domain)
    return state in dom_status


def get_libvirt_domstate(uri, domain):
    """Call virsh to get the state of the given domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'domstate', domain]
    try:
        sub_output = subprocess.check_output(command)
    except subprocess.CalledProcessError:
        raise Exception('%s failed' % command)
    return sub_output


def parse_euca(euca_output):
    for line in euca_output.splitlines():
        fields = line.split('\t')
        if fields[0] != 'INSTANCE':
            continue
        yield fields[1], fields[3]


def describe_instances(instances=None, running=False, job_name=None,
                       env=None):
    command = ['euca-describe-instances']
    if job_name is not None:
        command.extend(['--filter', 'tag:job_name=%s' % job_name])
    if running:
        command.extend(['--filter', 'instance-state-name=running'])
    if instances is not None:
        command.extend(instances)
    log.info(' '.join(command))
    return parse_euca(subprocess.check_output(command, env=env))


def has_nova_instance(boot_config, instance_id):
    """Return True if the instance-id is present.  False otherwise.

    This implementation was extracted from wait_for_state_server_to_shutdown.
    It can be fooled into thinking that the instance-id is present when it is
    not, but should be reliable for determining that the instance-id is not
    present.
    """
    environ = dict(os.environ)
    environ.update(translate_to_env(boot_config.make_config_copy()))
    output = subprocess.check_output(['nova', 'list'], env=environ)
    return bool(instance_id in output)


def get_job_instances(job_name):
    description = describe_instances(job_name=job_name, running=True)
    return (machine_id for machine_id, name in description)


def destroy_job_instances(job_name):
    instances = list(get_job_instances(job_name))
    if len(instances) == 0:
        return
    subprocess.check_call(['euca-terminate-instances'] + instances)


def resolve_remote_dns_names(env, remote_machines):
    """Update addresses of given remote_machines as needed by providers."""
    if env.provider != 'maas':
        # Only MAAS requires special handling at prsent.
        return
    # MAAS hostnames are not resolvable, but we can adapt them to IPs.
    with maas_account_from_boot_config(env) as account:
        allocated_ips = account.get_allocated_ips()
    for remote in remote_machines:
        if remote.get_address() in allocated_ips:
            remote.update_address(allocated_ips[remote.address])
