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

    def __init__(self, unit_name, state):
        msg = 'Unit %s is in state %s' % (unit_name, state)
        Exception.__init__(self, msg)


def until_timeout(timeout):
    """Yields None until timeout is reached.

    :param timeout: Number of seconds to wait.
    """
    start = datetime.now()
    while datetime.now() - start  < timedelta(0, timeout):
        yield None


def yaml_loads(yaml_str):
    return yaml.safe_load(StringIO(yaml_str))


class JujuClientDevel:
    # This client is meant to work with the latest version of juju.
    # Subclasses will retain support for older versions of juju, so that the
    # latest version is easy to read, and older versions can be trivially
    # deleted.

    @classmethod
    def get_version(cls):
        return cls.get_juju_output(None, '--version').strip()

    @classmethod
    def by_version(cls):
        version = cls.get_version()
        if version.startswith('1.16'):
            return JujuClient16
        else:
            return JujuClientDevel

    @staticmethod
    def _full_args(environment, command, sudo, args):
        full_args = ('juju', command) + args
        if environment is not None:
            full_args = full_args + ('-e', environment.environment)
        if sudo:
            full_args = ('sudo',) + full_args
        return full_args

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
        return yaml_loads(cls.get_juju_output(environment, 'status'))

    @classmethod
    def juju(cls, environment, command, args, sudo=False, check=True):
        """Run a command under juju for the current environment."""
        args = cls._full_args(environment, command, sudo, args)
        print ' '.join(args)
        sys.stdout.flush()
        if check:
            subprocess.check_call(args)
        return subprocess.call(args)


class JujuClient16(JujuClientDevel):

    @classmethod
    def destroy_environment(cls, environment):
        cls.juju(environment, 'destroy-environment', ('-y',),
                 environment.needs_sudo(), check=False)


class Environment:

    def __init__(self, environment):
        self.environment = environment
        self.client = JujuClientDevel.by_version()

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

    @staticmethod
    def agent_items(status):
        for machine_name, machine in sorted(status['machines'].items()):
            yield machine_name, machine
        for service in sorted(status['services'].values()):
            for unit_name, unit in service.get('units', {}).items():
                yield unit_name, unit

    @classmethod
    def agent_states(cls, status):
        """Map agent states to the units and machines in those states."""
        states = defaultdict(list)
        for item_name, item in cls.agent_items(status):
            states[item.get('agent-state', 'no-agent')].append(item_name)
        return states

    def wait_for_started(self):
        """Wait until all unit/machine agents are 'started'."""
        for ignored in until_timeout(300):
            status = self.get_status()
            states = self.agent_states(status)
            if states.keys() == ['started']:
                break
            for state, entries in states.items():
                if 'error' in state:
                    raise ErroredUnit(entries[0],  state)
            print format_listing(states, 'started', self.environment)
            sys.stdout.flush()
        else:
            raise Exception('Timed out!')
        return status


def format_listing(listing, expected, environment):
    value_listing = []
    for value, entries in listing.items():
        if value == expected:
            continue
        value_listing.append('%s: %s' % (value, ', '.join(entries)))
    return ('<%s> ' % environment) + ' | '.join(value_listing)


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
        raise Exception('Cannot get welcome screen at %s' % url)
