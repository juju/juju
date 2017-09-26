from argparse import (
    Namespace,
    )
from contextlib import contextmanager
from datetime import (
    datetime,
    timedelta,
    )
import json
import logging
import os
import subprocess
import sys
from unittest import (
    skipIf,
    TestCase,
    )

from mock import (
    call,
    MagicMock,
    patch,
    Mock,
    )
import yaml

from deploy_stack import (
    archive_logs,
    assess_juju_relations,
    assess_juju_run,
    assess_upgrade,
    boot_context,
    BootstrapManager,
    check_token,
    copy_local_logs,
    copy_remote_logs,
    CreateController,
    ExistingController,
    deploy_dummy_stack,
    deploy_job,
    _deploy_job,
    deploy_job_parse_args,
    dump_env_logs,
    dump_juju_timings,
    _get_clients_to_upgrade,
    iter_remote_machines,
    get_remote_machines,
    make_controller_strategy,
    PublicController,
    safe_print_status,
    retain_config,
    update_env,
    wait_for_state_server_to_shutdown,
    error_if_unclean,
    )
from jujupy import (
    fake_juju_client,
    fake_juju_client_optional_jes,
    get_cache_path,
    get_juju_home,
    get_timeout_prefix,
    JujuData,
    KILL_CONTROLLER,
    ModelClient,
    SimpleEnvironment,
    SoftDeadlineExceeded,
    Status,
    Machine,
    )

from jujupy.client import (
    CommandTime,
)
from jujupy.configuration import (
    get_environments_path,
    get_jenv_path,
    )
from remote import (
    _Remote,
    remote_from_address,
    SSHRemote,
    winrm,
    )
from tests import (
    assert_juju_call,
    FakeHomeTestCase,
    FakePopen,
    make_fake_juju_return,
    observable_temp_file,
    temp_os_env,
    use_context,
    )
from utility import (
    LoggedException,
    temp_dir,
    )


def make_logs(log_dir):
    def write_dumped_files(*args):
        with open(os.path.join(log_dir, 'cloud.log'), 'w') as l:
            l.write('fake log')
        with open(os.path.join(log_dir, 'extra'), 'w') as l:
            l.write('not compressed')
    return write_dumped_files


class DeployStackTestCase(FakeHomeTestCase):

    log_level = logging.DEBUG

    def test_assess_juju_run(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = ModelClient(env, None, None)
        response_ok = json.dumps(
            [{"MachineId": "1", "Stdout": "Linux\n"},
             {"MachineId": "2", "Stdout": "Linux\n"}])
        response_err = json.dumps([
            {"MachineId": "1", "Stdout": "Linux\n"},
            {"MachineId": "2",
             "Stdout": "Linux\n",
             "ReturnCode": 255,
             "Stderr": "Permission denied (publickey,password)"}])
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=response_ok) as gjo_mock:
            responses = assess_juju_run(client)
            for machine in responses:
                self.assertFalse(machine.get('ReturnCode', False))
                self.assertIn(machine.get('MachineId'), ["1", "2"])
            self.assertEqual(len(responses), 2)
        gjo_mock.assert_called_once_with(
            'run', '--format', 'json', '--application',
            'dummy-source,dummy-sink', 'uname')
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=response_err) as gjo_mock:
            with self.assertRaises(ValueError):
                responses = assess_juju_run(client)
        gjo_mock.assert_called_once_with(
            'run', '--format', 'json', '--application',
            'dummy-source,dummy-sink', 'uname')

    def test_safe_print_status(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = ModelClient(env, None, None)
        error = subprocess.CalledProcessError(1, 'status', 'status error')
        with patch.object(client, 'juju', autospec=True,
                          side_effect=[error]) as mock:
            with patch.object(client, 'iter_model_clients',
                              return_value=[client]) as imc_mock:
                safe_print_status(client)
        mock.assert_called_once_with('show-status', ('--format', 'yaml'))
        imc_mock.assert_called_once_with()

    def test_safe_print_status_ignores_soft_deadline(self):
        client = fake_juju_client()
        client._backend._past_deadline = True
        client.bootstrap()

        def raise_exception(e):
            raise e

        try:
            with patch('logging.exception', side_effect=raise_exception):
                safe_print_status(client)
        except SoftDeadlineExceeded:
            self.fail('Raised SoftDeadlineExceeded.')

    def test_update_env(self):
        env = SimpleEnvironment('foo', {'type': 'paas'})
        update_env(
            env, 'bar', series='wacky', bootstrap_host='baz',
            agent_url='url', agent_stream='devel')
        self.assertEqual('bar', env.environment)
        self.assertEqual('bar', env.get_option('name'))
        self.assertEqual('wacky', env.get_option('default-series'))
        self.assertEqual('baz', env.get_option('bootstrap-host'))
        self.assertEqual('url', env.get_option('tools-metadata-url'))
        self.assertEqual('devel', env.get_option('agent-stream'))
        self.assertNotIn('region', env._config)

    def test_update_env_region(self):
        env = SimpleEnvironment('foo', {'type': 'paas'})
        update_env(env, 'bar', region='region-foo')
        self.assertEqual('region-foo', env.get_region())

    def test_update_env_region_none(self):
        env = SimpleEnvironment('foo',
                                {'type': 'paas', 'region': 'region-foo'})
        update_env(env, 'bar', region=None)
        self.assertEqual('region-foo', env.get_region())

    def test_dump_juju_timings(self):
        first_start = datetime(2017, 3, 22, 23, 36, 52, 0)
        first_end = first_start + timedelta(seconds=2)
        second_start = datetime(2017, 3, 22, 23, 40, 51, 0)
        env = JujuData('foo', {'type': 'bar'})
        client = ModelClient(env, None, None)
        client._backend.juju_timings.extend([
            CommandTime('command1', ['command1', 'arg1'], start=first_start),
            CommandTime(
                'command2', ['command2', 'arg1', 'arg2'], start=second_start)
        ])
        client._backend.juju_timings[0].actual_completion(end=first_end)
        expected = [
            {
                'command': 'command1',
                'full_args': ['command1', 'arg1'],
                'start': first_start,
                'end': first_end,
                'total_seconds': 2,
            },
            {
                'command': 'command2',
                'full_args': ['command2', 'arg1', 'arg2'],
                'start': second_start,
                'end': None,
                'total_seconds': None,
            }
        ]
        with temp_dir() as fake_dir:
            dump_juju_timings(client, fake_dir)
            with open(os.path.join(fake_dir,
                      'juju_command_times.yaml')) as out_file:
                file_data = yaml.load(out_file)
        self.assertEqual(file_data, expected)

    def test_check_token(self):
        env = JujuData('foo', {'type': 'local'})
        client = ModelClient(env, None, None)
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    workload-status:
                      current: active
                      message: Token is token

            """)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'cat', autospec=True,
                              return_value='token') as rc_mock:
                with patch.object(client, 'get_status', autospec=True,
                                  return_value=status):
                    check_token(client, 'token', timeout=0)
        rc_mock.assert_called_once_with('/var/run/dummy-sink/token')
        self.assertTrue(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.',
             "INFO Token matches expected 'token'"],
            self.log_stream.getvalue().splitlines())

    def test_check_token_not_found(self):
        env = JujuData('foo', {'type': 'local'})
        client = ModelClient(env, None, None)
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    workload-status:
                      current: active
                      message: Waiting for token

            """)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        error = subprocess.CalledProcessError(1, 'ssh', '')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'cat', autospec=True,
                              side_effect=error) as rc_mock:
                with patch.object(remote, 'get_address',
                                  autospec=True) as ga_mock:
                    with patch.object(client, 'get_status', autospec=True,
                                      return_value=status):
                        with self.assertRaisesRegexp(ValueError,
                                                     "Token is ''"):
                            check_token(client, 'token', timeout=0)
        self.assertEqual(2, rc_mock.call_count)
        ga_mock.assert_called_once_with()
        self.assertFalse(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.'],
            self.log_stream.getvalue().splitlines())

    def test_check_token_not_found_juju_ssh_broken(self):
        env = JujuData('foo', {'type': 'local'})
        client = ModelClient(env, None, None)
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    workload-status:
                      current: active
                      message: Token is token

            """)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        error = subprocess.CalledProcessError(1, 'ssh', '')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'cat', autospec=True,
                              side_effect=[error, 'token']) as rc_mock:
                with patch.object(remote, 'get_address',
                                  autospec=True) as ga_mock:
                    with patch.object(client, 'get_status', autospec=True,
                                      return_value=status):
                        with self.assertRaisesRegexp(ValueError,
                                                     "Token is 'token'"):
                            check_token(client, 'token', timeout=0)
        self.assertEqual(2, rc_mock.call_count)
        rc_mock.assert_called_with('/var/run/dummy-sink/token')
        ga_mock.assert_called_once_with()
        self.assertFalse(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.',
             "INFO Token matches expected 'token'",
             'ERROR juju ssh to unit is broken.'],
            self.log_stream.getvalue().splitlines())

    def test_check_token_win_status(self):
        env = JujuData('foo', {'type': 'azure'})
        client = ModelClient(env, None, None)
        remote = MagicMock(spec=['cat', 'is_windows'])
        remote.is_windows.return_value = True
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    workload-status:
                      current: active
                      message: Token is token

            """)
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                check_token(client, 'token', timeout=0)
        # application-status had the token.
        self.assertEqual(0, remote.cat.call_count)
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.',
             "INFO Token matches expected 'token'"],
            self.log_stream.getvalue().splitlines())

    def test_check_token_win_client_status(self):
        env = SimpleEnvironment('foo', {'type': 'ec2'})
        client = ModelClient(env, None, None)
        remote = MagicMock(spec=['cat', 'is_windows'])
        remote.is_windows.return_value = False
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    workload-status:
                      current: active
                      message: Token is token

            """)
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with patch('sys.platform', 'win32'):
                    check_token(client, 'token', timeout=0)
        # application-status had the token.
        self.assertEqual(0, remote.cat.call_count)
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.',
             "INFO Token matches expected 'token'"],
            self.log_stream.getvalue().splitlines())

    def test_check_token_win_remote(self):
        env = JujuData('foo', {'type': 'azure'})
        client = ModelClient(env, None, None)
        remote = MagicMock(spec=['cat', 'is_windows'])
        remote.is_windows.return_value = True
        remote.cat.return_value = 'token'
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    juju-status:
                      current: active
            """)
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                check_token(client, 'token', timeout=0)
        # application-status did not have the token, winrm did.
        remote.cat.assert_called_once_with('%ProgramData%\\dummy-sink\\token')
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.',
             "INFO Token matches expected 'token'"],
            self.log_stream.getvalue().splitlines())

    def test_check_token_win_remote_failure(self):
        env = JujuData('foo', {'type': 'azure'})
        client = ModelClient(env, None, None)
        remote = MagicMock(spec=['cat', 'is_windows'])
        remote.is_windows.return_value = True
        error = winrm.exceptions.WinRMTransportError('a', 'oops')
        remote.cat.side_effect = error
        status = Status.from_text("""\
            applications:
              dummy-sink:
                units:
                  dummy-sink/0:
                    juju-status:
                      current: active
            """)
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with self.assertRaises(type(error)) as ctx:
                    check_token(client, 'token', timeout=0)
        self.assertIs(ctx.exception, error)
        remote.cat.assert_called_once_with('%ProgramData%\\dummy-sink\\token')
        self.assertEqual(
            ['INFO Waiting for applications to reach ready.',
             'INFO Retrieving token.'],
            self.log_stream.getvalue().splitlines())


