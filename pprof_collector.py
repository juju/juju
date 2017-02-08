"""Collector of pprof details using juju introspection."""

from __future__ import print_function

from datetime import datetime
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
        self.client = client
        self.machine_id = machine_id

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
        self.machine_id = machine_id
        self.introspection_ip = install_introspection_charm(
            client, machine_id)

    def _collect_profile(self, profile, filepath, seconds):
        profile_url = get_profile_url(
            self.introspection_ip, self.machine_id, profile, seconds)
        get_profile_reading(profile_url, filepath)

    def collect_profile(self, filepath, seconds):
        """Collect `seconds` worth of CPU profile."""
        log.info('Collecting CPU profile data.')
        self._collect_profile('profile', filepath, seconds)

    def collect_heap(self, filepath, seconds):
        """Collect `seconds` worth of heap profile."""
        log.info('Collecting heap profile data.')
        self._collect_profile('heap', filepath, seconds)

    def collect_goroutines(self, filepath, seconds):
        """Collect `seconds` worth of goroutines profile."""
        log.info('Collecting goroutines profile data.')
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

    def __init__(self, client, machine_ids, logs_dir, active=False):
        """Collector of pprof profiles from a machine.

        Defaults to being non-active meaning that any attempt to collect a
        profile will be a no-op.
        Setting active (either at creation or at any point during the lifespan
        of this object) will enable collection of pprof profiles from the
        `machine_id`.
        (Note. first time going active will result in the introspection charm
        being deployed to the `machine_id.)

        :param client: ModelClient to use to communicate with machine_ids.
        :param machine_ids: List of machine IDs to have collections for.
        :param logs_dir: Directory in which to store profile data.
        :param active: Bool indicating wherever to enable collection of data or
          not.

        """
        if not isinstance(machine_ids, list):
            raise ValueError('List of machine IDs required.')

        self._active_collectors = []
        self._noop_collectors = []
        self._active = active

        self._cpu_profile_path = os.path.join(logs_dir, 'cpu_profile')
        os.makedirs(self._cpu_profile_path)
        self._heap_profile_path = os.path.join(logs_dir, 'heap_profile')
        os.makedirs(self._heap_profile_path)
        self._goroutines_path = os.path.join(logs_dir, 'goroutines_profile')
        os.makedirs(self._goroutines_path)

        # Store in case we need to activate a collector at a later date.
        self._client = client
        self._machine_ids = machine_ids

        if self._active:
            self.set_active()
        else:
            self.unset_active()

    def set_active(self):
        log.info('Setting PPROF collection to ACTIVE.')
        if not self._active_collectors:
            for m_id in self._machine_ids:
                self._active_collectors.append(
                    ActiveCollector(self._client, m_id))
        self._collectors = self._active_collectors
        self._active = True

    def unset_active(self):
        log.info('Setting PPROF collection to INACTIVE.')
        if not self._noop_collectors:
            for m_id in self._machine_ids:
                self._noop_collectors.append(
                    NoopCollector(self._client, m_id))
        self._collectors = self._noop_collectors
        self._active = False

    def _get_profile_file_path(self, dir_path, machine_id):
        """Given a directory create a timestamped file path."""
        ts_file = datetime.utcnow().strftime(self.FILE_TIMESTAMP)
        return os.path.join(
            dir_path,
            'machine-{}-{}.pprof'.format(
                machine_id,
                ts_file))

    def collect_profile(self, seconds=5):
        """Collect `seconds` worth of CPU profile."""
        for collector in self._collectors:
            collector.collect_profile(
                self._get_profile_file_path(
                    self._cpu_profile_path,
                    collector.machine_id,
                ),
                seconds
            )

    def collect_heap(self, seconds=5):
        """Collect `seconds` worth of heap profile."""
        for collector in self._collectors:
            collector.collect_heap(
                self._get_profile_file_path(
                    self._heap_profile_path,
                    collector.machine_id,
                ),
                seconds
            )

    def collect_goroutines(self, seconds=5):
        """Collect `seconds` worth of goroutines profile."""
        for collector in self._collectors:
            collector.collect_goroutines(
                self._get_profile_file_path(
                    self._goroutines_path,
                    collector.machine_id,
                ),
                seconds
            )
