import logging
import os
import subprocess
from time import sleep

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


def terminate_instances(env, instance_ids):
    provider_type = env.config.get('type')
    environ = dict(os.environ)
    if provider_type == 'ec2':
        environ.update(get_euca_env(env.config))
        command_args = ['euca-terminate-instances'] + instance_ids
    elif provider_type == 'openstack':
        environ.update(translate_to_env(env.config))
        command_args = ['nova', 'delete'] + instance_ids
    else:
        raise ValueError(
            "This test does not support the %s provider" % provider_type)
    if len(instance_ids) == 0:
        print_now("No instances to delete.")
        return
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

    def list_security_groups(self):
        """List the security groups created by juju in this account."""
        lines = self.get_euca_output(
            'describe-groups', ['--filter', 'description=juju group'])
        for field in self.iter_field_lists(lines):
            if field[:1] != ['GROUP']:
                continue
            yield field[1], field[3]

    def list_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account."""
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