class DumpEnvLogsTestCase(FakeHomeTestCase):

    log_level = logging.DEBUG

    def assert_machines(self, expected, got):
        self.assertEqual(expected, dict((k, got[k].address) for k in got))

    r0 = remote_from_address('10.10.0.1')
    r1 = remote_from_address('10.10.0.11')
    r2 = remote_from_address('10.10.0.22', series='win2012hvr2')

    @classmethod
    def fake_remote_machines(cls):
        return {'0': cls.r0, '1': cls.r1, '2': cls.r2}

    def test_dump_env_logs_remote(self):
        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_remote_machines', autospec=True,
                       return_value=self.fake_remote_machines()) as gm_mock:
                with patch('deploy_stack._can_run_ssh', lambda: True):
                    with patch('deploy_stack.copy_remote_logs',
                               autospec=True) as crl_mock:
                        with patch('deploy_stack.archive_logs',
                                   autospec=True) as al_mock:
                            env = JujuData('foo', {'type': 'nonlocal'})
                            client = ModelClient(env, '1.234-76', None)
                            dump_env_logs(client, '10.10.0.1', artifacts_dir)
            al_mock.assert_called_once_with(artifacts_dir)
            self.assertEqual(
                ['machine-0', 'machine-1', 'machine-2'],
                sorted(os.listdir(artifacts_dir)))
        self.assertEqual(
            (client, {'0': '10.10.0.1'}), gm_mock.call_args[0])
        self.assertItemsEqual(
            [(self.r0, '%s/machine-0' % artifacts_dir),
             (self.r1, '%s/machine-1' % artifacts_dir),
             (self.r2, '%s/machine-2' % artifacts_dir)],
            [cal[0] for cal in crl_mock.call_args_list])
        self.assertEqual(
            ['INFO Retrieving logs for machine-0 using ' + repr(self.r0),
             'INFO Retrieving logs for machine-1 using ' + repr(self.r1),
             'INFO Retrieving logs for machine-2 using ' + repr(self.r2)],
            self.log_stream.getvalue().splitlines())

    def test_dump_env_logs_remote_no_ssh(self):
        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_remote_machines', autospec=True,
                       return_value=self.fake_remote_machines()) as gm_mock:
                with patch('deploy_stack._can_run_ssh', lambda: False):
                    with patch('deploy_stack.copy_remote_logs',
                               autospec=True) as crl_mock:
                        with patch('deploy_stack.archive_logs',
                                   autospec=True) as al_mock:
                            env = JujuData('foo', {'type': 'nonlocal'})
                            client = ModelClient(env, '1.234-76', None)
                            dump_env_logs(client, '10.10.0.1', artifacts_dir)
            al_mock.assert_called_once_with(artifacts_dir)
            self.assertEqual(
                ['machine-2'],
                sorted(os.listdir(artifacts_dir)))
        self.assertEqual((client, {'0': '10.10.0.1'}), gm_mock.call_args[0])
        self.assertEqual(
            [(self.r2, '%s/machine-2' % artifacts_dir)],
            [cal[0] for cal in crl_mock.call_args_list])
        self.assertEqual(
            ['INFO No ssh, skipping logs for machine-0 using ' + repr(self.r0),
             'INFO No ssh, skipping logs for machine-1 using ' + repr(self.r1),
             'INFO Retrieving logs for machine-2 using ' + repr(self.r2)],
            self.log_stream.getvalue().splitlines())

    def test_dump_env_logs_local_env(self):
        env = JujuData('foo', {'type': 'local'})
        client = ModelClient(env, '1.234-76', None)
        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_remote_machines',
                       autospec=True) as grm_mock:
                with patch('deploy_stack.copy_local_logs',
                           autospec=True) as cll_mock:
                    with patch('deploy_stack.archive_logs',
                               autospec=True) as al_mock:
                        dump_env_logs(client, '10.10.0.1', artifacts_dir)
            cll_mock.assert_called_with(env, artifacts_dir)
            al_mock.assert_called_once_with(artifacts_dir)
        self.assertEqual(grm_mock.call_args_list, [])
        self.assertEqual(
            ['INFO Retrieving logs for local environment'],
            self.log_stream.getvalue().splitlines())

    def test_archive_logs(self):
        with temp_dir() as log_dir:
            with open(os.path.join(log_dir, 'fake.log'), 'w') as f:
                f.write('log contents')
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                archive_logs(log_dir)
            log_path = os.path.join(log_dir, 'fake.log')
            cc_mock.assert_called_once_with(['gzip', '--best', '-f', log_path])

    def test_archive_logs_syslog(self):
        with temp_dir() as log_dir:
            log_path = os.path.join(log_dir, 'syslog')
            with open(log_path, 'w') as f:
                f.write('syslog contents')
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                archive_logs(log_dir)
            cc_mock.assert_called_once_with(['gzip', '--best', '-f', log_path])

    def test_archive_logs_subdir(self):
        with temp_dir() as log_dir:
            subdir = os.path.join(log_dir, "subdir")
            os.mkdir(subdir)
            with open(os.path.join(subdir, 'fake.log'), 'w') as f:
                f.write('log contents')
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                archive_logs(log_dir)
            log_path = os.path.join(subdir, 'fake.log')
            cc_mock.assert_called_once_with(['gzip', '--best', '-f', log_path])

    def test_archive_logs_none(self):
        with temp_dir() as log_dir:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                archive_logs(log_dir)
        self.assertEquals(cc_mock.call_count, 0)

    def test_archive_logs_multiple(self):
        with temp_dir() as log_dir:
            log_paths = []
            with open(os.path.join(log_dir, 'fake.log'), 'w') as f:
                f.write('log contents')
            log_paths.append(os.path.join(log_dir, 'fake.log'))
            subdir = os.path.join(log_dir, "subdir")
            os.mkdir(subdir)
            with open(os.path.join(subdir, 'syslog'), 'w') as f:
                f.write('syslog contents')
            log_paths.append(os.path.join(subdir, 'syslog'))
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                archive_logs(log_dir)
            self.assertEqual(1, cc_mock.call_count)
            call_args, call_kwargs = cc_mock.call_args
            gzip_args = call_args[0]
            self.assertEqual(0, len(call_kwargs))
            self.assertEqual(gzip_args[:3], ['gzip', '--best', '-f'])
            self.assertEqual(set(gzip_args[3:]), set(log_paths))

    def test_copy_local_logs(self):
        # Relevent local log files are copied, after changing their permissions
        # to allow access by non-root user.
        env = SimpleEnvironment('a-local', {'type': 'local'})
        with temp_dir() as juju_home_dir:
            log_dir = os.path.join(juju_home_dir, "a-local", "log")
            os.makedirs(log_dir)
            open(os.path.join(log_dir, "all-machines.log"), "w").close()
            template_dir = os.path.join(juju_home_dir, "templates")
            os.mkdir(template_dir)
            open(os.path.join(template_dir, "container.log"), "w").close()
            with patch('deploy_stack.get_juju_home', autospec=True,
                       return_value=juju_home_dir):
                with patch('deploy_stack.lxc_template_glob',
                           os.path.join(template_dir, "*.log")):
                    with patch('subprocess.check_call') as cc_mock:
                        copy_local_logs(env, '/destination/dir')
        expected_files = [os.path.join(juju_home_dir, *p) for p in (
            ('a-local', 'cloud-init-output.log'),
            ('a-local', 'log', 'all-machines.log'),
            ('templates', 'container.log'),
        )]
        self.assertEqual(cc_mock.call_args_list, [
            call(['sudo', 'chmod', 'go+r'] + expected_files),
            call(['cp'] + expected_files + ['/destination/dir']),
        ])

    def test_copy_local_logs_warns(self):
        env = SimpleEnvironment('a-local', {'type': 'local'})
        err = subprocess.CalledProcessError(1, 'cp', None)
        with temp_dir() as juju_home_dir:
            with patch('deploy_stack.get_juju_home', autospec=True,
                       return_value=juju_home_dir):
                with patch('deploy_stack.lxc_template_glob',
                           os.path.join(juju_home_dir, "*.log")):
                    with patch('subprocess.check_call', autospec=True,
                               side_effect=err):
                        copy_local_logs(env, '/destination/dir')
        self.assertEqual(
            ["WARNING Could not retrieve local logs: Command 'cp' returned"
             " non-zero exit status 1"],
            self.log_stream.getvalue().splitlines())

    def test_copy_remote_logs(self):
        # To get the logs, their permissions must be updated first,
        # then downloaded in the order that they will be created
        # to ensure errors do not prevent some logs from being retrieved.
        with patch('deploy_stack.wait_for_port', autospec=True):
            with patch('subprocess.check_output') as cc_mock:
                copy_remote_logs(remote_from_address('10.10.0.1'), '/foo')
        self.assertEqual(
            (get_timeout_prefix(120) + (
                'ssh',
                '-o', 'User ubuntu',
                '-o', 'UserKnownHostsFile /dev/null',
                '-o', 'StrictHostKeyChecking no',
                '-o', 'PasswordAuthentication no',
                '10.10.0.1',
                'sudo chmod -Rf go+r /var/log/cloud-init*.log'
                ' /var/log/juju/*.log'
                ' /var/lib/juju/containers/juju-*-lxc-*/'
                ' /var/log/lxd/juju-*'
                ' /var/log/lxd/lxd.log'
                ' /var/log/syslog'
                ' /var/log/mongodb/mongodb.log'
                ' /etc/network/interfaces'
                ' /etc/environment'
                ' /home/ubuntu/ifconfig.log'
                ),),
            cc_mock.call_args_list[0][0])
        self.assertEqual(
            (get_timeout_prefix(120) + (
                'ssh',
                '-o', 'User ubuntu',
                '-o', 'UserKnownHostsFile /dev/null',
                '-o', 'StrictHostKeyChecking no',
                '-o', 'PasswordAuthentication no',
                '10.10.0.1',
                'ifconfig > /home/ubuntu/ifconfig.log'),),
            cc_mock.call_args_list[1][0])
        self.assertEqual(
            (get_timeout_prefix(120) + (
                'scp', '-rC',
                '-o', 'User ubuntu',
                '-o', 'UserKnownHostsFile /dev/null',
                '-o', 'StrictHostKeyChecking no',
                '-o', 'PasswordAuthentication no',
                '10.10.0.1:/var/log/cloud-init*.log',
                '10.10.0.1:/var/log/juju/*.log',
                '10.10.0.1:/var/lib/juju/containers/juju-*-lxc-*/',
                '10.10.0.1:/var/log/lxd/juju-*',
                '10.10.0.1:/var/log/lxd/lxd.log',
                '10.10.0.1:/var/log/syslog',
                '10.10.0.1:/var/log/mongodb/mongodb.log',
                '10.10.0.1:/etc/network/interfaces',
                '10.10.0.1:/etc/environment',
                '10.10.0.1:/home/ubuntu/ifconfig.log',
                '/foo'),),
            cc_mock.call_args_list[2][0])

    def test_copy_remote_logs_windows(self):
        remote = remote_from_address('10.10.0.1', series="win2012hvr2")
        with patch.object(remote, "copy", autospec=True) as copy_mock:
            copy_remote_logs(remote, '/foo')
        paths = [
            "%ProgramFiles(x86)%\\Cloudbase Solutions\\Cloudbase-Init\\log\\*",
            "C:\\Juju\\log\\juju\\*.log",
        ]
        copy_mock.assert_called_once_with("/foo", paths)

    def test_copy_remote_logs_with_errors(self):
        # Ssh and scp errors will happen when /var/log/juju doesn't exist yet,
        # but we log the case anc continue to retrieve as much as we can.
        def remote_op(*args, **kwargs):
            if 'ssh' in args:
                raise subprocess.CalledProcessError('ssh error', 'output')
            else:
                raise subprocess.CalledProcessError('scp error', 'output')

        with patch('subprocess.check_output', side_effect=remote_op) as co:
            with patch('deploy_stack.wait_for_port', autospec=True):
                copy_remote_logs(remote_from_address('10.10.0.1'), '/foo')
        self.assertEqual(3, co.call_count)
        self.assertEqual(
            ["DEBUG ssh -o 'User ubuntu' -o 'UserKnownHostsFile /dev/null' "
             "-o 'StrictHostKeyChecking no' -o 'PasswordAuthentication no' "
             "10.10.0.1 'sudo chmod -Rf go+r /var/log/cloud-init*.log "
             "/var/log/juju/*.log /var/lib/juju/containers/juju-*-lxc-*/ "
             "/var/log/lxd/juju-* "
             "/var/log/lxd/lxd.log "
             "/var/log/syslog "
             "/var/log/mongodb/mongodb.log "
             "/etc/network/interfaces "
             "/etc/environment "
             "/home/ubuntu/ifconfig.log'",
             'WARNING Could not allow access to the juju logs:',
             'WARNING None',
             "DEBUG ssh -o 'User ubuntu' -o 'UserKnownHostsFile /dev/null' "
             "-o 'StrictHostKeyChecking no' -o 'PasswordAuthentication no' "
             "10.10.0.1 'ifconfig > /home/ubuntu/ifconfig.log'",
             'WARNING Could not capture ifconfig state:',
             'WARNING None', 'WARNING Could not retrieve some or all logs:',
             'WARNING CalledProcessError()',
             ],
            self.log_stream.getvalue().splitlines())

    def test_get_machines_for_logs(self):
        client = ModelClient(
            JujuData('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                dns-name: 10.11.12.13
              "1":
                dns-name: 10.11.12.14
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = get_remote_machines(client, {})
        self.assert_machines(
            {'0': '10.11.12.13', '1': '10.11.12.14'}, machines)

    def test_get_machines_for_logs_with_boostrap_host(self):
        client = ModelClient(
            JujuData('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                instance-id: pending
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = get_remote_machines(client, {'0': '10.11.111.222'})
        self.assert_machines({'0': '10.11.111.222'}, machines)

    def test_get_machines_for_logs_with_no_addresses(self):
        client = ModelClient(
            JujuData('cloud', {'type': 'ec2'}), '1.23.4', None)
        with patch.object(client, 'get_status', autospec=True,
                          side_effect=Exception):
            machines = get_remote_machines(client, {'0': '10.11.111.222'})
        self.assert_machines({'0': '10.11.111.222'}, machines)

    @patch('subprocess.check_call')
    def test_get_remote_machines_with_maas(self, cc_mock):
        config = {
            'type': 'maas',
            'name': 'foo',
            'maas-server': 'http://bar/MASS/',
            }
        juju_data = JujuData('cloud', config)
        cloud_name = 'mycloud'
        juju_data.clouds = {'clouds': {cloud_name: {
            'endpoint': config['maas-server'],
            'type': config['type'],
            }}}
        juju_data.credentials = {'credentials': {cloud_name: {'credentials': {
            'maas-oauth': 'baz',
            }}}}
        client = ModelClient(juju_data, '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                dns-name: node1.maas
              "1":
                dns-name: node2.maas
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            allocated_ips = {
                'node1.maas': '10.11.12.13',
                'node2.maas': '10.11.12.14',
            }
            with patch('substrate.MAASAccount.get_allocated_ips',
                       autospec=True, return_value=allocated_ips):
                machines = get_remote_machines(client, {'0': 'node1.maas'})
        self.assert_machines(
            {'0': '10.11.12.13', '1': '10.11.12.14'}, machines)

    def test_iter_remote_machines(self):
        client = ModelClient(
            JujuData('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                dns-name: 10.11.12.13
              "1":
                dns-name: 10.11.12.14
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = [(m, r.address)
                        for m, r in iter_remote_machines(client)]
        self.assertEqual(
            [('0', '10.11.12.13'), ('1', '10.11.12.14')], machines)

    def test_iter_remote_machines_with_series(self):
        client = ModelClient(
            JujuData('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                dns-name: 10.11.12.13
                series: trusty
              "1":
                dns-name: 10.11.12.14
                series: win2012hvr2
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = [(m, r.address, r.series)
                        for m, r in iter_remote_machines(client)]
        self.assertEqual(
            [('0', '10.11.12.13', 'trusty'),
             ('1', '10.11.12.14', 'win2012hvr2')], machines)

    def test_retain_config(self):
        with temp_dir() as jenv_dir:
            jenv_path = os.path.join(jenv_dir, "temp.jenv")
            with open(jenv_path, 'w') as f:
                f.write('jenv data')
                with temp_dir() as log_dir:
                    status = retain_config(jenv_path, log_dir)
                    self.assertIs(status, True)
                    self.assertEqual(['temp.jenv'], os.listdir(log_dir))

        with patch('shutil.copy', autospec=True,
                   side_effect=IOError) as rj_mock:
            status = retain_config('src', 'dst')
        self.assertIs(status, False)
        rj_mock.assert_called_with('src', 'dst')


class TestDeployDummyStack(FakeHomeTestCase):

    def test_deploy_dummy_stack_sets_centos_constraints(self):
        env = JujuData('foo', {'type': 'maas'})
        client = ModelClient(env, '2.0.0', '/foo/juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            with patch.object(ModelClient, 'wait_for_started'):
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    deploy_dummy_stack(client, 'centos')
        assert_juju_call(self, cc_mock, client,
                         ('juju', '--show-log', 'set-model-constraints', '-m',
                          'foo:foo', 'tags=MAAS_NIC_1'), 0)

    def test_assess_juju_relations(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = ModelClient(env, None, '/foo/juju')
        with patch.object(client, 'get_juju_output', side_effect='fake-token',
                          autospec=True):
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    with patch('deploy_stack.check_token',
                               autospec=True) as ct_mock:
                        assess_juju_relations(client)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'config', '-m', 'foo:foo',
            'dummy-source', 'token=fake-token'), 0)
        ct_mock.assert_called_once_with(client, 'fake-token')

    def test_deploy_dummy_stack_centos(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(client, 'deploy', autospec=True) as dp_mock:
            with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                deploy_dummy_stack(client, 'centos7')
        calls = [
            call('/tmp/repo/charms-centos/dummy-source', series='centos7'),
            call('/tmp/repo/charms-centos/dummy-sink', series='centos7')]
        self.assertEqual(dp_mock.mock_calls, calls)

    def test_deploy_dummy_stack_win(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(client, 'deploy', autospec=True) as dp_mock:
            with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                deploy_dummy_stack(client, 'win2012hvr2')
        calls = [
            call('/tmp/repo/charms-win/dummy-source', series='win2012hvr2'),
            call('/tmp/repo/charms-win/dummy-sink', series='win2012hvr2')]
        self.assertEqual(dp_mock.mock_calls, calls)

    def test_deploy_dummy_stack_charmstore(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(client, 'deploy', autospec=True) as dp_mock:
            deploy_dummy_stack(client, 'xenial', use_charmstore=True)
        calls = [
            call('cs:~juju-qa/dummy-source', series='xenial'),
            call('cs:~juju-qa/dummy-sink', series='xenial')]
        self.assertEqual(dp_mock.mock_calls, calls)

    def test_deploy_dummy_stack(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = ModelClient(env, '2.0.0', '/foo/juju')
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {
                'dummy-sink': {'units': {
                    'dummy-sink/0': {'agent-state': 'started'}}
                }
            }
        })

        def output(*args, **kwargs):
            token_file = '/var/run/dummy-sink/token'
            output = {
                ('show-status', '--format', 'yaml'): status,
                ('ssh', 'dummy-sink/0', 'cat', token_file): 'fake-token',
            }
            return output[args]

        with patch.object(client, 'get_juju_output', side_effect=output,
                          autospec=True) as gjo_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    with patch('sys.stdout', autospec=True):
                        with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                            deploy_dummy_stack(client, 'bar-')
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo:foo',
            '/tmp/repo/charms/dummy-source', '--series', 'bar-'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo:foo',
            '/tmp/repo/charms/dummy-sink', '--series', 'bar-'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-relation', '-m', 'foo:foo',
            'dummy-source', 'dummy-sink'), 2)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'expose', '-m', 'foo:foo', 'dummy-sink'), 3)
        self.assertEqual(cc_mock.call_count, 4)
        self.assertEqual(
            [
                call('show-status', '--format', 'yaml', controller=False)
            ],
            gjo_mock.call_args_list)

        client = client.clone(version='1.25.0')
        with patch.object(client, 'get_juju_output', side_effect=output,
                          autospec=True) as gjo_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    with patch('sys.stdout', autospec=True):
                        with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                            deploy_dummy_stack(client, 'bar-')
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo:foo',
            'local:bar-/dummy-source', '--series', 'bar-'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo:foo',
            'local:bar-/dummy-sink', '--series', 'bar-'), 1)


def fake_SimpleEnvironment(name):
    return SimpleEnvironment(name, {})


def fake_ModelClient(env, path=None, debug=None):
    return ModelClient(env=env, version='1.2.3.4', full_path=path)


class FakeBootstrapManager:

    def __init__(self, client, keep_env=False):
        self.client = client
        self.tear_down_client = client
        self.entered_top = False
        self.exited_top = False
        self.entered_bootstrap = False
        self.exited_bootstrap = False
        self.entered_runtime = False
        self.exited_runtime = False
        self.torn_down = False
        self.permanent = False
        self.known_hosts = {'0': '0.example.org'}
        self.keep_env = keep_env

    @contextmanager
    def top_context(self):
        try:
            self.entered_top = True
            yield ['bar']
        finally:
            self.exited_top = True

    @contextmanager
    def bootstrap_context(self, machines):
        initial_home = self.client.env.juju_home
        self.client.env.environment = self.client.env.environment + '-temp'
        self.client.env.controller.name = self.client.env.environment
        try:
            self.entered_bootstrap = True
            self.client.env.juju_home = os.path.join(initial_home, 'isolated')
            self.client.bootstrap()
            yield
        finally:
            self.exited_bootstrap = True
            if not self.permanent:
                self.client.env.juju_home = initial_home

    @contextmanager
    def runtime_context(self, machines):
        try:
            self.entered_runtime = True
            yield
        finally:
            if not self.keep_env:
                self.tear_down()
            self.exited_runtime = True

    def tear_down(self):
        self.tear_down_client.kill_controller()
        self.torn_down = True

    @contextmanager
    def booted_context(self, upload_tools, **kwargs):
        with self.top_context() as machines:
            with self.bootstrap_context(machines):
                self.client.bootstrap(upload_tools)
            with self.runtime_context(machines):
                yield machines


class TestDeployJob(FakeHomeTestCase):

    @contextmanager
    def ds_cxt(self):
        env = JujuData('foo', {})
        client = fake_ModelClient(env)
        bc_cxt = patch('deploy_stack.client_from_config',
                       return_value=client)
        fc_cxt = patch('jujupy.SimpleEnvironment.from_config',
                       return_value=env)
        mgr = MagicMock()
        bm_cxt = patch('deploy_stack.BootstrapManager', autospec=True,
                       return_value=mgr)
        juju_cxt = patch('jujupy.ModelClient.juju', autospec=True)
        ajr_cxt = patch('deploy_stack.assess_juju_run', autospec=True)
        dds_cxt = patch('deploy_stack.deploy_dummy_stack', autospec=True)
        with bc_cxt, fc_cxt, bm_cxt as bm_mock, juju_cxt, ajr_cxt, dds_cxt:
            yield client, bm_mock

    @skipIf(sys.platform in ('win32', 'darwin'),
            'Not supported on Windown and OS X')
    def test_background_chaos_used(self):
        args = Namespace(
            env='base', juju_bin='/fake/juju', logs='log', temp_env_name='foo',
            charm_prefix=None, bootstrap_host=None, machine=None,
            series='trusty', debug=False, agent_url=None, agent_stream=None,
            keep_env=False, upload_tools=False, with_chaos=1, jes=False,
            region=None, verbose=False, upgrade=False, deadline=None,
            controller_host=None, use_charmstore=False, to=None
        )
        with self.ds_cxt():
            with patch('deploy_stack.background_chaos',
                       autospec=True) as bc_mock:
                with patch('deploy_stack.assess_juju_relations',
                           autospec=True):
                    with patch('subprocess.Popen', autospec=True,
                               return_value=FakePopen('', '', 0)):
                        _deploy_job(args, 'local:trusty/', 'trusty')
        self.assertEqual(bc_mock.call_count, 1)
        self.assertEqual(bc_mock.mock_calls[0][1][0], 'foo')
        self.assertEqual(bc_mock.mock_calls[0][1][2], 'log')
        self.assertEqual(bc_mock.mock_calls[0][1][3], 1)

    @skipIf(sys.platform in ('win32', 'darwin'),
            'Not supported on Windown and OS X')
    def test_background_chaos_not_used(self):
        args = Namespace(
            env='base', juju_bin='/fake/juju', logs='log', temp_env_name='foo',
            charm_prefix=None, bootstrap_host=None, machine=None,
            series='trusty', debug=False, agent_url=None, agent_stream=None,
            keep_env=False, upload_tools=False, with_chaos=0, jes=False,
            region=None, verbose=False, upgrade=False, deadline=None,
            controller_host=None, use_charmstore=False, to=None
        )
        with self.ds_cxt():
            with patch('deploy_stack.background_chaos',
                       autospec=True) as bc_mock:
                with patch('deploy_stack.assess_juju_relations',
                           autospec=True):
                    with patch('subprocess.Popen', autospec=True,
                               return_value=FakePopen('', '', 0)):
                        _deploy_job(args, 'local:trusty/', 'trusty')
        self.assertEqual(bc_mock.call_count, 0)

    def test_region(self):
        args = Namespace(
            env='base', juju_bin='/fake/juju', logs='log', temp_env_name='foo',
            charm_prefix=None, bootstrap_host=None, machine=None,
            series='trusty', debug=False, agent_url=None, agent_stream=None,
            keep_env=False, upload_tools=False, with_chaos=0, jes=False,
            region='region-foo', verbose=False, upgrade=False, deadline=None,
            controller_host=None, use_charmstore=False, to=None)
        with self.ds_cxt() as (client, bm_mock):
            with patch('deploy_stack.assess_juju_relations',
                       autospec=True):
                with patch('subprocess.Popen', autospec=True,
                           return_value=FakePopen('', '', 0)):
                    with patch('deploy_stack.make_controller_strategy',
                               ) as mcs_mock:
                        _deploy_job(args, 'local:trusty/', 'trusty')
                    jes = client.is_jes_enabled()
        bm_mock.assert_called_once_with(
            'foo', client, client, None, None, 'trusty', None, None,
            'region-foo', 'log', False,
            permanent=jes, jes_enabled=jes,
            controller_strategy=mcs_mock.return_value)

    def test_deploy_job_changes_series_with_win(self):
        args = Namespace(
            series='windows', temp_env_name='windows', env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            region=None, verbose=None, use_charmstore=False, to=None)
        with patch('deploy_stack.deploy_job_parse_args', return_value=args,
                   autospec=True):
            with patch('deploy_stack._deploy_job', autospec=True) as ds_mock:
                deploy_job()
        ds_mock.assert_called_once_with(args, 'windows', 'trusty')

    def test_deploy_job_changes_series_with_centos(self):
        args = Namespace(
            series='centos', temp_env_name='centos', env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            region=None, verbose=None, use_charmstore=False, to=None)
        with patch('deploy_stack.deploy_job_parse_args', return_value=args,
                   autospec=True):
            with patch('deploy_stack._deploy_job', autospec=True) as ds_mock:
                deploy_job()
        ds_mock.assert_called_once_with(args, 'centos', 'trusty')


class TestTestUpgrade(FakeHomeTestCase):

    RUN_UNAME = (
        'juju', '--show-log', 'run', '-e', 'foo', '--format', 'json',
        '--service', 'dummy-source,dummy-sink', 'uname')
    STATUS = (
        'juju', '--show-log', 'show-status', '-m', 'foo:foo',
        '--format', 'yaml')
    CONTROLLER_STATUS = (
        'juju', '--show-log', 'show-status', '-m', 'foo:controller',
        '--format', 'yaml')
    GET_ENV = ('juju', '--show-log', 'model-config', '-m', 'foo:foo',
               'agent-metadata-url')
    GET_CONTROLLER_ENV = (
        'juju', '--show-log', 'model-config', '-m', 'foo:controller',
        'agent-metadata-url')
    LIST_MODELS = (
        'juju', '--show-log', 'list-models', '-c', 'foo', '--format', 'yaml')

    @classmethod
    def upgrade_output(cls, args, **kwargs):
        status = yaml.safe_dump({
            'machines': {'0': {
                'agent-state': 'started',
                'agent-version': '2.0-rc2'}},
            'services': {}})
        juju_run_out = json.dumps([
            {"MachineId": "1", "Stdout": "Linux\n"},
            {"MachineId": "2", "Stdout": "Linux\n"}])
        list_models = json.dumps(
            {'models': [
                {'name': 'controller'},
                {'name': 'foo'},
            ]})
        output = {
            cls.STATUS: status,
            cls.CONTROLLER_STATUS: status,
            cls.RUN_UNAME: juju_run_out,
            cls.GET_ENV: 'testing',
            cls.GET_CONTROLLER_ENV: 'testing',
            cls.LIST_MODELS: list_models,
        }
        return FakePopen(output[args], '', 0)

    @contextmanager
    def upgrade_mocks(self):
        with patch('subprocess.Popen', side_effect=self.upgrade_output,
                   autospec=True) as co_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.check_token', autospec=True):
                    with patch('deploy_stack.get_random_string',
                               return_value="FAKETOKEN", autospec=True):
                        with patch('jujupy.ModelClient.get_version',
                                   side_effect=lambda cls:
                                   '2.0-rc2-arch-series'):
                            with patch(
                                    'jujupy.client.get_timeout_prefix',
                                    autospec=True, return_value=()):
                                yield (co_mock, cc_mock)

    def test_assess_upgrade(self):
        env = JujuData('foo', {'type': 'foo'})
        old_client = ModelClient(env, None, '/foo/juju')
        with self.upgrade_mocks() as (co_mock, cc_mock):
            assess_upgrade(old_client, '/bar/juju')
        new_client = ModelClient(env, None, '/bar/juju')
        # Needs to upgrade the controller first.
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'upgrade-juju', '-m', 'foo:controller',
            '--agent-version', '2.0-rc2'), 0)
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'show-status', '-m', 'foo:controller',
            '--format', 'yaml'), 1)
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'list-models', '-c', 'foo'), 2)
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'upgrade-juju', '-m', 'foo:foo',
            '--agent-version', '2.0-rc2'), 3)
        self.assertEqual(cc_mock.call_count, 6)
        assert_juju_call(self, co_mock, new_client, self.LIST_MODELS, 0)
        assert_juju_call(self, co_mock, new_client, self.GET_CONTROLLER_ENV, 1)
        assert_juju_call(self, co_mock, new_client, self.GET_CONTROLLER_ENV, 2)
        assert_juju_call(self, co_mock, new_client, self.CONTROLLER_STATUS, 3)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 5)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 6)
        assert_juju_call(self, co_mock, new_client, self.STATUS, 7)
        self.assertEqual(co_mock.call_count, 9)

    def test__get_clients_to_upgrade_returns_controller_and_model(self):
        old_client = fake_juju_client()
        old_client.bootstrap()

        with patch('jujupy.ModelClient.get_version',
                   return_value='2.0-rc2-arch-series'):
            new_clients = _get_clients_to_upgrade(
                old_client, '/foo/newer/juju')

        self.assertEqual(len(new_clients), 2)
        self.assertEqual(new_clients[0].model_name, 'controller')
        self.assertEqual(new_clients[1].model_name, 'name')

    def test_mass_timeout(self):
        config = {'type': 'foo'}
        old_client = ModelClient(JujuData('foo', config), None, '/foo/juju')
        with self.upgrade_mocks():
            with patch.object(ModelClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
            wfv_mock.assert_has_calls([call('2.0-rc2', 600)] * 2)
            config['type'] = 'maas'
            with patch.object(ModelClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
        wfv_mock.assert_has_calls([call('2.0-rc2', 1200)] * 2)


class TestMakeControllerStrategy(TestCase):

    def test_make_controller_strategy_no_host(self):
        client = object()
        tear_down_client = object()
        strategy = make_controller_strategy(client, tear_down_client, None)
        self.assertIs(CreateController, type(strategy))
        self.assertEqual(client, strategy.client)
        self.assertEqual(tear_down_client, strategy.tear_down_client)

    def test_make_controller_strategy_host(self):
        client = object()
        tear_down_client = object()
        with patch.dict(os.environ, {
                'SSO_EMAIL': 'sso@email',
                'SSO_PASSWORD': 'sso-password'}):
            strategy = make_controller_strategy(client, tear_down_client,
                                                'host')
        self.assertIs(PublicController, type(strategy))
        self.assertEqual(client, strategy.client)
        self.assertEqual(tear_down_client, strategy.tear_down_client)
        self.assertEqual('sso@email', strategy.email)
        self.assertEqual('sso-password', strategy.password)


class TestExistingController(FakeHomeTestCase):

    def get_controller(self):
        client = fake_juju_client()
        create_controller = ExistingController(client)
        return create_controller

    def test_prepare(self):
        controller = self.get_controller()
        controller.prepare('foo')
        self.assertEqual('foo', controller.client.env.controller.name)

    def test_create_initial_model(self):
        controller = self.get_controller()
        client = controller.client
        self.assertEqual({'models': []}, client.get_models())
        controller.create_initial_model()
        self.assertEqual([{'name': 'name'}], client.get_models()['models'])

    def test_tear_down(self):
        controller = self.get_controller()
        client = controller.client
        client.add_model(client.env)
        self.assertEqual([{'name': 'name'}], client.get_models()['models'])
        controller.tear_down('')
        self.assertEqual([], client.get_models()['models'])


class TestCreateController(FakeHomeTestCase):

    def get_cleanup_controller(self):
        client = fake_juju_client()
        create_controller = CreateController(None, client)
        return create_controller

    def get_controller(self):
        client = fake_juju_client()
        create_controller = CreateController(client, None)
        return create_controller

    def test_prepare_no_existing(self):
        create_controller = self.get_cleanup_controller()
        client = create_controller.tear_down_client
        create_controller.prepare()
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'not-bootstrapped', client._backend.controller_state.state)

    def test_prepare_leftover(self):
        create_controller = self.get_cleanup_controller()
        client = create_controller.tear_down_client
        client.bootstrap()
        create_controller.prepare()
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'controller-killed', client._backend.controller_state.state)

    def test_create_initial_model(self):
        controller = self.get_controller()
        client = controller.client
        self.assertEqual({'models': []}, client.get_models())
        controller.create_initial_model(False, 'angsty', {})
        self.assertItemsEqual([{'name': 'controller'}, {'name': 'name'}],
                              client.get_models()['models'])
        self.assertEqual(
            'bootstrapped', client._backend.controller_state.state)

    def test_get_hosts(self):
        controller = self.get_controller()
        client = controller.client
        client.bootstrap()
        self.assertEqual({'0': '0.example.com'}, controller.get_hosts())

    def test_tear_down_existing(self):
        create_controller = self.get_cleanup_controller()
        client = create_controller.tear_down_client
        client.bootstrap()
        create_controller.tear_down(True)
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'controller-destroyed', client._backend.controller_state.state)

    def test_tear_down_existing_no_controller(self):
        create_controller = self.get_cleanup_controller()
        client = create_controller.tear_down_client
        client.bootstrap()
        create_controller.tear_down(False)
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'controller-killed', client._backend.controller_state.state)

    def test_tear_down_nothing(self):
        create_controller = self.get_cleanup_controller()
        with self.assertRaises(subprocess.CalledProcessError):
            create_controller.tear_down(True)


