from __future__ import print_function

__metaclass__ = type

import yaml

from collections import defaultdict
from cStringIO import StringIO
from datetime import datetime, timedelta
import httplib
import os
import socket
import subprocess
import sys
import tempfile
from time import sleep
import urllib2

from jujuconfig import get_selected_environment


WIN_JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')


class ErroredUnit(Exception):

    def __init__(self, unit_name, state):
        msg = '%s is in state %s' % (unit_name, state)
        Exception.__init__(self, msg)


class until_timeout:

    """Yields None until timeout is reached.

    :ivar timeout: Number of seconds to wait.
    """
    def __init__(self, timeout):
        self.timeout = timeout
        self.start = self.now()

    def __iter__(self):
        return self

    @staticmethod
    def now():
        return datetime.now()

    def next(self):
        if self.now() - self.start >= timedelta(0, self.timeout):
            raise StopIteration
        return None


def yaml_loads(yaml_str):
    return yaml.safe_load(StringIO(yaml_str))


class CannotConnectEnv(subprocess.CalledProcessError):

    def __init__(self, e):
        super(CannotConnectEnv, self).__init__(e.returncode, e.cmd, e.output)


class JujuClientDevel:
    # This client is meant to work with the latest version of juju.
    # Subclasses will retain support for older versions of juju, so that the
    # latest version is easy to read, and older versions can be trivially
    # deleted.

    def __init__(self, version, full_path):
        self.version = version
        self.full_path = full_path

    @classmethod
    def get_version(cls):
        return subprocess.check_output(('juju', '--version')).strip()

    @classmethod
    def get_full_path(cls):
        if sys.platform == 'win32':
            return WIN_JUJU_CMD
        return subprocess.check_output(('which', 'juju')).rstrip('\n')

    @classmethod
    def by_version(cls):
        version = cls.get_version()
        full_path = cls.get_full_path()
        if version.startswith('1.16'):
            return JujuClient16(version, full_path)
        else:
            return JujuClientDevel(version, full_path)

    def _full_args(self, environment, command, sudo, args):
        # sudo is not needed for devel releases.
        e_arg = () if environment is None else ('-e', environment.environment)
        return ('juju', '--show-log', command,) + e_arg + args

    def bootstrap(self, environment):
        """Bootstrap, using sudo if necessary."""
        if environment.hpcloud:
            constraints = 'mem=4G'
        else:
            constraints = 'mem=2G'
        self.juju(environment, 'bootstrap', ('--constraints', constraints),
                  environment.needs_sudo())

    def destroy_environment(self, environment):
        self.juju(
            None, 'destroy-environment',
            (environment.environment, '--force', '-y'),
            environment.needs_sudo(), check=False)

    def get_juju_output(self, environment, command, *args):
        args = self._full_args(environment, command, False, args)
        with tempfile.TemporaryFile() as stderr:
            try:
                return subprocess.check_output(args, stderr=stderr)
            except subprocess.CalledProcessError as e:
                stderr.seek(0)
                e.stderr = stderr.read()
                if ('Unable to connect to environment' in e.stderr
                        or 'MissingOrIncorrectVersionHeader' in e.stderr
                        or '307: Temporary Redirect' in e.stderr):
                    raise CannotConnectEnv(e)
                print('!!! ' + e.stderr)
                raise

    def get_status(self, environment):
        """Get the current status as a dict."""
        for ignored in until_timeout(30 * 30):
            try:
                return Status(yaml_loads(
                    self.get_juju_output(environment, 'status')))
            except subprocess.CalledProcessError as e:
                pass
        raise Exception(
            'Timed out waiting for juju status to succeed: %s' % e)

    def get_env_option(self, environment, option):
        """Return the value of the environment's configured option."""
        return self.get_juju_output(environment, 'get-env', option)

    def set_env_option(self, environment, option, value):
        """Set the value of the option in the environment."""
        option_value = "%s=%s" % (option, value)
        return self.juju(environment, 'set-env', (option_value,))

    def juju(self, environment, command, args, sudo=False, check=True):
        """Run a command under juju for the current environment."""
        args = self._full_args(environment, command, sudo, args)
        print(' '.join(args))
        sys.stdout.flush()
        if check:
            return subprocess.check_call(args)
        return subprocess.call(args)


class JujuClient16(JujuClientDevel):

    def destroy_environment(self, environment):
        self.juju(environment, 'destroy-environment', ('-y',),
                  environment.needs_sudo(), check=False)

    def _full_args(self, environment, command, sudo, args):
        # juju 1.16.x required sudo, so replace the juju command with it, as
        # appropriate.
        full = super(JujuClient16, self)._full_args(
            environment, command, sudo, args)
        sudo_args = ('sudo', '-E', self.full_path) if sudo else ('juju',)
        return sudo_args + full[1:]


