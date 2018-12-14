# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import warnings
warnings.warn("contrib.charmhelpers is deprecated", DeprecationWarning)  # noqa

import operator
import tempfile
import time
import yaml
import subprocess

import six
if six.PY3:
    from urllib.request import urlopen
    from urllib.error import (HTTPError, URLError)
else:
    from urllib2 import (urlopen, HTTPError, URLError)

"""Helper functions for writing Juju charms in Python."""

__metaclass__ = type
__all__ = [
    # 'get_config',             # core.hookenv.config()
    # 'log',                    # core.hookenv.log()
    # 'log_entry',              # core.hookenv.log()
    # 'log_exit',               # core.hookenv.log()
    # 'relation_get',           # core.hookenv.relation_get()
    # 'relation_set',           # core.hookenv.relation_set()
    # 'relation_ids',           # core.hookenv.relation_ids()
    # 'relation_list',          # core.hookenv.relation_units()
    # 'config_get',             # core.hookenv.config()
    # 'unit_get',               # core.hookenv.unit_get()
    # 'open_port',              # core.hookenv.open_port()
    # 'close_port',             # core.hookenv.close_port()
    # 'service_control',        # core.host.service()
    'unit_info',              # client-side, NOT IMPLEMENTED
    'wait_for_machine',       # client-side, NOT IMPLEMENTED
    'wait_for_page_contents',  # client-side, NOT IMPLEMENTED
    'wait_for_relation',      # client-side, NOT IMPLEMENTED
    'wait_for_unit',          # client-side, NOT IMPLEMENTED
]


SLEEP_AMOUNT = 0.1


# We create a juju_status Command here because it makes testing much,
# much easier.
def juju_status():
    subprocess.check_call(['juju', 'status'])

# re-implemented as charmhelpers.fetch.configure_sources()
# def configure_source(update=False):
#    source = config_get('source')
#    if ((source.startswith('ppa:') or
#         source.startswith('cloud:') or
#         source.startswith('http:'))):
#        run('add-apt-repository', source)
#    if source.startswith("http:"):
#        run('apt-key', 'import', config_get('key'))
#    if update:
#        run('apt-get', 'update')


# DEPRECATED: client-side only
def make_charm_config_file(charm_config):
    charm_config_file = tempfile.NamedTemporaryFile(mode='w+')
    charm_config_file.write(yaml.dump(charm_config))
    charm_config_file.flush()
    # The NamedTemporaryFile instance is returned instead of just the name
    # because we want to take advantage of garbage collection-triggered
    # deletion of the temp file when it goes out of scope in the caller.
    return charm_config_file


# DEPRECATED: client-side only
def unit_info(service_name, item_name, data=None, unit=None):
    if data is None:
        data = yaml.safe_load(juju_status())
    service = data['services'].get(service_name)
    if service is None:
        # XXX 2012-02-08 gmb:
        #     This allows us to cope with the race condition that we
        #     have between deploying a service and having it come up in
        #     `juju status`. We could probably do with cleaning it up so
        #     that it fails a bit more noisily after a while.
        return ''
    units = service['units']
    if unit is not None:
        item = units[unit][item_name]
    else:
        # It might seem odd to sort the units here, but we do it to
        # ensure that when no unit is specified, the first unit for the
        # service (or at least the one with the lowest number) is the
        # one whose data gets returned.
        sorted_unit_names = sorted(units.keys())
        item = units[sorted_unit_names[0]][item_name]
    return item


# DEPRECATED: client-side only
def get_machine_data():
    return yaml.safe_load(juju_status())['machines']


# DEPRECATED: client-side only
def wait_for_machine(num_machines=1, timeout=300):
    """Wait `timeout` seconds for `num_machines` machines to come up.

    This wait_for... function can be called by other wait_for functions
    whose timeouts might be too short in situations where only a bare
    Juju setup has been bootstrapped.

    :return: A tuple of (num_machines, time_taken). This is used for
             testing.
    """
    # You may think this is a hack, and you'd be right. The easiest way
    # to tell what environment we're working in (LXC vs EC2) is to check
    # the dns-name of the first machine. If it's localhost we're in LXC
    # and we can just return here.
    if get_machine_data()[0]['dns-name'] == 'localhost':
        return 1, 0
    start_time = time.time()
    while True:
        # Drop the first machine, since it's the Zookeeper and that's
        # not a machine that we need to wait for. This will only work
        # for EC2 environments, which is why we return early above if
        # we're in LXC.
        machine_data = get_machine_data()
        non_zookeeper_machines = [
            machine_data[key] for key in list(machine_data.keys())[1:]]
        if len(non_zookeeper_machines) >= num_machines:
            all_machines_running = True
            for machine in non_zookeeper_machines:
                if machine.get('instance-state') != 'running':
                    all_machines_running = False
                    break
            if all_machines_running:
                break
        if time.time() - start_time >= timeout:
            raise RuntimeError('timeout waiting for service to start')
        time.sleep(SLEEP_AMOUNT)
    return num_machines, time.time() - start_time


# DEPRECATED: client-side only
def wait_for_unit(service_name, timeout=480):
    """Wait `timeout` seconds for a given service name to come up."""
    wait_for_machine(num_machines=1)
    start_time = time.time()
    while True:
        state = unit_info(service_name, 'agent-state')
        if 'error' in state or state == 'started':
            break
        if time.time() - start_time >= timeout:
            raise RuntimeError('timeout waiting for service to start')
        time.sleep(SLEEP_AMOUNT)
    if state != 'started':
        raise RuntimeError('unit did not start, agent-state: ' + state)


# DEPRECATED: client-side only
def wait_for_relation(service_name, relation_name, timeout=120):
    """Wait `timeout` seconds for a given relation to come up."""
    start_time = time.time()
    while True:
        relation = unit_info(service_name, 'relations').get(relation_name)
        if relation is not None and relation['state'] == 'up':
            break
        if time.time() - start_time >= timeout:
            raise RuntimeError('timeout waiting for relation to be up')
        time.sleep(SLEEP_AMOUNT)


# DEPRECATED: client-side only
def wait_for_page_contents(url, contents, timeout=120, validate=None):
    if validate is None:
        validate = operator.contains
    start_time = time.time()
    while True:
        try:
            stream = urlopen(url)
        except (HTTPError, URLError):
            pass
        else:
            page = stream.read()
            if validate(page, contents):
                return page
        if time.time() - start_time >= timeout:
            raise RuntimeError('timeout waiting for contents of ' + url)
        time.sleep(SLEEP_AMOUNT)