class TestPublicController(FakeHomeTestCase):

    def get_cleanup_controller(self):
        client = fake_juju_client()
        public_controller = PublicController('host', 'email2', 'password2',
                                             None, client)
        return public_controller

    def get_controller(self):
        client = fake_juju_client()
        public_controller = PublicController('host', 'email2', 'password2',
                                             client, None)
        return public_controller

    def test_prepare_no_existing(self):
        public_controller = self.get_cleanup_controller()
        client = public_controller.tear_down_client
        public_controller.prepare()
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'not-bootstrapped', client._backend.controller_state.state)

    def test_prepare_leftover(self):
        public_controller = self.get_cleanup_controller()
        client = public_controller.tear_down_client
        client.add_model(client.env)
        public_controller.prepare()
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'model-destroyed', client._backend.controller_state.state)

    def test_create_initial_model(self):
        controller = self.get_controller()
        client = controller.client
        self.assertEqual({'models': []}, client.get_models())
        controller.create_initial_model(False, 'angsty', {})
        self.assertItemsEqual([{'name': 'name'}],
                              client.get_models()['models'])
        self.assertEqual(
            'created', client._backend.controller_state.state)

    def test_get_hosts(self):
        controller = self.get_controller()
        client = controller.client
        client.bootstrap()
        self.assertEqual({}, controller.get_hosts())

    def test_tear_down_existing(self):
        public_controller = self.get_cleanup_controller()
        client = public_controller.tear_down_client
        client.add_model(client.env)
        public_controller.tear_down(True)
        self.assertEqual({'models': []}, client.get_models())
        self.assertEqual(
            'model-destroyed', client._backend.controller_state.state)

    def test_tear_down_nothing(self):
        public_controller = self.get_cleanup_controller()
        with self.assertRaises(subprocess.CalledProcessError):
            public_controller.tear_down(True)


