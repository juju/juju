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
        if self.now() - self.start  >= timedelta(0, self.timeout):
            raise StopIteration
        return None


def yaml_loads(yaml_str):
    return yaml.safe_load(StringIO(yaml_str))


class JujuClientDevel:
    # This client is meant to work with the latest version of juju.
    # Subclasses will retain support for older versions of juju, so that the
    # latest version is easy to read, and older versions can be trivially
    # deleted.

    def __init__(self, version):
        self.version = version

    @classmethod
    def get_version(cls):
        return cls.get_juju_output(None, '--version').strip()

    @classmethod
    def by_version(cls):
        version = cls.get_version()
        if version.startswith('1.16'):
            return JujuClient16(version)
        else:
            return JujuClientDevel(version)

    @staticmethod
    def _full_args(environment, command, sudo, args):
        e_arg = () if environment is None else ('-e', environment.environment)
        sudo_arg = ('sudo',) if sudo else ()
        return sudo_arg + ('juju', command) + e_arg + args

    @classmethod
    def bootstrap(cls, environment):
        """Bootstrap, using sudo if necessary."""
        cls.juju(environment, 'bootstrap', ('--constraints', 'mem=2G'),
                 environment.needs_sudo())

    @classmethod
    def destroy_environment(cls, environment):
        cls.juju(
            None, 'destroy-environment', (environment.environment, '-y'),
            environment.needs_sudo(), check=False)

    @classmethod
    def get_juju_output(cls, environment, command):
        args = cls._full_args(environment, command, False, ())
        return subprocess.check_output(args)

    @classmethod
    def get_status(cls, environment):
        """Get the current status as a dict."""
        return Status(yaml_loads(cls.get_juju_output(environment, 'status')))

    @classmethod
    def juju(cls, environment, command, args, sudo=False, check=True):
        """Run a command under juju for the current environment."""
        args = cls._full_args(environment, command, sudo, args)
        print ' '.join(args)
        sys.stdout.flush()
        if check:
            return subprocess.check_call(args)
        return subprocess.call(args)


class JujuClient16(JujuClientDevel):

    @classmethod
    def destroy_environment(cls, environment):
        cls.juju(environment, 'destroy-environment', ('-y',),
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

    def __init__(self, environment, client=None):
        self.environment = environment
        if client is None:
            client = JujuClientDevel.by_version()
        self.client = client

    def needs_sudo(self):
        return bool(self.environment == 'local')

    def bootstrap(self):
        return self.client.bootstrap(self)

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
