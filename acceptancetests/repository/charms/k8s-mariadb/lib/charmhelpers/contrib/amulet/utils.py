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

import io
import json
import logging
import os
import re
import socket
import subprocess
import sys
import time
import uuid

import amulet
import distro_info
import six
from six.moves import configparser
if six.PY3:
    from urllib import parse as urlparse
else:
    import urlparse


class AmuletUtils(object):
    """Amulet utilities.

       This class provides common utility functions that are used by Amulet
       tests.
       """

    def __init__(self, log_level=logging.ERROR):
        self.log = self.get_logger(level=log_level)
        self.ubuntu_releases = self.get_ubuntu_releases()

    def get_logger(self, name="amulet-logger", level=logging.DEBUG):
        """Get a logger object that will log to stdout."""
        log = logging
        logger = log.getLogger(name)
        fmt = log.Formatter("%(asctime)s %(funcName)s "
                            "%(levelname)s: %(message)s")

        handler = log.StreamHandler(stream=sys.stdout)
        handler.setLevel(level)
        handler.setFormatter(fmt)

        logger.addHandler(handler)
        logger.setLevel(level)

        return logger

    def valid_ip(self, ip):
        if re.match(r"^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$", ip):
            return True
        else:
            return False

    def valid_url(self, url):
        p = re.compile(
            r'^(?:http|ftp)s?://'
            r'(?:(?:[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?\.)+(?:[A-Z]{2,6}\.?|[A-Z0-9-]{2,}\.?)|'  # noqa
            r'localhost|'
            r'\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})'
            r'(?::\d+)?'
            r'(?:/?|[/?]\S+)$',
            re.IGNORECASE)
        if p.match(url):
            return True
        else:
            return False

    def get_ubuntu_release_from_sentry(self, sentry_unit):
        """Get Ubuntu release codename from sentry unit.

        :param sentry_unit: amulet sentry/service unit pointer
        :returns: list of strings - release codename, failure message
        """
        msg = None
        cmd = 'lsb_release -cs'
        release, code = sentry_unit.run(cmd)
        if code == 0:
            self.log.debug('{} lsb_release: {}'.format(
                sentry_unit.info['unit_name'], release))
        else:
            msg = ('{} `{}` returned {} '
                   '{}'.format(sentry_unit.info['unit_name'],
                               cmd, release, code))
        if release not in self.ubuntu_releases:
            msg = ("Release ({}) not found in Ubuntu releases "
                   "({})".format(release, self.ubuntu_releases))
        return release, msg

    def validate_services(self, commands):
        """Validate that lists of commands succeed on service units.  Can be
           used to verify system services are running on the corresponding
           service units.

        :param commands: dict with sentry keys and arbitrary command list vals
        :returns: None if successful, Failure string message otherwise
        """
        self.log.debug('Checking status of system services...')

        # /!\ DEPRECATION WARNING (beisner):
        # New and existing tests should be rewritten to use
        # validate_services_by_name() as it is aware of init systems.
        self.log.warn('DEPRECATION WARNING:  use '
                      'validate_services_by_name instead of validate_services '
                      'due to init system differences.')

        for k, v in six.iteritems(commands):
            for cmd in v:
                output, code = k.run(cmd)
                self.log.debug('{} `{}` returned '
                               '{}'.format(k.info['unit_name'],
                                           cmd, code))
                if code != 0:
                    return "command `{}` returned {}".format(cmd, str(code))
        return None

    def validate_services_by_name(self, sentry_services):
        """Validate system service status by service name, automatically
           detecting init system based on Ubuntu release codename.

        :param sentry_services: dict with sentry keys and svc list values
        :returns: None if successful, Failure string message otherwise
        """
        self.log.debug('Checking status of system services...')

        # Point at which systemd became a thing
        systemd_switch = self.ubuntu_releases.index('vivid')

        for sentry_unit, services_list in six.iteritems(sentry_services):
            # Get lsb_release codename from unit
            release, ret = self.get_ubuntu_release_from_sentry(sentry_unit)
            if ret:
                return ret

            for service_name in services_list:
                if (self.ubuntu_releases.index(release) >= systemd_switch or
                        service_name in ['rabbitmq-server', 'apache2',
                                         'memcached']):
                    # init is systemd (or regular sysv)
                    cmd = 'sudo service {} status'.format(service_name)
                    output, code = sentry_unit.run(cmd)
                    service_running = code == 0
                elif self.ubuntu_releases.index(release) < systemd_switch:
                    # init is upstart
                    cmd = 'sudo status {}'.format(service_name)
                    output, code = sentry_unit.run(cmd)
                    service_running = code == 0 and "start/running" in output

                self.log.debug('{} `{}` returned '
                               '{}'.format(sentry_unit.info['unit_name'],
                                           cmd, code))
                if not service_running:
                    return u"command `{}` returned {} {}".format(
                        cmd, output, str(code))
        return None

    def _get_config(self, unit, filename):
        """Get a ConfigParser object for parsing a unit's config file."""
        file_contents = unit.file_contents(filename)

        # NOTE(beisner):  by default, ConfigParser does not handle options
        # with no value, such as the flags used in the mysql my.cnf file.
        # https://bugs.python.org/issue7005
        config = configparser.ConfigParser(allow_no_value=True)
        config.readfp(io.StringIO(file_contents))
        return config

    def validate_config_data(self, sentry_unit, config_file, section,
                             expected):
        """Validate config file data.

           Verify that the specified section of the config file contains
           the expected option key:value pairs.

           Compare expected dictionary data vs actual dictionary data.
           The values in the 'expected' dictionary can be strings, bools, ints,
           longs, or can be a function that evaluates a variable and returns a
           bool.
           """
        self.log.debug('Validating config file data ({} in {} on {})'
                       '...'.format(section, config_file,
                                    sentry_unit.info['unit_name']))
        config = self._get_config(sentry_unit, config_file)

        if section != 'DEFAULT' and not config.has_section(section):
            return "section [{}] does not exist".format(section)

        for k in expected.keys():
            if not config.has_option(section, k):
                return "section [{}] is missing option {}".format(section, k)

            actual = config.get(section, k)
            v = expected[k]
            if (isinstance(v, six.string_types) or
                    isinstance(v, bool) or
                    isinstance(v, six.integer_types)):
                # handle explicit values
                if actual != v:
                    return "section [{}] {}:{} != expected {}:{}".format(
                           section, k, actual, k, expected[k])
            # handle function pointers, such as not_null or valid_ip
            elif not v(actual):
                return "section [{}] {}:{} != expected {}:{}".format(
                       section, k, actual, k, expected[k])
        return None

    def _validate_dict_data(self, expected, actual):
        """Validate dictionary data.

           Compare expected dictionary data vs actual dictionary data.
           The values in the 'expected' dictionary can be strings, bools, ints,
           longs, or can be a function that evaluates a variable and returns a
           bool.
           """
        self.log.debug('actual: {}'.format(repr(actual)))
        self.log.debug('expected: {}'.format(repr(expected)))

        for k, v in six.iteritems(expected):
            if k in actual:
                if (isinstance(v, six.string_types) or
                        isinstance(v, bool) or
                        isinstance(v, six.integer_types)):
                    # handle explicit values
                    if v != actual[k]:
                        return "{}:{}".format(k, actual[k])
                # handle function pointers, such as not_null or valid_ip
                elif not v(actual[k]):
                    return "{}:{}".format(k, actual[k])
            else:
                return "key '{}' does not exist".format(k)
        return None

    def validate_relation_data(self, sentry_unit, relation, expected):
        """Validate actual relation data based on expected relation data."""
        actual = sentry_unit.relation(relation[0], relation[1])
        return self._validate_dict_data(expected, actual)

    def _validate_list_data(self, expected, actual):
        """Compare expected list vs actual list data."""
        for e in expected:
            if e not in actual:
                return "expected item {} not found in actual list".format(e)
        return None

    def not_null(self, string):
        if string is not None:
            return True
        else:
            return False

    def _get_file_mtime(self, sentry_unit, filename):
        """Get last modification time of file."""
        return sentry_unit.file_stat(filename)['mtime']

    def _get_dir_mtime(self, sentry_unit, directory):
        """Get last modification time of directory."""
        return sentry_unit.directory_stat(directory)['mtime']

    def _get_proc_start_time(self, sentry_unit, service, pgrep_full=None):
        """Get start time of a process based on the last modification time
           of the /proc/pid directory.

        :sentry_unit:  The sentry unit to check for the service on
        :service:  service name to look for in process table
        :pgrep_full:  [Deprecated] Use full command line search mode with pgrep
        :returns:  epoch time of service process start
        :param commands:  list of bash commands
        :param sentry_units:  list of sentry unit pointers
        :returns:  None if successful; Failure message otherwise
        """
        if pgrep_full is not None:
            # /!\ DEPRECATION WARNING (beisner):
            # No longer implemented, as pidof is now used instead of pgrep.
            # https://bugs.launchpad.net/charm-helpers/+bug/1474030
            self.log.warn('DEPRECATION WARNING:  pgrep_full bool is no '
                          'longer implemented re: lp 1474030.')

        pid_list = self.get_process_id_list(sentry_unit, service)
        pid = pid_list[0]
        proc_dir = '/proc/{}'.format(pid)
        self.log.debug('Pid for {} on {}: {}'.format(
            service, sentry_unit.info['unit_name'], pid))

        return self._get_dir_mtime(sentry_unit, proc_dir)

    def service_restarted(self, sentry_unit, service, filename,
                          pgrep_full=None, sleep_time=20):
        """Check if service was restarted.

           Compare a service's start time vs a file's last modification time
           (such as a config file for that service) to determine if the service
           has been restarted.
           """
        # /!\ DEPRECATION WARNING (beisner):
        # This method is prone to races in that no before-time is known.
        # Use validate_service_config_changed instead.

        # NOTE(beisner) pgrep_full is no longer implemented, as pidof is now
        # used instead of pgrep.  pgrep_full is still passed through to ensure
        # deprecation WARNS.  lp1474030
        self.log.warn('DEPRECATION WARNING:  use '
                      'validate_service_config_changed instead of '
                      'service_restarted due to known races.')

        time.sleep(sleep_time)
        if (self._get_proc_start_time(sentry_unit, service, pgrep_full) >=
                self._get_file_mtime(sentry_unit, filename)):
            return True
        else:
            return False

    def service_restarted_since(self, sentry_unit, mtime, service,
                                pgrep_full=None, sleep_time=20,
                                retry_count=30, retry_sleep_time=10):
        """Check if service was been started after a given time.

        Args:
          sentry_unit (sentry): The sentry unit to check for the service on
          mtime (float): The epoch time to check against
          service (string): service name to look for in process table
          pgrep_full: [Deprecated] Use full command line search mode with pgrep
          sleep_time (int): Initial sleep time (s) before looking for file
          retry_sleep_time (int): Time (s) to sleep between retries
          retry_count (int): If file is not found, how many times to retry

        Returns:
          bool: True if service found and its start time it newer than mtime,
                False if service is older than mtime or if service was
                not found.
        """
        # NOTE(beisner) pgrep_full is no longer implemented, as pidof is now
        # used instead of pgrep.  pgrep_full is still passed through to ensure
        # deprecation WARNS.  lp1474030

        unit_name = sentry_unit.info['unit_name']
        self.log.debug('Checking that %s service restarted since %s on '
                       '%s' % (service, mtime, unit_name))
        time.sleep(sleep_time)
        proc_start_time = None
        tries = 0
        while tries <= retry_count and not proc_start_time:
            try:
                proc_start_time = self._get_proc_start_time(sentry_unit,
                                                            service,
                                                            pgrep_full)
                self.log.debug('Attempt {} to get {} proc start time on {} '
                               'OK'.format(tries, service, unit_name))
            except IOError as e:
                # NOTE(beisner) - race avoidance, proc may not exist yet.
                # https://bugs.launchpad.net/charm-helpers/+bug/1474030
                self.log.debug('Attempt {} to get {} proc start time on {} '
                               'failed\n{}'.format(tries, service,
                                                   unit_name, e))
                time.sleep(retry_sleep_time)
                tries += 1

        if not proc_start_time:
            self.log.warn('No proc start time found, assuming service did '
                          'not start')
            return False
        if proc_start_time >= mtime:
            self.log.debug('Proc start time is newer than provided mtime'
                           '(%s >= %s) on %s (OK)' % (proc_start_time,
                                                      mtime, unit_name))
            return True
        else:
            self.log.warn('Proc start time (%s) is older than provided mtime '
                          '(%s) on %s, service did not '
                          'restart' % (proc_start_time, mtime, unit_name))
            return False

    def config_updated_since(self, sentry_unit, filename, mtime,
                             sleep_time=20, retry_count=30,
                             retry_sleep_time=10):
        """Check if file was modified after a given time.

        Args:
          sentry_unit (sentry): The sentry unit to check the file mtime on
          filename (string): The file to check mtime of
          mtime (float): The epoch time to check against
          sleep_time (int): Initial sleep time (s) before looking for file
          retry_sleep_time (int): Time (s) to sleep between retries
          retry_count (int): If file is not found, how many times to retry

        Returns:
          bool: True if file was modified more recently than mtime, False if
                file was modified before mtime, or if file not found.
        """
        unit_name = sentry_unit.info['unit_name']
        self.log.debug('Checking that %s updated since %s on '
                       '%s' % (filename, mtime, unit_name))
        time.sleep(sleep_time)
        file_mtime = None
        tries = 0
        while tries <= retry_count and not file_mtime:
            try:
                file_mtime = self._get_file_mtime(sentry_unit, filename)
                self.log.debug('Attempt {} to get {} file mtime on {} '
                               'OK'.format(tries, filename, unit_name))
            except IOError as e:
                # NOTE(beisner) - race avoidance, file may not exist yet.
                # https://bugs.launchpad.net/charm-helpers/+bug/1474030
                self.log.debug('Attempt {} to get {} file mtime on {} '
                               'failed\n{}'.format(tries, filename,
                                                   unit_name, e))
                time.sleep(retry_sleep_time)
                tries += 1

        if not file_mtime:
            self.log.warn('Could not determine file mtime, assuming '
                          'file does not exist')
            return False

        if file_mtime >= mtime:
            self.log.debug('File mtime is newer than provided mtime '
                           '(%s >= %s) on %s (OK)' % (file_mtime,
                                                      mtime, unit_name))
            return True
        else:
            self.log.warn('File mtime is older than provided mtime'
                          '(%s < on %s) on %s' % (file_mtime,
                                                  mtime, unit_name))
            return False

    def validate_service_config_changed(self, sentry_unit, mtime, service,
                                        filename, pgrep_full=None,
                                        sleep_time=20, retry_count=30,
                                        retry_sleep_time=10):
        """Check service and file were updated after mtime

        Args:
          sentry_unit (sentry): The sentry unit to check for the service on
          mtime (float): The epoch time to check against
          service (string): service name to look for in process table
          filename (string): The file to check mtime of
          pgrep_full: [Deprecated] Use full command line search mode with pgrep
          sleep_time (int): Initial sleep in seconds to pass to test helpers
          retry_count (int): If service is not found, how many times to retry
          retry_sleep_time (int): Time in seconds to wait between retries

        Typical Usage:
            u = OpenStackAmuletUtils(ERROR)
            ...
            mtime = u.get_sentry_time(self.cinder_sentry)
            self.d.configure('cinder', {'verbose': 'True', 'debug': 'True'})
            if not u.validate_service_config_changed(self.cinder_sentry,
                                                     mtime,
                                                     'cinder-api',
                                                     '/etc/cinder/cinder.conf')
                amulet.raise_status(amulet.FAIL, msg='update failed')
        Returns:
          bool: True if both service and file where updated/restarted after
                mtime, False if service is older than mtime or if service was
                not found or if filename was modified before mtime.
        """

        # NOTE(beisner) pgrep_full is no longer implemented, as pidof is now
        # used instead of pgrep.  pgrep_full is still passed through to ensure
        # deprecation WARNS.  lp1474030

        service_restart = self.service_restarted_since(
            sentry_unit, mtime,
            service,
            pgrep_full=pgrep_full,
            sleep_time=sleep_time,
            retry_count=retry_count,
            retry_sleep_time=retry_sleep_time)

        config_update = self.config_updated_since(
            sentry_unit,
            filename,
            mtime,
            sleep_time=sleep_time,
            retry_count=retry_count,
            retry_sleep_time=retry_sleep_time)

        return service_restart and config_update

    def get_sentry_time(self, sentry_unit):
        """Return current epoch time on a sentry"""
        cmd = "date +'%s'"
        return float(sentry_unit.run(cmd)[0])

    def relation_error(self, name, data):
        return 'unexpected relation data in {} - {}'.format(name, data)

    def endpoint_error(self, name, data):
        return 'unexpected endpoint data in {} - {}'.format(name, data)

    def get_ubuntu_releases(self):
        """Return a list of all Ubuntu releases in order of release."""
        _d = distro_info.UbuntuDistroInfo()
        _release_list = _d.all
        return _release_list

    def file_to_url(self, file_rel_path):
        """Convert a relative file path to a file URL."""
        _abs_path = os.path.abspath(file_rel_path)
        return urlparse.urlparse(_abs_path, scheme='file').geturl()

    def check_commands_on_units(self, commands, sentry_units):
        """Check that all commands in a list exit zero on all
        sentry units in a list.

        :param commands:  list of bash commands
        :param sentry_units:  list of sentry unit pointers
        :returns: None if successful; Failure message otherwise
        """
        self.log.debug('Checking exit codes for {} commands on {} '
                       'sentry units...'.format(len(commands),
                                                len(sentry_units)))
        for sentry_unit in sentry_units:
            for cmd in commands:
                output, code = sentry_unit.run(cmd)
                if code == 0:
                    self.log.debug('{} `{}` returned {} '
                                   '(OK)'.format(sentry_unit.info['unit_name'],
                                                 cmd, code))
                else:
                    return ('{} `{}` returned {} '
                            '{}'.format(sentry_unit.info['unit_name'],
                                        cmd, code, output))
        return None

    def get_process_id_list(self, sentry_unit, process_name,
                            expect_success=True):
        """Get a list of process ID(s) from a single sentry juju unit
        for a single process name.

        :param sentry_unit: Amulet sentry instance (juju unit)
        :param process_name: Process name
        :param expect_success: If False, expect the PID to be missing,
            raise if it is present.
        :returns: List of process IDs
        """
        cmd = 'pidof -x "{}"'.format(process_name)
        if not expect_success:
            cmd += " || exit 0 && exit 1"
        output, code = sentry_unit.run(cmd)
        if code != 0:
            msg = ('{} `{}` returned {} '
                   '{}'.format(sentry_unit.info['unit_name'],
                               cmd, code, output))
            amulet.raise_status(amulet.FAIL, msg=msg)
        return str(output).split()

    def get_unit_process_ids(self, unit_processes, expect_success=True):
        """Construct a dict containing unit sentries, process names, and
        process IDs.

        :param unit_processes: A dictionary of Amulet sentry instance
            to list of process names.
        :param expect_success: if False expect the processes to not be
            running, raise if they are.
        :returns: Dictionary of Amulet sentry instance to dictionary
            of process names to PIDs.
        """
        pid_dict = {}
        for sentry_unit, process_list in six.iteritems(unit_processes):
            pid_dict[sentry_unit] = {}
            for process in process_list:
                pids = self.get_process_id_list(
                    sentry_unit, process, expect_success=expect_success)
                pid_dict[sentry_unit].update({process: pids})
        return pid_dict

    def validate_unit_process_ids(self, expected, actual):
        """Validate process id quantities for services on units."""
        self.log.debug('Checking units for running processes...')
        self.log.debug('Expected PIDs: {}'.format(expected))
        self.log.debug('Actual PIDs: {}'.format(actual))

        if len(actual) != len(expected):
            return ('Unit count mismatch.  expected, actual: {}, '
                    '{} '.format(len(expected), len(actual)))

        for (e_sentry, e_proc_names) in six.iteritems(expected):
            e_sentry_name = e_sentry.info['unit_name']
            if e_sentry in actual.keys():
                a_proc_names = actual[e_sentry]
            else:
                return ('Expected sentry ({}) not found in actual dict data.'
                        '{}'.format(e_sentry_name, e_sentry))

            if len(e_proc_names.keys()) != len(a_proc_names.keys()):
                return ('Process name count mismatch.  expected, actual: {}, '
                        '{}'.format(len(expected), len(actual)))

            for (e_proc_name, e_pids), (a_proc_name, a_pids) in \
                    zip(e_proc_names.items(), a_proc_names.items()):
                if e_proc_name != a_proc_name:
                    return ('Process name mismatch.  expected, actual: {}, '
                            '{}'.format(e_proc_name, a_proc_name))

                a_pids_length = len(a_pids)
                fail_msg = ('PID count mismatch. {} ({}) expected, actual: '
                            '{}, {} ({})'.format(e_sentry_name, e_proc_name,
                                                 e_pids, a_pids_length,
                                                 a_pids))

                # If expected is a list, ensure at least one PID quantity match
                if isinstance(e_pids, list) and \
                        a_pids_length not in e_pids:
                    return fail_msg
                # If expected is not bool and not list,
                # ensure PID quantities match
                elif not isinstance(e_pids, bool) and \
                        not isinstance(e_pids, list) and \
                        a_pids_length != e_pids:
                    return fail_msg
                # If expected is bool True, ensure 1 or more PIDs exist
                elif isinstance(e_pids, bool) and \
                        e_pids is True and a_pids_length < 1:
                    return fail_msg
                # If expected is bool False, ensure 0 PIDs exist
                elif isinstance(e_pids, bool) and \
                        e_pids is False and a_pids_length != 0:
                    return fail_msg
                else:
                    self.log.debug('PID check OK: {} {} {}: '
                                   '{}'.format(e_sentry_name, e_proc_name,
                                               e_pids, a_pids))
        return None

    def validate_list_of_identical_dicts(self, list_of_dicts):
        """Check that all dicts within a list are identical."""
        hashes = []
        for _dict in list_of_dicts:
            hashes.append(hash(frozenset(_dict.items())))

        self.log.debug('Hashes: {}'.format(hashes))
        if len(set(hashes)) == 1:
            self.log.debug('Dicts within list are identical')
        else:
            return 'Dicts within list are not identical'

        return None

    def validate_sectionless_conf(self, file_contents, expected):
        """A crude conf parser.  Useful to inspect configuration files which
        do not have section headers (as would be necessary in order to use
        the configparser).  Such as openstack-dashboard or rabbitmq confs."""
        for line in file_contents.split('\n'):
            if '=' in line:
                args = line.split('=')
                if len(args) <= 1:
                    continue
                key = args[0].strip()
                value = args[1].strip()
                if key in expected.keys():
                    if expected[key] != value:
                        msg = ('Config mismatch.  Expected, actual:  {}, '
                               '{}'.format(expected[key], value))
                        amulet.raise_status(amulet.FAIL, msg=msg)

    def get_unit_hostnames(self, units):
        """Return a dict of juju unit names to hostnames."""
        host_names = {}
        for unit in units:
            host_names[unit.info['unit_name']] = \
                str(unit.file_contents('/etc/hostname').strip())
        self.log.debug('Unit host names: {}'.format(host_names))
        return host_names

    def run_cmd_unit(self, sentry_unit, cmd):
        """Run a command on a unit, return the output and exit code."""
        output, code = sentry_unit.run(cmd)
        if code == 0:
            self.log.debug('{} `{}` command returned {} '
                           '(OK)'.format(sentry_unit.info['unit_name'],
                                         cmd, code))
        else:
            msg = ('{} `{}` command returned {} '
                   '{}'.format(sentry_unit.info['unit_name'],
                               cmd, code, output))
            amulet.raise_status(amulet.FAIL, msg=msg)
        return str(output), code

    def file_exists_on_unit(self, sentry_unit, file_name):
        """Check if a file exists on a unit."""
        try:
            sentry_unit.file_stat(file_name)
            return True
        except IOError:
            return False
        except Exception as e:
            msg = 'Error checking file {}: {}'.format(file_name, e)
            amulet.raise_status(amulet.FAIL, msg=msg)

    def file_contents_safe(self, sentry_unit, file_name,
                           max_wait=60, fatal=False):
        """Get file contents from a sentry unit.  Wrap amulet file_contents
        with retry logic to address races where a file checks as existing,
        but no longer exists by the time file_contents is called.
        Return None if file not found. Optionally raise if fatal is True."""
        unit_name = sentry_unit.info['unit_name']
        file_contents = False
        tries = 0
        while not file_contents and tries < (max_wait / 4):
            try:
                file_contents = sentry_unit.file_contents(file_name)
            except IOError:
                self.log.debug('Attempt {} to open file {} from {} '
                               'failed'.format(tries, file_name,
                                               unit_name))
                time.sleep(4)
                tries += 1

        if file_contents:
            return file_contents
        elif not fatal:
            return None
        elif fatal:
            msg = 'Failed to get file contents from unit.'
            amulet.raise_status(amulet.FAIL, msg)

    def port_knock_tcp(self, host="localhost", port=22, timeout=15):
        """Open a TCP socket to check for a listening sevice on a host.

        :param host: host name or IP address, default to localhost
        :param port: TCP port number, default to 22
        :param timeout: Connect timeout, default to 15 seconds
        :returns: True if successful, False if connect failed
        """

        # Resolve host name if possible
        try:
            connect_host = socket.gethostbyname(host)
            host_human = "{} ({})".format(connect_host, host)
        except socket.error as e:
            self.log.warn('Unable to resolve address: '
                          '{} ({}) Trying anyway!'.format(host, e))
            connect_host = host
            host_human = connect_host

        # Attempt socket connection
        try:
            knock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            knock.settimeout(timeout)
            knock.connect((connect_host, port))
            knock.close()
            self.log.debug('Socket connect OK for host '
                           '{} on port {}.'.format(host_human, port))
            return True
        except socket.error as e:
            self.log.debug('Socket connect FAIL for'
                           ' {} port {} ({})'.format(host_human, port, e))
            return False

    def port_knock_units(self, sentry_units, port=22,
                         timeout=15, expect_success=True):
        """Open a TCP socket to check for a listening sevice on each
        listed juju unit.

        :param sentry_units: list of sentry unit pointers
        :param port: TCP port number, default to 22
        :param timeout: Connect timeout, default to 15 seconds
        :expect_success: True by default, set False to invert logic
        :returns: None if successful, Failure message otherwise
        """
        for unit in sentry_units:
            host = unit.info['public-address']
            connected = self.port_knock_tcp(host, port, timeout)
            if not connected and expect_success:
                return 'Socket connect failed.'
            elif connected and not expect_success:
                return 'Socket connected unexpectedly.'

    def get_uuid_epoch_stamp(self):
        """Returns a stamp string based on uuid4 and epoch time.  Useful in
        generating test messages which need to be unique-ish."""
        return '[{}-{}]'.format(uuid.uuid4(), time.time())

    # amulet juju action helpers:
    def run_action(self, unit_sentry, action,
                   _check_output=subprocess.check_output,
                   params=None):
        """Translate to amulet's built in run_action(). Deprecated.

        Run the named action on a given unit sentry.

        params a dict of parameters to use
        _check_output parameter is no longer used

        @return action_id.
        """
        self.log.warn('charmhelpers.contrib.amulet.utils.run_action has been '
                      'deprecated for amulet.run_action')
        return unit_sentry.run_action(action, action_args=params)

    def wait_on_action(self, action_id, _check_output=subprocess.check_output):
        """Wait for a given action, returning if it completed or not.

        action_id a string action uuid
        _check_output parameter is no longer used
        """
        data = amulet.actions.get_action_output(action_id, full_output=True)
        return data.get(u"status") == "completed"

    def status_get(self, unit):
        """Return the current service status of this unit."""
        raw_status, return_code = unit.run(
            "status-get --format=json --include-data")
        if return_code != 0:
            return ("unknown", "")
        status = json.loads(raw_status)
        return (status["status"], status["message"])