class TestBootstrapManager(FakeHomeTestCase):

    def test_from_args(self):
        deadline = datetime(2012, 11, 10, 9, 8, 7)
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host='example.org', machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True,
            deadline=deadline, to=None)
        with patch('deploy_stack.client_from_config') as fc_mock:
            bs_manager = BootstrapManager.from_args(args)
        fc_mock.assert_called_once_with('foo', 'bar', debug=True,
                                        soft_deadline=deadline)
        self.assertEqual('baz', bs_manager.temp_env_name)
        self.assertIs(fc_mock.return_value, bs_manager.client)
        self.assertIs(fc_mock.return_value, bs_manager.tear_down_client)
        self.assertEqual('example.org', bs_manager.bootstrap_host)
        self.assertEqual(['example.com'], bs_manager.machines)
        self.assertEqual('angsty', bs_manager.series)
        self.assertEqual('qux', bs_manager.agent_url)
        self.assertEqual('escaped', bs_manager.agent_stream)
        self.assertEqual('eu-west-northwest-5', bs_manager.region)
        self.assertIs(True, bs_manager.keep_env)
        self.assertEqual('pine', bs_manager.log_dir)
        jes_enabled = bs_manager.client.is_jes_enabled.return_value
        self.assertEqual(jes_enabled, bs_manager.permanent)
        self.assertEqual(jes_enabled, bs_manager.jes_enabled)
        self.assertEqual({'0': 'example.org'}, bs_manager.known_hosts)
        self.assertIsFalse(bs_manager.has_controller)

    def test_from_existing(self):
        deadline = datetime(2012, 11, 10, 9, 8, 7)
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host='example.org', machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True,
            deadline=deadline, to=None, existing='existing')
        with patch('deploy_stack.client_for_existing') as fc_mock:
            with patch.dict('os.environ', {'JUJU_DATA': 'foo'}):
                bs_manager = BootstrapManager.from_existing_controller(args)
        fc_mock.assert_called_once_with(
            'bar', 'foo', controller_name='existing', model_name='baz')
        self.assertEqual('baz', bs_manager.temp_env_name)
        self.assertIs(fc_mock.return_value, bs_manager.client)
        self.assertIs(fc_mock.return_value, bs_manager.tear_down_client)
        self.assertEqual('example.org', bs_manager.bootstrap_host)
        self.assertEqual(['example.com'], bs_manager.machines)
        self.assertEqual('angsty', bs_manager.series)
        self.assertEqual('qux', bs_manager.agent_url)
        self.assertEqual('escaped', bs_manager.agent_stream)
        self.assertEqual('eu-west-northwest-5', bs_manager.region)
        self.assertIs(True, bs_manager.keep_env)
        self.assertEqual('pine', bs_manager.log_dir)
        jes_enabled = bs_manager.client.is_jes_enabled.return_value
        self.assertEqual(jes_enabled, bs_manager.permanent)
        self.assertEqual(jes_enabled, bs_manager.jes_enabled)
        self.assertEqual({'0': 'example.org'}, bs_manager.known_hosts)
        self.assertIsFalse(bs_manager.has_controller)

    def test_from_client(self):
        deadline = datetime(2012, 11, 10, 9, 8, 7)
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host='example.org', machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True,
            deadline=deadline, to=None)
        client = fake_juju_client()
        bs_manager = BootstrapManager.from_client(args, client)
        self.assertEqual('baz', bs_manager.temp_env_name)
        self.assertIs(client, bs_manager.client)
        self.assertEqual('example.org', bs_manager.bootstrap_host)
        self.assertEqual(['example.com'], bs_manager.machines)
        self.assertEqual('angsty', bs_manager.series)
        self.assertEqual('qux', bs_manager.agent_url)
        self.assertEqual('escaped', bs_manager.agent_stream)
        self.assertEqual('eu-west-northwest-5', bs_manager.region)
        self.assertIs(True, bs_manager.keep_env)
        self.assertEqual('pine', bs_manager.log_dir)
        jes_enabled = client.is_jes_enabled()
        self.assertEqual(jes_enabled, bs_manager.permanent)
        self.assertEqual(jes_enabled, bs_manager.jes_enabled)
        self.assertEqual({'0': 'example.org'}, bs_manager.known_hosts)
        self.assertIsFalse(bs_manager.has_controller)

    def test_no_args(self):
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host='example.org', machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs=None, keep_env=True,
            deadline=None, to=None)
        with patch('deploy_stack.client_from_config') as fc_mock:
            with patch('utility.os.makedirs'):
                bs_manager = BootstrapManager.from_args(args)
        fc_mock.assert_called_once_with('foo', 'bar', debug=True,
                                        soft_deadline=None)
        self.assertEqual('baz', bs_manager.temp_env_name)
        self.assertIs(fc_mock.return_value, bs_manager.client)
        self.assertIs(fc_mock.return_value, bs_manager.tear_down_client)
        self.assertEqual('example.org', bs_manager.bootstrap_host)
        self.assertEqual(['example.com'], bs_manager.machines)
        self.assertEqual('angsty', bs_manager.series)
        self.assertEqual('qux', bs_manager.agent_url)
        self.assertEqual('escaped', bs_manager.agent_stream)
        self.assertEqual('eu-west-northwest-5', bs_manager.region)
        self.assertIs(True, bs_manager.keep_env)
        logs_arg = bs_manager.log_dir.split("/")
        logs_ts = logs_arg[4]
        self.assertEqual(logs_arg[1:4], ['tmp', 'baz', 'logs'])
        self.assertTrue(logs_ts, datetime.strptime(logs_ts, "%Y%m%d%H%M%S"))
        jes_enabled = bs_manager.client.is_jes_enabled.return_value
        self.assertEqual(jes_enabled, bs_manager.permanent)
        self.assertEqual(jes_enabled, bs_manager.jes_enabled)
        self.assertEqual({'0': 'example.org'}, bs_manager.known_hosts)
        self.assertIsFalse(bs_manager.has_controller)

    def test_jes_not_permanent(self):
        with self.assertRaisesRegexp(ValueError, 'Cannot set permanent False'
                                     ' if jes_enabled is True.'):
            BootstrapManager(
                jes_enabled=True, permanent=False,
                temp_env_name=None, client=None, tear_down_client=None,
                bootstrap_host=None, machines=[], series=None, agent_url=None,
                agent_stream=None, region=None, log_dir=None, keep_env=None)

    def test_from_args_no_host(self):
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host=None, machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True,
            deadline=None, to=None)
        with patch('deploy_stack.client_from_config'):
            bs_manager = BootstrapManager.from_args(args)
        self.assertIs(None, bs_manager.bootstrap_host)
        self.assertEqual({}, bs_manager.known_hosts)

    def make_client(self):
        client = MagicMock()
        client.env = SimpleEnvironment(
            'foo', {'type': 'baz'}, use_context(self, temp_dir()))
        client.is_jes_enabled.return_value = False
        client.get_matching_agent_version.return_value = '3.14'
        client.get_cache_path.return_value = get_cache_path(
            client.env.juju_home)
        return client

    def test_bootstrap_context_kill(self):
        client = fake_juju_client()
        client.env.juju_home = use_context(self, temp_dir())
        initial_home = client.env.juju_home
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)

        def check_config(try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            jenv_path = get_jenv_path(client.env.juju_home, 'foobar')
            self.assertFalse(os.path.exists(jenv_path))
            environments_path = get_environments_path(client.env.juju_home)
            self.assertTrue(os.path.isfile(environments_path))
            self.assertNotEqual(initial_home, client.env.juju_home)

        ije_cxt = patch.object(client, 'is_jes_enabled')
        with patch('jujupy.ModelClient.kill_controller',
                   side_effect=check_config) as kill_mock, ije_cxt:
            with bs_manager.bootstrap_context([]):
                pass
        kill_mock.assert_called_once_with()

    def test_bootstrap_context_tear_down_jenv(self):
        client = self.make_client()
        initial_home = client.env.juju_home
        jenv_path = get_jenv_path(client.env.juju_home, 'foobar')
        os.makedirs(os.path.dirname(jenv_path))
        with open(jenv_path, 'w'):
            pass

        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)

        def check_config(try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            self.assertTrue(os.path.isfile(jenv_path))
            environments_path = get_environments_path(client.env.juju_home)
            self.assertFalse(os.path.exists(environments_path))
            self.assertEqual(initial_home, client.env.juju_home)

        with patch.object(client, 'kill_controller',
                          side_effect=check_config) as kill_mock:
            with bs_manager.bootstrap_context([]):
                pass
        kill_mock.assert_called_once_with()

    def test_bootstrap_context_tear_down_client(self):
        client = self.make_client()
        tear_down_client = self.make_client()
        tear_down_client.env = client.env
        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client, None, [], None, None, None,
            None, client.env.juju_home, False, False, False)

        def check_config(try_jes=False):
            self.assertIsFalse(client.is_jes_enabled.called)
            self.assertIsFalse(tear_down_client.is_jes_enabled.called)

        with patch.object(tear_down_client, 'kill_controller',
                          side_effect=check_config) as kill_mock:
            with patch('deploy_stack.BootstrapManager.tear_down') as td_mock:
                with bs_manager.bootstrap_context([]):
                    pass
        kill_mock.assert_called_once_with()
        self.assertIsFalse(td_mock.called)

    def test_bootstrap_context_tear_down_client_jenv(self):
        client = self.make_client()
        tear_down_client = self.make_client()
        tear_down_client.env = client.env
        jenv_path = get_jenv_path(client.env.juju_home, 'foobar')
        os.makedirs(os.path.dirname(jenv_path))
        with open(jenv_path, 'w'):
            pass

        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client,
            None, [], None, None, None, None, client.env.juju_home, False,
            False, False)

        def check_config(try_jes=False):
            self.assertIsFalse(client.is_jes_enabled.called)
            self.assertIsFalse(tear_down_client.is_jes_enabled.called)

        with patch.object(tear_down_client, 'kill_controller',
                          side_effect=check_config) as kill_mock:
            with bs_manager.bootstrap_context([]):
                kill_mock.assert_called_once_with()

    def test_bootstrap_context_no_set_home(self):
        orig_home = get_juju_home()
        client = self.make_client()
        jenv_path = get_jenv_path(client.env.juju_home, 'foobar')
        os.makedirs(os.path.dirname(jenv_path))
        with open(jenv_path, 'w'):
            pass

        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)
        with bs_manager.bootstrap_context([]):
            self.assertEqual(orig_home, get_juju_home())

    def test_bootstrap_context_calls_update_env(self):
        client = fake_juju_client()
        client.env.juju_home = use_context(self, temp_dir())
        ue_mock = use_context(
            self, patch('deploy_stack.update_env', wraps=update_env))
        wfp_mock = use_context(
            self, patch('deploy_stack.wait_for_port', autospec=True))
        bs_manager = BootstrapManager(
            'bar', client, client, None,
            [], 'wacky', 'url', 'devel', None, client.env.juju_home, False,
            True, True)
        bs_manager.known_hosts['0'] = 'bootstrap.example.org'
        with bs_manager.bootstrap_context([]):
            pass
        ue_mock.assert_called_with(
            client.env, 'bar', series='wacky',
            bootstrap_host='bootstrap.example.org',
            agent_url='url', agent_stream='devel', region=None)
        wfp_mock.assert_called_once_with(
            'bootstrap.example.org', 22, timeout=120)

    def test_bootstrap_context_calls_update_env_omit(self):
        client = fake_juju_client()
        client.env.juju_home = use_context(self, temp_dir())
        ue_mock = use_context(
            self, patch('deploy_stack.update_env', wraps=update_env))
        wfp_mock = use_context(
            self, patch('deploy_stack.wait_for_port', autospec=True))
        bs_manager = BootstrapManager(
            'bar', client, client, None,
            [], 'wacky', 'url', 'devel', None, client.env.juju_home, True,
            True, True)
        bs_manager.known_hosts['0'] = 'bootstrap.example.org'
        with bs_manager.bootstrap_context(
                [], omit_config={'bootstrap_host', 'series'}):
            pass
        ue_mock.assert_called_with(client.env, 'bar', agent_url='url',
                                   agent_stream='devel', region=None)
        wfp_mock.assert_called_once_with(
            'bootstrap.example.org', 22, timeout=120)

    def test_bootstrap_context_sets_has_controller(self):
        client = self.make_client()
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            None, False, False, False)
        with patch.object(client, 'kill_controller'):
            with bs_manager.bootstrap_context([]):
                self.assertIsTrue(bs_manager.has_controller)
        self.assertIsTrue(bs_manager.has_controller)

    def test_existing_bootstrap_context_sets_has_controller(self):
        client = self.make_client()
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            None, False, False, False)
        with patch.object(client, 'kill_controller'):
            with bs_manager.existing_bootstrap_context([]):
                self.assertIsTrue(bs_manager.has_controller)
        self.assertIsTrue(bs_manager.has_controller)

    def test_handle_bootstrap_exceptions_ignores_soft_deadline(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = ModelClient(env, None, None)
        tear_down_client = ModelClient(env, None, None)
        soft_deadline = datetime(2015, 1, 2, 3, 4, 6)
        now = soft_deadline + timedelta(seconds=1)
        client.env.juju_home = use_context(self, temp_dir())
        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client, None, [], None, None, None,
            None, client.env.juju_home, False, permanent=True,
            jes_enabled=True)

        def do_check(*args, **kwargs):
            with client.check_timeouts():
                with tear_down_client.check_timeouts():
                    return make_fake_juju_return()

        with patch.object(bs_manager.tear_down_client, 'juju',
                          side_effect=do_check, autospec=True):
            with patch.object(client._backend, '_now', return_value=now):
                fake_exception = Exception()
                with self.assertRaises(LoggedException) as exc:
                    with bs_manager.handle_bootstrap_exceptions():
                        client._backend.soft_deadline = soft_deadline
                        tear_down_client._backend.soft_deadline = soft_deadline
                        raise fake_exception
                self.assertIs(fake_exception, exc.exception.exception)

    def test_tear_down(self):
        client = fake_juju_client()
        with patch.object(client, 'tear_down') as tear_down_mock:
            with temp_dir() as log_dir:
                bs_manager = BootstrapManager(
                    'foobar', client, client, None, [], None, None, None,
                    None, log_dir, False, False, jes_enabled=False)
                bs_manager.has_controller = True
                bs_manager.tear_down()
        tear_down_mock.assert_called_once_with()
        self.assertIsFalse(bs_manager.has_controller)

    def test_tear_down_requires_same_env(self):
        client = self.make_client()
        client.env.juju_home = 'foobar'
        tear_down_client = self.make_client()
        tear_down_client.env.juju_home = 'barfoo'
        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client,
            None, [], None, None, None, None, client.env.juju_home, False,
            False, False)
        with self.assertRaisesRegexp(AssertionError,
                                     'Tear down client needs same env'):
            with patch.object(client, 'destroy_controller',
                              autospec=True) as destroy_mock:
                bs_manager.tear_down()
        self.assertEqual('barfoo', tear_down_client.env.juju_home)
        self.assertIsFalse(destroy_mock.called)

    def test_dump_all_no_jes_one_model(self):
        client = fake_juju_client()
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                False, jes_enabled=False)
            with patch('deploy_stack.dump_env_logs_known_hosts'):
                with patch.object(client, 'iter_model_clients') as imc_mock:
                    bs_manager.dump_all_logs()
        self.assertEqual(0, imc_mock.call_count)

    def test_dump_all_multi_model(self):
        client = fake_juju_client()
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                permanent=True, jes_enabled=True)
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                with patch.object(bs_manager, '_should_dump',
                                  return_value=True):
                    bs_manager.dump_all_logs()

        clients = dict((c[1][0].env.environment, c[1][0])
                       for c in del_mock.mock_calls)
        self.assertItemsEqual(
            [call(client, os.path.join(log_dir, 'name'), None, {}),
             call(clients['controller'], os.path.join(log_dir, 'controller'),
                  'foo/models/cache.yaml', {})],
            del_mock.mock_calls)

    def test_dump_all_multi_model_iter_failure(self):
        client = fake_juju_client()
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                permanent=True, jes_enabled=True)
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                with patch.object(client, 'iter_model_clients',
                                  side_effect=Exception):
                    with patch.object(bs_manager, '_should_dump',
                                      return_value=True):
                        bs_manager.dump_all_logs()

        clients = dict((c[1][0].env.environment, c[1][0])
                       for c in del_mock.mock_calls)

        self.assertItemsEqual(
            [call(client, os.path.join(log_dir, 'name'), None, {}),
             call(clients['controller'], os.path.join(log_dir, 'controller'),
                  'foo/models/cache.yaml', {})],
            del_mock.mock_calls)

    def test_dump_all_logs_uses_known_hosts(self):
        client = fake_juju_client_optional_jes(jes_enabled=False)
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                False, False)
            bs_manager.known_hosts['2'] = 'example.org'
            client.bootstrap()
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                with patch.object(bs_manager, '_should_dump',
                                  return_value=True):
                    bs_manager.dump_all_logs()
        del_mock.assert_called_once_with(
            client, os.path.join(log_dir, 'name'),
            'foo/environments/name.jenv', {
                '2': 'example.org',
                })

    def test_dump_all_logs_ignores_soft_deadline(self):

        def do_check(client, *args, **kwargs):
            with client.check_timeouts():
                pass

        client = fake_juju_client()
        client._backend._past_deadline = True
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            with patch.object(bs_manager, '_should_dump', return_value=True,
                              autospec=True):
                with patch('deploy_stack.dump_env_logs_known_hosts',
                           side_effect=do_check, autospec=True):
                    bs_manager.dump_all_logs()

    def test_runtime_context_raises_logged_exception(self):
        client = fake_juju_client()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            True, True)
        bs_manager.has_controller = True
        test_error = Exception("Some exception")
        test_error.output = "a stdout value"
        test_error.stderr = "a stderr value"
        with patch.object(bs_manager, 'dump_all_logs', autospec=True):
            with patch('deploy_stack.safe_print_status',
                       autospec=True) as sp_mock:
                with self.assertRaises(LoggedException) as err_ctx:
                    with bs_manager.runtime_context([]):
                        raise test_error
                    self.assertIs(err_ctx.exception.exception, test_error)
        self.assertIn("a stdout value", self.log_stream.getvalue())
        self.assertIn("a stderr value", self.log_stream.getvalue())
        sp_mock.assert_called_once_with(client)

    def test_runtime_context_raises_logged_exception_no_controller(self):
        client = fake_juju_client()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            True, True)
        bs_manager.has_controller = False
        test_error = Exception("Some exception")
        test_error.output = "a stdout value"
        test_error.stderr = "a stderr value"
        with patch.object(bs_manager, 'dump_all_logs', autospec=True):
            with patch('deploy_stack.safe_print_status',
                       autospec=True) as sp_mock:
                with self.assertRaises(LoggedException) as err_ctx:
                    with bs_manager.runtime_context([]):
                        raise test_error
                    self.assertIs(err_ctx.exception.exception, test_error)
        self.assertIn("a stdout value", self.log_stream.getvalue())
        self.assertIn("a stderr value", self.log_stream.getvalue())
        self.assertEqual(0, sp_mock.call_count)
        self.assertIn(
            "Client lost controller, not calling status",
            self.log_stream.getvalue())

    def test_runtime_context_looks_up_host(self):
        client = fake_juju_client()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            True, True)
        with patch.object(bs_manager, 'dump_all_logs', autospec=True):
            with bs_manager.runtime_context([]):
                self.assertEqual({
                    '0': '0.example.com'}, bs_manager.known_hosts)

    @patch('deploy_stack.dump_env_logs_known_hosts', autospec=True)
    def test_runtime_context_addable_machines_no_known_hosts(self, del_mock):
        client = fake_juju_client()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            True, True)
        bs_manager.known_hosts = {}
        with patch.object(bs_manager.client, 'add_ssh_machines',
                          autospec=True) as ads_mock:
            with patch.object(bs_manager, 'dump_all_logs', autospec=True):
                with bs_manager.runtime_context(['baz']):
                    ads_mock.assert_called_once_with(['baz'])

    @patch('deploy_stack.BootstrapManager.dump_all_logs', autospec=True)
    def test_runtime_context_addable_machines_with_known_hosts(self, dal_mock):
        client = fake_juju_client()
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            bs_manager.known_hosts['0'] = 'example.org'
            with patch.object(bs_manager.client, 'add_ssh_machines',
                              autospec=True) as ads_mock:
                with bs_manager.runtime_context(['baz']):
                    ads_mock.assert_called_once_with(['baz'])

    @contextmanager
    def no_controller_manager(self):
        client = fake_juju_client()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, self.juju_home, False,
            True, True)
        bs_manager.has_controller = False
        with patch('deploy_stack.safe_print_status',
                   autospec=True) as sp_mock:
            with patch.object(
                    client, 'juju', wrap=client.juju,
                    return_value=make_fake_juju_return()) as juju_mock:
                with patch.object(client, 'get_juju_output',
                                  wraps=client.get_juju_output) as gjo_mock:
                    with patch.object(bs_manager, '_should_dump',
                                      return_value=True, autospec=True):
                        with patch('deploy_stack.get_remote_machines',
                                   return_value={}):
                                yield bs_manager
        self.assertEqual(sp_mock.call_count, 0)
        self.assertEqual(0, gjo_mock.call_count)
        juju_mock.assert_called_once_with(
            'kill-controller', ('name', '-y'), check=True, include_e=False,
            timeout=600)

    def test_runtime_context_no_controller(self):
        with self.no_controller_manager() as bs_manager:
            with bs_manager.runtime_context([]):
                pass

    def test_runtime_context_no_controller_exception(self):
        with self.no_controller_manager() as bs_manager:
            with self.assertRaises(LoggedException):
                with bs_manager.runtime_context([]):
                    raise ValueError

    @contextmanager
    def logged_exception_bs_manager(self):
        client = fake_juju_client()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            juju_home = os.path.join(root, 'juju-home')
            os.mkdir(juju_home)
            client.env.juju_home = juju_home
            yield bs_manager

    def test_booted_context_handles_logged_exception(self):
        with self.logged_exception_bs_manager() as bs_manager:
            with self.assertRaises(SystemExit):
                with patch.object(bs_manager, 'dump_all_logs'):
                    with bs_manager.booted_context(False):
                        raise LoggedException()

    def test_booted_context_raises_logged_exception(self):
        with self.logged_exception_bs_manager() as bs_manager:
            bs_manager.logged_exception_exit = False
            with self.assertRaises(LoggedException):
                with patch.object(bs_manager, 'dump_all_logs'):
                    with bs_manager.booted_context(False):
                        raise LoggedException()

    def test_booted_context_omits_supported(self):
        client = fake_juju_client()
        client.env.juju_home = use_context(self, temp_dir())
        client.bootstrap_replaces = {'agent-version', 'series',
                                     'bootstrap-host', 'agent-stream'}
        ue_mock = use_context(
            self, patch('deploy_stack.update_env', wraps=update_env))
        wfp_mock = use_context(
            self, patch('deploy_stack.wait_for_port', autospec=True))
        bs_manager = BootstrapManager(
            'bar', client, client, 'bootstrap.example.org',
            [], 'wacky', 'url', 'devel', None, client.env.juju_home, False,
            True, True)
        with patch.object(bs_manager, 'runtime_context'):
            with bs_manager.booted_context([]):
                pass
        self.assertEqual({
            'name': 'bar',
            'default-series': 'wacky',
            'agent-metadata-url': 'url',
            'type': 'foo',
            'region': 'bar',
            'test-mode': True,
            }, client.get_model_config())
        ue_mock.assert_called_with(client.env, 'bar', agent_url='url',
                                   region=None)
        wfp_mock.assert_called_once_with(
            'bootstrap.example.org', 22, timeout=120)

    @contextmanager
    def booted_to_bootstrap(self, bs_manager):
        """Preform patches to focus on the call to bootstrap."""
        with patch.object(bs_manager, 'dump_all_logs'):
            with patch.object(bs_manager, 'runtime_context'):
                with patch.object(
                        bs_manager.client, 'juju',
                        return_value=make_fake_juju_return()):
                    with patch.object(bs_manager.client, 'bootstrap') as mock:
                        yield mock

    def test_booted_context_kwargs(self):
        client = fake_juju_client()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            juju_home = os.path.join(root, 'juju-home')
            os.mkdir(juju_home)
            client.env.juju_home = juju_home
            with self.booted_to_bootstrap(bs_manager) as bootstrap_mock:
                with bs_manager.booted_context(False, to='test'):
                    bootstrap_mock.assert_called_once_with(
                        upload_tools=False, to='test', bootstrap_series=None)
            with self.booted_to_bootstrap(bs_manager) as bootstrap_mock:
                with bs_manager.existing_booted_context(False, to='test'):
                    bootstrap_mock.assert_called_once_with(
                        upload_tools=False, to='test', bootstrap_series=None)

    def test_runtime_context_teardown_ignores_soft_deadline(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        soft_deadline = datetime(2015, 1, 2, 3, 4, 6)
        now = soft_deadline + timedelta(seconds=1)
        client = ModelClient(env, None, None)
        tear_down_client = ModelClient(env, None, None)

        def do_check_client(*args, **kwargs):
            with client.check_timeouts():
                return iter([])

        def do_check_teardown_client(*args, **kwargs):
            with tear_down_client.check_timeouts():
                return iter([])

        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, tear_down_client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            bs_manager.known_hosts['0'] = 'example.org'
            with patch.object(bs_manager.client, 'juju',
                              side_effect=do_check_client, autospec=True):
                with patch.object(bs_manager.client, 'iter_model_clients',
                                  side_effect=do_check_client, autospec=True,
                                  ):
                    with patch.object(bs_manager, 'tear_down',
                                      do_check_teardown_client):
                        with patch.object(client._backend, '_now',
                                          return_value=now):
                            with bs_manager.runtime_context(['baz']):
                                client._backend.soft_deadline = soft_deadline
                                td_backend = tear_down_client._backend
                                td_backend.soft_deadline = soft_deadline

    @contextmanager
    def make_bootstrap_manager(self):
        client = fake_juju_client()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            yield bs_manager

    def test_top_context_dumps_timings(self):
        with self.make_bootstrap_manager() as bs_manager:
            with patch('deploy_stack.dump_juju_timings') as djt_mock:
                with bs_manager.top_context():
                    pass
        djt_mock.assert_called_once_with(bs_manager.client, bs_manager.log_dir)

    def test_top_context_dumps_timings_on_exception(self):
        with self.make_bootstrap_manager() as bs_manager:
            with patch('deploy_stack.dump_juju_timings') as djt_mock:
                with self.assertRaises(ValueError):
                    with bs_manager.top_context():
                        raise ValueError
        djt_mock.assert_called_once_with(bs_manager.client, bs_manager.log_dir)

    def test_top_context_no_log_dir_skips_timings(self):
        with self.make_bootstrap_manager() as bs_manager:
            bs_manager.log_dir = None
            with patch('deploy_stack.dump_juju_timings') as djt_mock:
                with bs_manager.top_context():
                    pass
        self.assertEqual(djt_mock.call_count, 0)

    def test_collect_resource_details_collects_expected_details(self):
        controller_uuid = 'eb67e1eb-6c54-45f5-8b6a-b6243be97202'
        members = [
            Machine('0', {'dns-name': '10.0.0.0',
                          'instance-id': 'juju-aaaa-machine-0'}),
            Machine('1', {'dns-name': '10.0.0.1',
                          'instance-id': 'juju-dddd-machine-1'}),
        ]
        client = fake_juju_client()
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)
        result = {
            'controller-uuid': controller_uuid,
            'instances': [(m.info['instance-id'], m.info['dns-name'])
                          for m in members]
        }

        with patch.object(client, 'get_controller_uuid', autospec=True,
                          return_value=controller_uuid):
            with patch.object(client, 'get_controller_members', autospec=True,
                              return_value=members):
                bs_manager.collect_resource_details()
                self.assertEqual(bs_manager.resource_details, result)

    def test_ensure_cleanup(self):
        client = fake_juju_client()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'controller', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            mock_substrate = Mock()
            mock_details = {}
            with patch('deploy_stack.make_substrate_manager', autospec=True)\
                    as msm:
                msm.return_value.__enter__.return_value = mock_substrate
                bs_manager.resource_details = mock_details
                bs_manager.ensure_cleanup()
            mock_substrate.ensure_cleanup.assert_called_once_with(
                mock_details)
            msm.assert_called_once_with(client.env)

    def test_ensure_cleanup_resource_details_empty(self):
        client = fake_juju_client()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'controller', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            with patch('deploy_stack.make_substrate_manager', autospec=True) \
                    as msm:
                rl = bs_manager.ensure_cleanup()
                self.assertEquals(0, msm.call_count)
                self.assertEquals(rl, [])

    def test_ensure_cleanup_substrate_none(self):
        client = fake_juju_client()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'controller', client, client,
                None, [], None, None, None, None, log_dir, False,
                True, True)
            mock_details = {}
            bs_manager.resource_details = mock_details
            with patch('deploy_stack.make_substrate_manager', autospec=True)\
                    as msm:
                msm.return_value.__enter__.return_value = None
                rl = bs_manager.ensure_cleanup()
                self.assertIn("foo is an unknown provider."
                              " Unable to ensure cleanup",
                              self.log_stream.getvalue())
                self.assertEquals(rl, [])