class Status:

    def __init__(self, status):
        self.status = status

    def agent_items(self):
        for machine_name, machine in sorted(self.status['machines'].items()):
            yield machine_name, machine
        for service in sorted(self.status['services'].values()):
            for unit_name, unit in service.get('units', {}).items():
                yield unit_name, unit

    def agent_states(self):
        """Map agent states to the units and machines in those states."""
        states = defaultdict(list)
        for item_name, item in self.agent_items():
            states[item.get('agent-state', 'no-agent')].append(item_name)
        return states

    def check_agents_started(self, environment_name):
        """Check whether all agents are in the 'started' state.

        If not, return agent_states output.  If so, return None.
        If an error is encountered for an agent, raise ErroredUnit
        """
        # Look for errors preventing an agent from being installed
        for item_name, item in self.agent_items():
            state_info = item.get('agent-state-info', '')
            if 'error' in state_info:
                raise ErroredUnit(item_name, state_info)
        states = self.agent_states()
        if states.keys() == ['started']:
            return None
        for state, entries in states.items():
            if 'error' in state:
                raise ErroredUnit(entries[0],  state)
        return states

    def get_agent_versions(self):
        versions = defaultdict(set)
        for item_name, item in self.agent_items():
            versions[item.get('agent-version', 'unknown')].add(item_name)
        return versions


class Environment:

    def __init__(self, environment, client=None, config=None):
        self.environment = environment
        self.client = client
        self.config = config
        if self.config is not None:
            self.local = bool(self.config.get('type') == 'local')
            self.hpcloud = bool(
                'hpcloudsvc' in self.config.get('auth-url', ''))
        else:
            self.local = False
            self.hpcloud = False

    @classmethod
    def from_config(cls, name):
        client = JujuClientDevel.by_version()
        return cls(name, client, get_selected_environment(name)[0])

    def needs_sudo(self):
        return self.local

    def bootstrap(self):
        return self.client.bootstrap(self)

    def upgrade_juju(self):
        args = ('--version', self.get_matching_agent_version(no_build=True))
        if self.local:
            args += ('--upload-tools',)
        self.client.juju(self, 'upgrade-juju', args)

    def destroy_environment(self):
        return self.client.destroy_environment(self)

    def deploy(self, charm):
        args = (charm,)
        return self.juju('deploy', *args)

    def juju(self, command, *args):
        return self.client.juju(self, command, args)

    def get_status(self):
        return self.client.get_status(self)

    def wait_for_started(self, timeout=1200):
        """Wait until all unit/machine agents are 'started'."""
        for ignored in until_timeout(timeout):
            try:
                status = self.get_status()
            except CannotConnectEnv:
                print('Supressing "Unable to connect to environment"')
                continue
            states = status.check_agents_started(self.environment)
            if states is None:
                break
            print(format_listing(states, 'started'))
            sys.stdout.flush()
        else:
            raise Exception('Timed out waiting for agents to start in %s.' %
                            self.environment)
        return status

    def wait_for_version(self, version):
        for ignored in until_timeout(30 * 60):
            try:
                versions = self.get_status().get_agent_versions()
            except CannotConnectEnv:
                print('Supressing "Unable to connect to environment"')
                continue
            if versions.keys() == [version]:
                break
            print(format_listing(versions, version))
            sys.stdout.flush()
        else:
            raise Exception('Some versions did not update.')

    def get_matching_agent_version(self, no_build=False):
        version_number = self.client.version.split('-')[0]
        if not no_build and self.local:
            version_number += '.1'
        return version_number

    def set_testing_tools_metadata_url(self):
        url = self.client.get_env_option(self, 'tools-metadata-url')
        if 'testing' not in url:
            testing_url = url.replace('/tools', '/testing/tools')
            self.client.set_env_option(self, 'tools-metadata-url',  testing_url)


def format_listing(listing, expected):
    value_listing = []
    for value, entries in listing.items():
        if value == expected:
            continue
        value_listing.append('%s: %s' % (value, ', '.join(entries)))
    return ' | '.join(value_listing)


def check_wordpress(host):
    """"Check whether Wordpress has come up successfully.

    Times out after 30 seconds.
    """
    welcome_text = ('Welcome to the famous five minute WordPress'
                    ' installation process!')
    url = 'http://%s/wp-admin/install.php' % host
    for ignored in until_timeout(30):
        try:
            page = urllib2.urlopen(url)
        except (urllib2.URLError, httplib.HTTPException, socket.error):
            pass
        else:
            if welcome_text in page.read():
                break
        # Let's not DOS wordpress
        sleep(1)
    else:
        raise Exception(
            'Cannot get welcome screen at %s' % (url))
