__metaclass__ = type

import yaml

from collections import defaultdict
from cStringIO import StringIO
from datetime import datetime, timedelta
import httplib
import socket
import subprocess
import sys
from time import sleep
import urllib2

from jujuconfig import get_selected_environment


class ErroredUnit(Exception):

    def __init__(self, environment, unit_name, state):
        msg = '<%s> %s is in state %s' % (environment, unit_name, state)
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
        e_arg = () if environment is None else ('-e', environment.environment)
        sudo_args = ('sudo', '-E', self.full_path) if sudo else ('juju',)
        return sudo_args + ('--show-log', command,) + e_arg + args

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
            None, 'destroy-environment', (environment.environment, '-y'),
            environment.needs_sudo(), check=False)

    def get_juju_output(self, environment, command):
        args = self._full_args(environment, command, False, ())
        return subprocess.check_output(args)

    def get_status(self, environment):
        """Get the current status as a dict."""
        return Status(yaml_loads(self.get_juju_output(environment, 'status')))

    def juju(self, environment, command, args, sudo=False, check=True):
        """Run a command under juju for the current environment."""
        args = self._full_args(environment, command, sudo, args)
        print ' '.join(args)
        sys.stdout.flush()
        if check:
            return subprocess.check_call(args)
        return subprocess.call(args)


class JujuClient16(JujuClientDevel):

    def destroy_environment(self, environment):
        self.juju(environment, 'destroy-environment', ('-y',),
                  environment.needs_sudo(), check=False)


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
                raise ErroredUnit(environment_name, item_name, state_info)
        states = self.agent_states()
        if states.keys() == ['started']:
            return None
        for state, entries in states.items():
            if 'error' in state:
                raise ErroredUnit(environment_name, entries[0],  state)
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

    def juju(self, command, *args):
        return self.client.juju(self, command, args)

    def get_status(self):
        return self.client.get_status(self)

    def wait_for_started(self):
        """Wait until all unit/machine agents are 'started'."""
        for ignored in until_timeout(1200):
            status = self.get_status()
            states = status.check_agents_started(self.environment)
            if states is None:
                break
            print format_listing(states, 'started', self.environment)
            sys.stdout.flush()
        else:
            raise Exception('Timed out waiting for agents to start in %s.' %
                            self.environment)
        return status

    def wait_for_version(self, version):
        for ignored in until_timeout(300):
            versions = self.get_status().get_agent_versions()
            if versions.keys() == [version]:
                break
            print format_listing(versions, version, self.environment)
            sys.stdout.flush()
        else:
            raise Exception('Some versions did not update.')

    def get_matching_agent_version(self, no_build=False):
        version_number = self.client.version.split('-')[0]
        if not no_build and self.local:
            version_number += '.1'
        return version_number


def format_listing(listing, expected, environment):
    value_listing = []
    for value, entries in listing.items():
        if value == expected:
            continue
        value_listing.append('%s: %s' % (value, ', '.join(entries)))
    return ('<%s> ' % environment) + ' | '.join(value_listing)


def check_wordpress(environment, host):
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
            'Cannot get welcome screen at %s %s' % (url, environment))