class TestBootContext(FakeHomeTestCase):

    def setUp(self):
        super(TestBootContext, self).setUp()
        self.addContext(patch('sys.stdout'))

    @contextmanager
    def bc_context(self, client, log_dir=None, jes=None, keep_env=False):
        dal_mock = self.addContext(
            patch('deploy_stack.BootstrapManager.dump_all_logs'))
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo', autospec=True))
        self.addContext(patch(
            'deploy_stack.BootstrapManager.collect_resource_details',
            autospec=True))
        models = [{'name': 'controller'}, {'name': 'bar'}]
        self.addContext(patch.object(client, '_get_models',
                                     return_value=models, autospec=True))
        if jes:
            output = jes
        else:
            output = ''
        po_count = 0
        with patch('subprocess.Popen', autospec=True,
                   return_value=FakePopen(output, '', 0)) as po_mock:
            with patch('deploy_stack.BootstrapManager.tear_down',
                       autospec=True) as tear_down_mock:
                with patch.object(client, 'kill_controller',
                                  autospec=True) as kill_mock:
                    yield
        self.assertEqual(po_count, po_mock.call_count)
        dal_mock.assert_called_once_with()
        tear_down_count = 0 if keep_env else 1
        self.assertEqual(1, kill_mock.call_count)
        self.assertEqual(tear_down_count, tear_down_mock.call_count)

    def test_bootstrap_context(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  'log_dir', keep_env=False,
                                  upload_tools=False):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'paas/qux', 'bar', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-controllers'), 1)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-models', '-c', 'bar'), 2)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:controller',
            '--format', 'yaml'), 3)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:bar',
            '--format', 'yaml'), 4)

    def test_keep_env(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, keep_env=True, jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  None, keep_env=True, upload_tools=False):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'paas/qux', 'bar', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-controllers'), 1)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-models', '-c', 'bar'), 2)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:controller',
            '--format', 'yaml'), 3)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:bar',
            '--format', 'yaml'), 4)

    def test_upload_tools(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  None, keep_env=False, upload_tools=True):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'bootstrap', '--upload-tools',
            '--constraints', 'mem=2G', 'paas/qux', 'bar', '--config',
            config_file.name, '--default-model', 'bar'), 0)

    def test_calls_update_env(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '2.3', 'path')
        ue_mock = self.addContext(
            patch('deploy_stack.update_env', wraps=update_env))
        with self.bc_context(client, jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], 'wacky', 'url',
                                  'devel', None, keep_env=False,
                                  upload_tools=False):
                    pass
        ue_mock.assert_called_with(
            client.env, 'bar', agent_url='url', agent_stream='devel',
            series='wacky', bootstrap_host=None, region=None)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'bootstrap', '--constraints', 'mem=2G',
            'paas/qux', 'bar', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '2.3',
            '--bootstrap-series', 'wacky'), 0)

    def test_with_bootstrap_failure(self):

        class FakeException(Exception):
            """A sentry exception to be raised by bootstrap."""

        client = ModelClient(JujuData(
            'foo', {'type': 'paas'}), '1.23', 'path')
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo'))
        self.addContext(patch('subprocess.check_call'))
        tear_down_mock = self.addContext(
            patch('deploy_stack.BootstrapManager.tear_down', autospec=True))
        kill_mock = self.addContext(
            patch('jujupy.ModelClient.kill_controller', autospec=True))
        po_mock = self.addContext(patch(
            'subprocess.Popen', autospec=True,
            return_value=FakePopen('kill-controller', '', 0)))
        self.addContext(patch('deploy_stack.wait_for_port'))
        fake_exception = FakeException()
        self.addContext(patch.object(client, 'bootstrap',
                                     side_effect=fake_exception))
        crl_mock = self.addContext(patch('deploy_stack.copy_remote_logs'))
        al_mock = self.addContext(patch('deploy_stack.archive_logs'))
        le_mock = self.addContext(patch('logging.exception'))
        with self.assertRaises(SystemExit):
            with boot_context('bar', client, 'baz', [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=True):
                pass
        le_mock.assert_called_once_with(fake_exception)
        self.assertEqual(crl_mock.call_count, 1)
        call_args = crl_mock.call_args[0]
        self.assertIsInstance(call_args[0], _Remote)
        self.assertEqual(call_args[0].get_address(), 'baz')
        self.assertEqual(call_args[1], 'log_dir')
        al_mock.assert_called_once_with('log_dir')
        self.assertEqual(0, tear_down_mock.call_count)
        self.assertEqual(2, kill_mock.call_count)
        self.assertEqual(0, po_mock.call_count)

    def test_jes(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.26', 'path')
        with self.bc_context(client, 'log_dir', jes=KILL_CONTROLLER):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False):
                pass

    def test_region(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes='kill-controller'):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False,
                              region='steve'):
                pass
        self.assertEqual('steve', client.env.get_region())

    def test_status_error_raises(self):
        """An error on final show-status propagates so an assess will fail."""
        error = subprocess.CalledProcessError(1, ['juju'], '')
        effects = [None, None, None, None, None, None, error]
        cc_mock = self.addContext(patch('subprocess.check_call', autospec=True,
                                        side_effect=effects))
        client = ModelClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes='kill-controller'):
            with observable_temp_file() as config_file:
                with self.assertRaises(subprocess.CalledProcessError) as ctx:
                    with boot_context('bar', client, None, [], None, None,
                                      None, 'log_dir', keep_env=False,
                                      upload_tools=False):
                        pass
                self.assertIs(ctx.exception, error)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'paas/qux', 'bar', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-controllers'), 1)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'list-models', '-c', 'bar'), 2)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:controller',
            '--format', 'yaml'), 3)
        assert_juju_call(self, cc_mock, client, (
            'path', '--show-log', 'show-status', '-m', 'bar:bar',
            '--format', 'yaml'), 4)


