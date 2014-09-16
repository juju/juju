from __future__ import print_function

__metaclass__ = type

import yaml

from collections import defaultdict
from cStringIO import StringIO
from datetime import datetime
import os
import subprocess
import sys
import tempfile

from jujuconfig import get_selected_environment


WIN_JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')


class ErroredUnit(Exception):

    def __init__(self, unit_name, state):
        msg = '%s is in state %s' % (unit_name, state)
        Exception.__init__(self, msg)


class until_timeout:

    """Yields remaining number of seconds.  Stops when timeout is reached.

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
        elapsed = self.now() - self.start
        remaining = self.timeout - elapsed.total_seconds()
        if remaining <= 0:
            raise StopIteration
        return remaining


def yaml_loads(yaml_str):
    return yaml.safe_load(StringIO(yaml_str))


class CannotConnectEnv(subprocess.CalledProcessError):

    def __init__(self, e):
        super(CannotConnectEnv, self).__init__(e.returncode, e.cmd, e.output)


class EnvJujuClient:

    @staticmethod
    def get_full_path():
        if sys.platform == 'win32':
            return WIN_JUJU_CMD
        return subprocess.check_output(('which', 'juju')).rstrip('\n')

    @classmethod
    def by_version(cls, env):
        version = cls.get_version()
        full_path = cls.get_full_path()
        if version.startswith('1.16'):
            raise Exception('Unsupported juju: %s' % version)
        else:
            return EnvJujuClient(env, version, full_path)

    def __init__(self, env, version, full_path, debug=False):
        self.env = env
        self.version = version
        self.full_path = full_path
        self.debug = debug

    def _full_args(self, command, sudo, args, timeout=None, include_e=True):
        # sudo is not needed for devel releases.
        if self.env is None or not include_e:
            e_arg = ()
        else:
            e_arg = ('-e', self.env.environment)
        if timeout is None:
            prefix = ()
        else:
            prefix = ('timeout', '%.2fs' % timeout)
        logging = '--debug' if self.debug else '--show-log'
        return prefix + ('juju', logging, command,) + e_arg + args

    def _shell_environ(self):
        """Generate a suitable shell environment.

        Juju's directory must be in the PATH to support plugins.
        """
        env = dict(os.environ)
        if self.full_path is not None:
            env['PATH'] = '{}:{}'.format(os.path.dirname(self.full_path),
                                         env['PATH'])
        return env

    def juju(self, command, args, sudo=False, check=True, include_e=True):
        """Run a command under juju for the current environment."""
        args = self._full_args(command, sudo, args, include_e=include_e)
        print(' '.join(args))
        sys.stdout.flush()
        env = self._shell_environ()
        if check:
            return subprocess.check_call(args, env=env)
        return subprocess.call(args, env=env)

    def get_juju_output(self, command, *args, **kwargs):
        args = self._full_args(command, False, args,
                               timeout=kwargs.get('timeout'))
        env = self._shell_environ()
        with tempfile.TemporaryFile() as stderr:
            try:
                return subprocess.check_output(args, stderr=stderr, env=env)
            except subprocess.CalledProcessError as e:
                stderr.seek(0)
                e.stderr = stderr.read()
                if ('Unable to connect to environment' in e.stderr
                        or 'MissingOrIncorrectVersionHeader' in e.stderr
                        or '307: Temporary Redirect' in e.stderr):
                    raise CannotConnectEnv(e)
                print('!!! ' + e.stderr)
                raise

    @staticmethod
    def get_version():
        return subprocess.check_output(('juju', '--version')).strip()

    def bootstrap(self):
        """Bootstrap, using sudo if necessary."""
        if self.env.hpcloud:
            constraints = 'mem=2G'
        else:
            constraints = 'mem=2G'
        self.juju('bootstrap', ('--constraints', constraints),
                  self.env.needs_sudo())

    def destroy_environment(self):
        self.juju(
            'destroy-environment', (self.env.environment, '--force', '-y'),
            self.env.needs_sudo(), check=False, include_e=False)

    def get_status(self, timeout=60):
        """Get the current status as a dict."""
        for ignored in until_timeout(timeout):
            try:
                return Status(yaml_loads(
                    self.get_juju_output('status')))
            except subprocess.CalledProcessError as e:
                pass
        raise Exception(
            'Timed out waiting for juju status to succeed: %s' % e)

    def get_env_option(self, option):
        """Return the value of the environment's configured option."""
        return self.get_juju_output('get-env', option)

    def set_env_option(self, option, value):
        """Set the value of the option in the environment."""
        option_value = "%s=%s" % (option, value)
        return self.juju('set-env', (option_value,))


class JujuClientDevel:
    # This client is meant to work with the latest version of juju.
    # Subclasses will retain support for older versions of juju, so that the
    # latest version is easy to read, and older versions can be trivially
    # deleted.

    def __init__(self, version, full_path):
        self.version = version
        self.full_path = full_path
        self.debug = False

    @classmethod
    def get_version(cls):
        return EnvJujuClient.get_version()

    @classmethod
    def get_full_path(cls):
        return EnvJujuClient.get_full_path()

    @classmethod
    def by_version(cls):
        version = cls.get_version()
        full_path = cls.get_full_path()
        if version.startswith('1.16'):
            raise Exception('Unsupported juju: %s' % version)
        else:
            return JujuClientDevel(version, full_path)

    def _get_env_client(self, environment):
        return EnvJujuClient(environment, self.version, self.full_path,
                             self.debug)

    def bootstrap(self, environment):
        """Bootstrap, using sudo if necessary."""
        return self._get_env_client(environment).bootstrap()

    def destroy_environment(self, environment):
        return self._get_env_client(environment).destroy_environment()

    def get_juju_output(self, environment, command, *args, **kwargs):
        return self._get_env_client(environment).get_juju_output(
            command, *args, **kwargs)

    def get_status(self, environment, timeout=60):
        """Get the current status as a dict."""
        return self._get_env_client(environment).get_status(timeout)

    def get_env_option(self, environment, option):
        """Return the value of the environment's configured option."""
        return self._get_env_client(environment).get_env_option(option)

    def set_env_option(self, environment, option, value):
        """Set the value of the option in the environment."""
        return self._get_env_client(environment).set_env_option(option, value)

    def juju(self, environment, command, args, sudo=False, check=True):
        """Run a command under juju for the current environment."""
        return self._get_env_client(environment).juju(
            command, args, sudo, check)


class Status:

    def __init__(self, status):
        self.status = status

    def iter_machines(self):
        for machine_name, machine in sorted(self.status['machines'].items()):
            yield machine_name, machine

    def agent_items(self):
        for result in self.iter_machines():
            yield result
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
            self.kvm = (
                self.local and bool(self.config.get('container') == 'kvm'))
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

    def get_status(self, timeout=60):
        return self.client.get_status(self, timeout)

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

    def wait_for_version(self, version, timeout=300):
        for ignored in until_timeout(timeout):
            try:
                versions = self.get_status(120).get_agent_versions()
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
        # strip the series and srch from the built version.
        version_parts = self.client.version.split('-')
        if len(version_parts) == 4:
            version_number = '-'.join(version_parts[0:2])
        else:
            version_number = version_parts[0]
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
