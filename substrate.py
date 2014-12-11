__metaclass__ = type

import logging
import os
import subprocess
import sys
from time import sleep

sys.path.insert(
    0, os.path.realpath(os.path.join(__file__, '../../juju-release-tools')))

from boto import ec2
from boto.exception import EC2ResponseError

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from utility import (
    print_now,
    until_timeout,
)


LIBVIRT_DOMAIN_RUNNING = 'running'
LIBVIRT_DOMAIN_SHUT_OFF = 'shut off'


class StillProvisioning(Exception):
    """Attempted to terminate instances still provisioning."""

    def __init__(self, instance_ids):
        super(StillProvisioning, self).__init__(
            'Still provisioning: {}'.format(', '.join(instance_ids)))
        self.instance_ids = instance_ids


def terminate_instances(env, instance_ids):
    if len(instance_ids) == 0:
        print_now("No instances to delete.")
        return
    provider_type = env.config.get('type')
    environ = dict(os.environ)
    if provider_type == 'ec2':
        environ.update(get_euca_env(env.config))
        command_args = ['euca-terminate-instances'] + instance_ids
    elif provider_type == 'openstack':
        environ.update(translate_to_env(env.config))
        command_args = ['nova', 'delete'] + instance_ids
    elif provider_type == 'maas':
        profile_name = env.environment
        maas_url = env.config.get('maas-server') + 'api/1.0/'
        maas_credentials = env.config.get('maas-oauth')
        for instance in instance_ids:
            maas_system_id = instance.split('/')[5]
            commands = [
                ['maas', 'login', profile_name, maas_url, maas_credentials],
                ['maas', profile_name, 'node', 'release', maas_system_id],
                ['maas', 'logout', profile_name]
            ]
            print_now("Deleting %s." % instance)
            for cmd in commands:
                subprocess.check_call(cmd)
        return
    else:
        substrate = make_substrate(env.config)
        if substrate is None:
            raise ValueError(
                "This test does not support the %s provider" % provider_type)
        return substrate.terminate_instances(instance_ids)
    print_now("Deleting %s." % ', '.join(instance_ids))
    subprocess.check_call(command_args, env=environ)


class AWSAccount:
    """Represent the credentials of an AWS account."""

    @classmethod
    def from_config(cls, config):
        """Create an AWSAccount from a juju environment dict."""
        return cls(get_euca_env(config), config['region'])

    def __init__(self, euca_environ, region):
        self.euca_environ = euca_environ
        self.region = region

    def get_environ(self):
        """Return the environ to run euca in."""
        environ = dict(os.environ)
        environ.update(self.euca_environ)
        return environ

    @staticmethod
    def get_commandline(command, args):
        """Return the euca commandline."""
        return ['euca-' + command] + args

    def euca(self, command, args):
        """Run a euca-* command."""
        commandline = self.get_commandline(command, args)
        logging.info(' '.join(commandline))
        return subprocess.check_call(commandline,
                                     env=self.get_environ())

    def get_euca_output(self, command, args):
        """Run a euca-* command and return its output."""
        commandline = self.get_commandline(command, args)
        logging.debug(' '.join(commandline))
        return subprocess.check_output(commandline,
                                       env=self.get_environ())

    @staticmethod
    def iter_field_lists(lines):
        """Iterate through lists of fields for euca output."""
        for line in lines.splitlines():
            yield line.split('\t')

    def iter_security_groups(self):
        """Iterate through security groups created by juju in this account.

        :return: an itertator of (group-id, group-name) tuples.
        """
        lines = self.get_euca_output(
            'describe-groups', ['--filter', 'description=juju group'])
        for field in self.iter_field_lists(lines):
            if field[:1] != ['GROUP']:
                continue
            yield field[1], field[3]

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an itertator of (group-id, group-name) tuples.
        """
        logging.info('Listing security groups in use.')
        connection = self.get_ec2_connection()
        reservations = connection.get_all_instances(instance_ids=instance_ids)
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
            try:
                self.euca('delete-group', [group])
            except subprocess.CalledProcessError:
                failures.append(group)
        return failures

    def get_ec2_connection(self):
        return ec2.connect_to_region(
            self.region, aws_access_key_id=self.euca_environ['EC2_ACCESS_KEY'],
            aws_secret_access_key=self.euca_environ['EC2_SECRET_KEY'],
        )

    def delete_detached_interfaces(self, security_groups):
        """Delete detached network interfaces for supplied groups.

        :param security_groups: A collection of security_group ids.
        :return: A collection of security groups which still have interfaces in
            them.
        """
        connection = self.get_ec2_connection()
        interfaces = connection.get_all_network_interfaces(
            filters={'status': 'available'})
        unclean = set()
        for interface in interfaces:
            for group in interface.groups:
                if group.id in security_groups:
                    try:
                        interface.delete()
                    except EC2ResponseError as e:
                        if e.error_code not in (
                                'InvalidNetworkInterface.InUse',
                                'InvalidNetworkInterfaceID.NotFound'):
                            raise
                        logging.info(
                            'Failed to delete interface {}'.format(
                                interface.id))
                        unclean.update(g.id for g in interface.groups)
                    break
        return unclean


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
    def from_config(cls, config):
        """Create an OpenStackAccount from a juju environment dict."""
        return cls(
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

        :return: an itertator of (group-id, group-name) tuples.
        """
        return ((g.id, g.name) for g in self.client.security_groups.list()
                if g.description == 'juju group')

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an itertator of (group-id, group-name) tuples.
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