class TestDeployJobParseArgs(FakeHomeTestCase):

    def test_deploy_job_parse_args(self):
        args = deploy_job_parse_args(['foo', 'bar/juju', 'baz', 'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            debug=False,
            env='foo',
            temp_env_name='qux',
            keep_env=False,
            logs='baz',
            machine=[],
            juju_bin='bar/juju',
            series=None,
            upgrade=False,
            verbose=logging.INFO,
            upload_tools=False,
            with_chaos=0,
            jes=False,
            region=None,
            to=None,
            deadline=None,
            controller_host=None,
            use_charmstore=False,
            existing=None
        ))

    def test_upload_tools(self):
        args = deploy_job_parse_args(
            ['foo', 'bar/juju', 'baz', 'qux', '--upload-tools'])
        self.assertEqual(args.upload_tools, True)

    def test_agent_stream(self):
        args = deploy_job_parse_args(
            ['foo', 'bar/juju', 'baz', 'qux', '--agent-stream', 'wacky'])
        self.assertEqual('wacky', args.agent_stream)

    def test_jes(self):
        args = deploy_job_parse_args(
            ['foo', 'bar/juju', 'baz', 'qux', '--jes'])
        self.assertIs(args.jes, True)

    def test_use_charmstore(self):
        args = deploy_job_parse_args(
            ['foo', 'bar/juju', 'baz', 'qux', '--use-charmstore'])
        self.assertIs(args.use_charmstore, True)


