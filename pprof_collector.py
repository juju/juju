"""Collector of pprof details using juju introspection."""

from __future__ import print_function

from datetime import datetime
from functools import partial
import logging
import os
import requests

from utility import get_unit_ipaddress

__metaclass__ = type

log = logging.getLogger("pprof_collector")


class NoopCollector:

    def __init__(self, client, machine_id):
        """Internal collector for pprof profiles, not intended to be used
        directly

        Does not actually install or collect data. Used as a stand in when
        collection is disabled.

        :param client: jujupy.ModelClient object to install the pprof software
          with.
        :machine_id: ID of the juju machine with which to install pprof
          software/charm.
        """
        pass

    def collect_profile(self, filepath, seconds):
        """Collect `seconds` worth of CPU profile."""
        pass

    def collect_heap(self, filepath, seconds):
        """Collect `seconds` worth of heap profile."""
        pass

    def collect_goroutines(self, filepath, seconds):
        """Collect `seconds` worth of goroutines profile."""
        pass


class ActiveCollector(NoopCollector):

    def __init__(self, client, machine_id):
        """Internal collector for pprof profiles, not intended to be used
        directly.

        Installs required  on `machine_id` and enable collection of
        pprof details.

        For instance to install on the controller machine pass in
        client=client.get_contoller_client() and machine_id='0'

        :param client: jujupy.ModelClient object to install the pprof software
          with.
        :machine_id: ID of the juju machine with which to install pprof
          software/charm.
        """
        introspection_ip = install_introspection_charm(
            client, machine_id)
        self._get_url_func = partial(
            get_profile_url, introspection_ip, machine_id)

    def _collect_profile(self, profile, filepath, seconds):
        get_profile_reading(self._get_url_func(profile, seconds), filepath)

    def collect_profile(self, filepath, seconds):
        """Collect `seconds` worth of CPU profile."""
        self._collect_profile('profile', filepath, seconds)

    def collect_heap(self, filepath, seconds):
        """Collect `seconds` worth of heap profile."""
        self._collect_profile('heap', filepath, seconds)

    def collect_goroutines(self, filepath, seconds):
        """Collect `seconds` worth of goroutines profile."""
        self._collect_profile('goroutines', filepath, seconds)


def get_profile_url(ipaddress, machine_id, profile_name, seconds):
    profile_url = 'agents/machine-{}/debug/pprof/{}'.format(
        machine_id, profile_name)
    return 'http://{}:19090/{}?seconds={}'.format(
        ipaddress, profile_url, seconds)


def get_profile_reading(url, filepath):
    res = requests.get(url)
    with open(filepath, 'wb') as f:
        f.write(res.content)


def install_introspection_charm(client, machine_id):
    client.deploy(_get_introspection_charm_url(), to=machine_id)
    client.wait_for_started()
    client.wait_for_workloads()
    client.juju('expose', 'juju-introspection')

    return get_unit_ipaddress(client, 'juju-introspection/0')


def _get_introspection_charm_url():
    return 'cs:~axwalk/juju-introspection'


class PPROFCollector:

    FILE_TIMESTAMP = '%y%m%d-%H%M%S'

    def __init__(self, client, machine_id, logs_dir, active=False):
        """Collector of pprof profiles from a machine.

        Defaults to being non-active meaning that any attempt to collect a
        profile will be a no-op.
        Setting active (either at creation or at any point during the lifespan
        of this object) will enable collection of pprof profiles from the
        `machine_id`.
        (Note. first time going active will result in the introspection charm
        being deployed to the `machine_id.)

        """
        self._active_collector = None
        self._noop_collector = None
        self._active = active

        self._cpu_profile_path = os.path.join(logs_dir, 'cpu_profile')
        os.makedirs(self._cpu_profile_path)
        self._heap_profile_path = os.path.join(logs_dir, 'heap_profile')
        os.makedirs(self._heap_profile_path)
        self._goroutines_path = os.path.join(logs_dir, 'goroutines_profile')
        os.makedirs(self._goroutines_path)

        # Store in case we need to activate a collector at a later date.
        self._client = client
        self._machine_id = machine_id

        if self._active:
            self.set_active()
        else:
            self.unset_active()

    def set_active(self):
        if self._active_collector is None:
            self._active_collector = ActiveCollector(
                self._client, self._machine_id)
        self._collector = self._active_collector

    def unset_active(self):
        if self._noop_collector is None:
            self._noop_collector = NoopCollector(
                self._client, self._machine_id)
        self._collector = self._noop_collector

    def _get_profile_file_path(self, dir_path):
        """Given a directory create a timestamped file path."""
        ts_file = datetime.utcnow().strftime(self.FILE_TIMESTAMP)
        return os.path.join(dir_path, '{}.pprof'.format(ts_file))

    def collect_profile(self, seconds=5):
        """Collect `seconds` worth of CPU profile."""
        self._collector.collect_profile(
            self._get_profile_file_path(self._cpu_profile_path),
            seconds)

    def collect_heap(self, seconds=5):
        """Collect `seconds` worth of heap profile."""
        self._collector.collect_heap(
            self._get_profile_file_path(self._heap_profile_path),
            seconds)

    def collect_goroutines(self, seconds=5):
        """Collect `seconds` worth of goroutines profile."""
        self._collector.collect_goroutines(
            self._get_profile_file_path(self._goroutines_path),
            seconds)