class JoyentAccount:
    """Represent a Joyent account."""

    def __init__(self, client):
        self.client = client

    @classmethod
    def from_config(cls, config):
        """Create a JoyentAccount from a juju environment dict."""
        from joyent import Client
        return cls(Client(config['sdc-url'], config['manta-user'],
                          config['manta-key-id']))

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        provisioning = []
        for instance_id in instance_ids:
            machine_info = self.client._list_machines(instance_id)
            if machine_info['state'] == 'provisioning':
                provisioning.append(instance_id)
                continue
            self._terminate_instance(instance_id)
        if len(provisioning) > 0:
            raise StillProvisioning(provisioning)

    def _terminate_instance(self, machine_id):
        logging.info('Stopping instance {}'.format(machine_id))
        self.client.stop_machine(machine_id)
        for ignored in until_timeout(30):
            stopping_machine = self.client._list_machines(machine_id)
            if stopping_machine['state'] == 'stopped':
                break
            sleep(3)
        else:
            raise Exception('Instance did not stop: {}'.format(machine_id))
        logging.info('Terminating instance {}'.format(machine_id))
        self.client.delete_machine(machine_id)


def make_substrate(config):
    """Return an Account for the config's substrate.

    Returns None if the substrate is not supported.
    """
    substrate_factory = {
        'ec2': AWSAccount.from_config,
        'openstack': OpenStackAccount.from_config,
        'joyent': JoyentAccount.from_config,
        }
    return substrate_factory.get(config['type'], lambda x: None)(config)


def start_libvirt_domain(URI, domain):
    """Call virsh to start the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', URI, 'start', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'already active' in e.output:
            return '%s is already running; nothing to do.' % domain
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(URI, domain, LIBVIRT_DOMAIN_RUNNING):
            return "%s is now running" % domain
        sleep(2)
    raise Exception('libvirt domain %s did not start.' % domain)


def stop_libvirt_domain(URI, domain):
    """Call virsh to shutdown the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', URI, 'shutdown', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'domain is not running' in e.output:
            return ('%s is not running; nothing to do.' % domain)
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(URI, domain, LIBVIRT_DOMAIN_SHUT_OFF):
            return "%s is now shut off" % domain
        sleep(2)
    raise Exception('libvirt domain %s is not shut off.' % domain)


def verify_libvirt_domain(URI, domain, state=LIBVIRT_DOMAIN_RUNNING):
    """Returns a bool based on if the domain is in the given state.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    @Parm state: The state to verify (e.g. "running or "shut off").
    """

    dom_status = get_libvirt_domstate(URI, domain)
    return state in dom_status


def get_libvirt_domstate(URI, domain):
    """Call virsh to get the state of the given domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', URI, 'domstate', domain]
    try:
        sub_output = subprocess.check_output(command)
    except subprocess.CalledProcessError:
        raise Exception('%s failed' % command)
    return sub_output
