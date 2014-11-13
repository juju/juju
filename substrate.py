import logging
import os
import subprocess

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from utility import print_now


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
        return cls(get_euca_env(config))

    def __init__(self, euca_environ):
        self.euca_environ = euca_environ

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

    def list_instance_security_groups(self):
        """List the security groups used by instances in this account."""
        lines = self.get_euca_output(
            'describe-instances', [])
        for field in self.iter_field_lists(lines):
            if field[:1] != ['GROUP']:
                continue
            yield field[1], field[2]

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