class TestWaitForStateServerToShutdown(FakeHomeTestCase):

    def test_openstack(self):
        env = JujuData('foo', {
            'type': 'openstack',
            'region': 'lcy05',
            'username': 'steve',
            'password': 'password1',
            'tenant-name': 'steven',
            'auth-url': 'http://example.org',
            }, self.juju_home)
        client = fake_juju_client(env=env)
        with patch('deploy_stack.wait_for_port', autospec=True) as wfp_mock:
            with patch('deploy_stack.has_nova_instance', autospec=True,
                       return_value=False) as hni_mock:
                with patch('deploy_stack.print_now', autospec=True) as pn_mock:
                    wait_for_state_server_to_shutdown(
                        'example.org', client, 'i-255')
        self.assertEqual(pn_mock.mock_calls, [
            call('Waiting for port to close on example.org'),
            call('Closed.'),
            call('i-255 was removed from nova list'),
            ])
        wfp_mock.assert_called_once_with('example.org', 17070, closed=True,
                                         timeout=60)
        hni_mock.assert_called_once_with(client.env, 'i-255')


class TestErrorIfUnclean(FakeHomeTestCase):
    def test_empty_unclean_resources(self):
        uncleaned_resources = []
        error_if_unclean(uncleaned_resources)
        self.assertEquals(self.log_stream.getvalue(), '')

    def test_contain_unclean_resources(self):
        uncleaned_resources = [
                {
                    'resource': 'instances',
                    'errors': [('ifoo', 'err-msg'), ('ibar', 'err-msg')]
                },
                {
                    'resource': 'security groups',
                    'errors': [('sg-bar', 'err-msg')]
                }
            ]
        error_if_unclean(uncleaned_resources)
        self.assertListEqual(self.log_stream.getvalue().splitlines(), [
            "CRITICAL Following resource requires manual cleanup",
            "CRITICAL instances",
            "CRITICAL \tifoo: err-msg",
            "CRITICAL \tibar: err-msg",
            "CRITICAL security groups",
            "CRITICAL \tsg-bar: err-msg"
        ])

    def test_unclean_resources_without_sg_error(self):
        uncleaned_resources = [
                {
                    'resource': 'instances',
                    'errors': [('ifoo', 'err-msg'), ('ibar', 'err-msg')]
                },
        ]
        error_if_unclean(uncleaned_resources)
        self.assertListEqual(self.log_stream.getvalue().splitlines(), [
            "CRITICAL Following resource requires manual cleanup",
            "CRITICAL instances",
            "CRITICAL \tifoo: err-msg",
            "CRITICAL \tibar: err-msg",
        ])

    def test_unclean_resources_without_instances_error(self):
        uncleaned_resources = [
                {
                    'resource': 'security groups',
                    'errors': [('sg-bar', 'err-msg')]
                }
            ]
        error_if_unclean(uncleaned_resources)
        self.assertListEqual(self.log_stream.getvalue().splitlines(), [
            "CRITICAL Following resource requires manual cleanup",
            "CRITICAL security groups",
            "CRITICAL \tsg-bar: err-msg"
        ])
