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
        Exception.__init__('Unit %s is in state %s' % unit_name, state)


def until_timeout(timeout):
    """Yields None until timeout is reached.

    :param timeout: Number of seconds to wait.
    """
    start = datetime.now()
    while datetime.now() - start  < timedelta(0, timeout):
        yield None


class Environment:

    def __init__(self, environment):
        self.environment = environment

    def _full_args(self, command, *args):
        return ('juju', command, '-e', self.environment) + args

    def juju(self, command, *args):
        """Run a command under juju for the current environment."""
        args = self._full_args(command, *args)
        print ' '.join(args)
        sys.stdout.flush()
        return subprocess.check_call(args)

    @staticmethod
    def agent_states(status):
        """Map agent states to the units and machines in those states."""
        states = defaultdict(list)
        for machine_name, machine in sorted(status['machines'].items()):
            states[machine.get('agent-state', 'no-agent')].append(machine_name)
        for service in sorted(status['services'].values()):
            for unit_name, unit in service['units'].items():
                states[unit['agent-state']].append(unit_name)
        return states

    def get_status(self):
        """Get the current status as a dict."""
        args = self._full_args('status')
        return yaml.safe_load(StringIO(subprocess.check_output(args)))

    def wait_for_started(self):
        """Wait until all unit/machine agents are 'started'."""
        for ignored in until_timeout(300):
            status = self.get_status()
            states = self.agent_states(status)
            pending = False
            state_listing = []
            for state, entries in states.items():
                if state == 'started':
                    continue
                if 'error' in state:
                    raise ErroredUnit(entries[0],  state)
                pending = True
                state_listing.append('%s: %s' % (state, ' '.join(entries)))
            print ' | '.join(state_listing)
            sys.stdout.flush()
            if not pending:
                return status
        else:
            raise Exception('Timed out!')


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
