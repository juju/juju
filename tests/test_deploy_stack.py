from argparse import (
    Namespace,
)
from contextlib import contextmanager
import json
import logging
import os
import subprocess
import sys
from unittest import (
    skipIf,
)

from mock import (
    call,
    MagicMock,
    patch,
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
    deploy_dummy_stack,
    deploy_job,
    _deploy_job,
    deploy_job_parse_args,
    destroy_environment,
    dump_env_logs,
    dump_juju_timings,
    iter_remote_machines,
    get_remote_machines,
    GET_TOKEN_SCRIPT,
    safe_print_status,
    retain_config,
    update_env,
)
from jujuconfig import (
    get_environments_path,
    get_jenv_path,
    get_juju_home,
    )
from jujupy import (
    EnvJujuClient,
    EnvJujuClient1X,
    get_cache_path,
    get_timeout_prefix,
    get_timeout_path,
    JujuData,
    KILL_CONTROLLER,
    SimpleEnvironment,
    Status,
)
from remote import (
    _Remote,
    remote_from_address,
    SSHRemote,
)
from tests import (
    FakeHomeTestCase,
    temp_os_env,
    use_context,
)
from test_jujupy import (
    assert_juju_call,
    FakeJujuClient,
    FakePopen,
    observable_temp_file,
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

    def test_destroy_environment(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client,
                          'destroy_environment', autospec=True) as de_mock:
            with patch('deploy_stack.destroy_job_instances',
                       autospec=True) as dji_mock:
                destroy_environment(client, 'foo')
        self.assertEqual(1, de_mock.call_count)
        self.assertEqual(0, dji_mock.call_count)

    def test_destroy_environment_with_manual_type_aws(self):
        os.environ['AWS_ACCESS_KEY'] = 'fake-juju-ci-testing-key'
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'manual'}), '1.234-76', None)
        with patch.object(client,
                          'destroy_environment', autospec=True) as de_mock:
            with patch('deploy_stack.destroy_job_instances',
                       autospec=True) as dji_mock:
                destroy_environment(client, 'foo')
        self.assertEqual(1, de_mock.call_count)
        dji_mock.assert_called_once_with('foo')

    def test_destroy_environment_with_manual_type_non_aws(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'manual'}), '1.234-76', None)
        with patch.object(client,
                          'destroy_environment', autospec=True) as de_mock:
            with patch('deploy_stack.destroy_job_instances',
                       autospec=True) as dji_mock:
                destroy_environment(client, 'foo')
        self.assertEqual(os.environ.get('AWS_ACCESS_KEY'), None)
        self.assertEqual(1, de_mock.call_count)
        self.assertEqual(0, dji_mock.call_count)

    def test_assess_juju_run(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, None, None)
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
                          return_value=response_ok):
            responses = assess_juju_run(client)
            for machine in responses:
                self.assertFalse(machine.get('ReturnCode', False))
                self.assertIn(machine.get('MachineId'), ["1", "2"])
            self.assertEqual(len(responses), 2)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=response_err):
            with self.assertRaises(ValueError):
                responses = assess_juju_run(client)

    def test_safe_print_status(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, None, None)
        with patch.object(
                client, 'juju', autospec=True,
                side_effect=subprocess.CalledProcessError(
                    1, 'status', 'status error')
        ) as mock:
            safe_print_status(client)
        mock.assert_called_once_with('show-status', ('--format', 'yaml'))

    def test_update_env(self):
        env = SimpleEnvironment('foo', {'type': 'paas'})
        update_env(
            env, 'bar', series='wacky', bootstrap_host='baz',
            agent_url='url', agent_stream='devel')
        self.assertEqual('bar', env.environment)
        self.assertEqual('bar', env.config['name'])
        self.assertEqual('wacky', env.config['default-series'])
        self.assertEqual('baz', env.config['bootstrap-host'])
        self.assertEqual('url', env.config['tools-metadata-url'])
        self.assertEqual('devel', env.config['agent-stream'])
        self.assertNotIn('region', env.config)

    def test_update_env_region(self):
        env = SimpleEnvironment('foo', {'type': 'paas'})
        update_env(env, 'bar', region='region-foo')
        self.assertEqual('region-foo', env.config['region'])

    def test_update_env_region_none(self):
        env = SimpleEnvironment('foo',
                                {'type': 'paas', 'region': 'region-foo'})
        update_env(env, 'bar', region=None)
        self.assertEqual('region-foo', env.config['region'])

    def test_dump_juju_timings(self):
        env = JujuData('foo', {'type': 'bar'})
        client = EnvJujuClient(env, None, None)
        client._backend.juju_timings = {("juju", "op1"): [1],
                                        ("juju", "op2"): [2]}
        expected = {"juju op1": [1], "juju op2": [2]}
        with temp_dir() as fake_dir:
            dump_juju_timings(client, fake_dir)
            with open(os.path.join(fake_dir,
                      'juju_command_times.json')) as out_file:
                file_data = json.load(out_file)
        self.assertEqual(file_data, expected)

    def test_check_token(self):
        env = JujuData('foo', {'type': 'local'})
        client = EnvJujuClient(env, None, None)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'run', autospec=True,
                              return_value='token') as rr_mock:
                check_token(client, 'token', timeout=0)
        rr_mock.assert_called_once_with(GET_TOKEN_SCRIPT)
        self.assertTrue(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Retrieving token.',
             "INFO Token matches expected 'token'"],
            self.log_stream.getvalue().splitlines())

    def test_check_token_not_found(self):
        env = JujuData('foo', {'type': 'local'})
        client = EnvJujuClient(env, None, None)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'run', autospec=True,
                              return_value='') as rr_mock:
                with patch.object(remote, 'get_address',
                                  autospec=True) as ga_mock:
                    with self.assertRaisesRegexp(ValueError, "Token is ''"):
                        check_token(client, 'token', timeout=0)
        self.assertEqual(2, rr_mock.call_count)
        rr_mock.assert_called_with(GET_TOKEN_SCRIPT)
        ga_mock.assert_called_once_with()
        self.assertFalse(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Retrieving token.'],
            self.log_stream.getvalue().splitlines())

    def test_check_token_not_found_juju_ssh_broken(self):
        env = JujuData('foo', {'type': 'local'})
        client = EnvJujuClient(env, None, None)
        remote = SSHRemote(client, 'unit', None, series='xenial')
        with patch('deploy_stack.remote_from_unit', autospec=True,
                   return_value=remote):
            with patch.object(remote, 'run', autospec=True,
                              side_effect=['', 'token']) as rr_mock:
                with patch.object(remote, 'get_address',
                                  autospec=True) as ga_mock:
                    with self.assertRaisesRegexp(ValueError,
                                                 "Token is 'token'"):
                        check_token(client, 'token', timeout=0)
        self.assertEqual(2, rr_mock.call_count)
        rr_mock.assert_called_with(GET_TOKEN_SCRIPT)
        ga_mock.assert_called_once_with()
        self.assertFalse(remote.use_juju_ssh)
        self.assertEqual(
            ['INFO Retrieving token.',
             "INFO Token matches expected 'token'",
             'ERROR juju ssh to unit is broken.'],
            self.log_stream.getvalue().splitlines())

    log_level = logging.DEBUG


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
                            client = EnvJujuClient(env, '1.234-76', None)
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
                            client = EnvJujuClient(env, '1.234-76', None)
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
        client = EnvJujuClient(env, '1.234-76', None)
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
                ' /var/log/syslog'
                ' /var/log/mongodb/mongodb.log'
                ),),
            cc_mock.call_args_list[0][0])
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
                '10.10.0.1:/var/log/syslog',
                '10.10.0.1:/var/log/mongodb/mongodb.log',
                '/foo'),),
            cc_mock.call_args_list[1][0])

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
        self.assertEqual(2, co.call_count)
        self.assertEqual(
            ["DEBUG ssh -o 'User ubuntu' -o 'UserKnownHostsFile /dev/null' "
             "-o 'StrictHostKeyChecking no' -o 'PasswordAuthentication no' "
             "10.10.0.1 'sudo chmod -Rf go+r /var/log/cloud-init*.log "
             "/var/log/juju/*.log /var/lib/juju/containers/juju-*-lxc-*/ "
             "/var/log/syslog /var/log/mongodb/mongodb.log'",
             'WARNING Could not allow access to the juju logs:',
             'WARNING None',
             'WARNING Could not retrieve some or all logs:',
             'WARNING CalledProcessError()'],
            self.log_stream.getvalue().splitlines())

    def test_get_machines_for_logs(self):
        client = EnvJujuClient(
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
        client = EnvJujuClient(
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
        client = EnvJujuClient(
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
            'maas-oauth': 'baz'}
        client = EnvJujuClient(JujuData('cloud', config), '1.23.4', None)
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
        client = EnvJujuClient(
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
        client = EnvJujuClient(
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
        client = EnvJujuClient(env, '2.0.0', '/foo/juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            with patch.object(EnvJujuClient, 'wait_for_started'):
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    deploy_dummy_stack(client, 'centos')
        assert_juju_call(self, cc_mock, client,
                         ('juju', '--show-log', 'set-model-constraints', '-m',
                          'foo', 'tags=MAAS_NIC_1'), 0)

    def test_assess_juju_relations(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, None, '/foo/juju')
        with patch.object(client, 'get_juju_output', side_effect='fake-token',
                          autospec=True):
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    with patch('deploy_stack.check_token',
                               autospec=True) as ct_mock:
                        assess_juju_relations(client)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'set-config', '-m', 'foo', 'dummy-source',
            'token=fake-token'), 0)
        ct_mock.assert_called_once_with(client, 'fake-token')

    def test_deploy_dummy_stack_centos(self):
        client = FakeJujuClient()
        client.bootstrap()
        with patch.object(client, 'deploy', autospec=True) as dp_mock:
            with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                deploy_dummy_stack(client, 'centos7')
        calls = [
            call('/tmp/repo/charms-centos/dummy-source', series='centos7'),
            call('/tmp/repo/charms-centos/dummy-sink', series='centos7')]
        self.assertEqual(dp_mock.mock_calls, calls)

    def test_deploy_dummy_stack_win(self):
        client = FakeJujuClient()
        client.bootstrap()
        with patch.object(client, 'deploy', autospec=True) as dp_mock:
            with temp_os_env('JUJU_REPOSITORY', '/tmp/repo'):
                deploy_dummy_stack(client, 'win2012hvr2')
        calls = [
            call('/tmp/repo/charms-win/dummy-source', series='win2012hvr2'),
            call('/tmp/repo/charms-win/dummy-sink', series='win2012hvr2')]
        self.assertEqual(dp_mock.mock_calls, calls)

    def test_deploy_dummy_stack(self):
        env = JujuData('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, '2.0.0', '/foo/juju')
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {
                'dummy-sink': {'units': {
                    'dummy-sink/0': {'agent-state': 'started'}}
                }
            }
        })

        def output(*args, **kwargs):
            output = {
                ('show-status', '--format', 'yaml'): status,
                ('ssh', 'dummy-sink/0', GET_TOKEN_SCRIPT): 'fake-token',
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
            'juju', '--show-log', 'deploy', '-m', 'foo',
            '/tmp/repo/charms/dummy-source', '--series', 'bar-'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo',
            '/tmp/repo/charms/dummy-sink', '--series', 'bar-'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-relation', '-m', 'foo',
            'dummy-source', 'dummy-sink'), 2)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'expose', '-m', 'foo', 'dummy-sink'), 3)
        self.assertEqual(cc_mock.call_count, 4)
        self.assertEqual(
            [
                call('show-status', '--format', 'yaml', admin=False)
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
            'juju', '--show-log', 'deploy', '-m', 'foo',
            'local:bar-/dummy-source', '--series', 'bar-'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-m', 'foo',
            'local:bar-/dummy-sink', '--series', 'bar-'), 1)


def fake_SimpleEnvironment(name):
    return SimpleEnvironment(name, {})


def fake_EnvJujuClient(env, path=None, debug=None):
    return EnvJujuClient(env=env, version='1.2.3.4', full_path=path)


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
        try:
            self.entered_bootstrap = True
            self.client.env.juju_home = os.path.join(initial_home, 'isolated')
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
        self.tear_down_client.destroy_environment()
        self.torn_down = True

    @contextmanager
    def booted_context(self, upload_tools):
        with self.top_context() as machines:
            with self.bootstrap_context(machines):
                self.client.bootstrap(upload_tools)
            with self.runtime_context(machines):
                yield machines


class TestDeployJob(FakeHomeTestCase):

    @contextmanager
    def ds_cxt(self):
        env = JujuData('foo', {})
        client = fake_EnvJujuClient(env)
        bc_cxt = patch('jujupy.EnvJujuClient.by_version',
                       return_value=client)
        fc_cxt = patch('jujupy.SimpleEnvironment.from_config',
                       return_value=env)
        mgr = MagicMock()
        bm_cxt = patch('deploy_stack.BootstrapManager', autospec=True,
                       return_value=mgr)
        juju_cxt = patch('deploy_stack.EnvJujuClient.juju', autospec=True)
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
            region=None, verbose=False, upgrade=False,
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
            region=None, verbose=False, upgrade=False,
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
            region='region-foo', verbose=False, upgrade=False,
        )
        with self.ds_cxt() as (client, bm_mock):
            with patch('deploy_stack.assess_juju_relations',
                       autospec=True):
                with patch('subprocess.Popen', autospec=True,
                           return_value=FakePopen('', '', 0)):
                    _deploy_job(args, 'local:trusty/', 'trusty')
                    jes = client.is_jes_enabled()
        bm_mock.assert_called_once_with(
            'foo', client, client, None, None, 'trusty', None, None,
            'region-foo', 'log', False, permanent=jes, jes_enabled=jes)

    def test_deploy_job_changes_series_with_win(self):
        args = Namespace(
            series='windows', temp_env_name=None, env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            region=None, verbose=None)
        with patch('deploy_stack.deploy_job_parse_args', return_value=args,
                   autospec=True):
            with patch('deploy_stack._deploy_job', autospec=True) as ds_mock:
                deploy_job()
        ds_mock.assert_called_once_with(args, 'windows', 'trusty')

    def test_deploy_job_changes_series_with_centos(self):
        args = Namespace(
            series='centos', temp_env_name=None, env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            region=None, verbose=None)
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
        'juju', '--show-log', 'show-status', '-m', 'foo', '--format', 'yaml')
    GET_ENV = ('juju', '--show-log', 'get-model-config', '-m', 'foo',
               'tools-metadata-url')

    @classmethod
    def upgrade_output(cls, args, **kwargs):
        status = yaml.safe_dump({
            'machines': {'0': {
                'agent-state': 'started',
                'agent-version': '2.0-alpha3'}},
            'services': {}})
        juju_run_out = json.dumps([
            {"MachineId": "1", "Stdout": "Linux\n"},
            {"MachineId": "2", "Stdout": "Linux\n"}])
        output = {
            cls.STATUS: status,
            cls.RUN_UNAME: juju_run_out,
            cls.GET_ENV: 'testing'
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
                        with patch('jujupy.EnvJujuClient.get_version',
                                   side_effect=lambda cls:
                                   '2.0-alpha3-arch-series'):
                            yield (co_mock, cc_mock)

    def test_assess_upgrade(self):
        env = JujuData('foo', {'type': 'foo'})
        old_client = EnvJujuClient(env, None, '/foo/juju')
        with self.upgrade_mocks() as (co_mock, cc_mock):
            assess_upgrade(old_client, '/bar/juju')
        new_client = EnvJujuClient(env, None, '/bar/juju')
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'upgrade-juju', '-m', 'foo', '--version',
            '2.0-alpha3'), 0)
        self.assertEqual(cc_mock.call_count, 1)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 0)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 1)
        assert_juju_call(self, co_mock, new_client, self.STATUS, 2)
        self.assertEqual(co_mock.call_count, 3)

    def test_mass_timeout(self):
        config = {'type': 'foo'}
        old_client = EnvJujuClient(JujuData('foo', config), None, '/foo/juju')
        with self.upgrade_mocks():
            with patch.object(EnvJujuClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
            wfv_mock.assert_called_once_with('2.0-alpha3', 600)
            config['type'] = 'maas'
            with patch.object(EnvJujuClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
        wfv_mock.assert_called_once_with('2.0-alpha3', 1200)


class TestBootstrapManager(FakeHomeTestCase):

    def test_from_args(self):
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host='example.org', machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True)
        with patch.object(SimpleEnvironment, 'from_config') as fc_mock:
            with patch.object(EnvJujuClient, 'by_version') as bv_mock:
                bs_manager = BootstrapManager.from_args(args)
        fc_mock.assert_called_once_with('foo')
        bv_mock.assert_called_once_with(fc_mock.return_value, 'bar',
                                        debug=True)
        self.assertEqual('baz', bs_manager.temp_env_name)
        self.assertIs(bv_mock.return_value, bs_manager.client)
        self.assertIs(bv_mock.return_value, bs_manager.tear_down_client)
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

    def test_jes_not_permanent(self):
        with self.assertRaisesRegexp(ValueError, 'Cannot set permanent False'
                                     ' if jes_enabled is True.'):
            BootstrapManager(
                jes_enabled=True, permanent=False,
                temp_env_name=None, client=None, tear_down_client=None,
                bootstrap_host=None, machines=[], series=None, agent_url=None,
                agent_stream=None, region=None, log_dir=None, keep_env=None)

    def test_aws_machines_updates_bootstrap_host(self):
        client = FakeJujuClient()
        client.env.config['type'] = 'manual'
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)
        with patch('deploy_stack.run_instances',
                   return_value=[('foo', 'aws.example.org')]):
            with patch('deploy_stack.destroy_job_instances'):
                with bs_manager.aws_machines():
                    self.assertEqual({'0': 'aws.example.org'},
                                     bs_manager.known_hosts)

    def test_from_args_no_host(self):
        args = Namespace(
            env='foo', juju_bin='bar', debug=True, temp_env_name='baz',
            bootstrap_host=None, machine=['example.com'],
            series='angsty', agent_url='qux', agent_stream='escaped',
            region='eu-west-northwest-5', logs='pine', keep_env=True)
        with patch.object(SimpleEnvironment, 'from_config'):
            with patch.object(EnvJujuClient, 'by_version'):
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

    def test_bootstrap_context_tear_down(self):
        client = FakeJujuClient()
        client.env.juju_home = use_context(self, temp_dir())
        initial_home = client.env.juju_home
        bs_manager = BootstrapManager(
            'foobar', client, client, None, [], None, None, None, None,
            client.env.juju_home, False, False, False)

        def check_config(client_, jes_enabled, try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            jenv_path = get_jenv_path(client.env.juju_home, 'foobar')
            self.assertFalse(os.path.exists(jenv_path))
            environments_path = get_environments_path(client.env.juju_home)
            self.assertTrue(os.path.isfile(environments_path))
            self.assertNotEqual(initial_home, client.env.juju_home)

        ije_cxt = patch.object(client, 'is_jes_enabled')
        with patch('deploy_stack.tear_down',
                   side_effect=check_config) as td_mock, ije_cxt:
            with bs_manager.bootstrap_context([]):
                td_mock.assert_called_once_with(client, False, try_jes=True)

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

        def check_config(client_, jes_enabled, try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            self.assertTrue(os.path.isfile(jenv_path))
            environments_path = get_environments_path(client.env.juju_home)
            self.assertFalse(os.path.exists(environments_path))
            self.assertEqual(initial_home, client.env.juju_home)

        with patch('deploy_stack.tear_down',
                   side_effect=check_config) as td_mock:
            with bs_manager.bootstrap_context([]):
                td_mock.assert_called_once_with(client, False, try_jes=False)

    def test_bootstrap_context_tear_down_client(self):
        client = self.make_client()
        tear_down_client = self.make_client()
        tear_down_client.env = client.env
        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client, None, [], None, None, None,
            None, client.env.juju_home, False, False, False)

        def check_config(client_, jes_enabled, try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            tear_down_client.is_jes_enabled.assert_called_once_with()

        with patch('deploy_stack.tear_down',
                   side_effect=check_config) as td_mock:
            with bs_manager.bootstrap_context([]):
                td_mock.assert_called_once_with(tear_down_client,
                                                False, try_jes=True)

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

        def check_config(client_, jes_enabled, try_jes=False):
            self.assertEqual(0, client.is_jes_enabled.call_count)
            tear_down_client.is_jes_enabled.assert_called_once_with()

        with patch('deploy_stack.tear_down',
                   side_effect=check_config) as td_mock:
            with bs_manager.bootstrap_context([]):
                td_mock.assert_called_once_with(tear_down_client, False,
                                                try_jes=False)

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
        client = FakeJujuClient()
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
        client = FakeJujuClient()
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

    def test_tear_down_requires_same_env(self):
        client = self.make_client()
        client.env.juju_home = 'foobar'
        tear_down_client = self.make_client()
        tear_down_client.env.juju_home = 'barfoo'
        bs_manager = BootstrapManager(
            'foobar', client, tear_down_client,
            None, [], None, None, None, None, client.env.juju_home, False,
            False, False)

        def check_home(foo, bar, try_jes):
            self.assertEqual(client.env.juju_home,
                             tear_down_client.env.juju_home)

        with self.assertRaisesRegexp(AssertionError,
                                     'Tear down client needs same env'):
            with patch('deploy_stack.tear_down', autospec=True,
                       side_effect=check_home):
                bs_manager.tear_down()
        self.assertEqual('barfoo', tear_down_client.env.juju_home)

    def test_dump_all_no_jes_one_model(self):
        client = FakeJujuClient()
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
        client = FakeJujuClient(jes_enabled=True)
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                permanent=True, jes_enabled=True)
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                bs_manager.dump_all_logs()

        clients = dict((c[1][0].env.environment, c[1][0])
                       for c in del_mock.mock_calls)

        self.assertItemsEqual(
            [call(client, os.path.join(log_dir, 'name'), None, {}),
             call(clients['admin'], os.path.join(log_dir, 'admin'),
                  'foo/models/cache.yaml', {})],
            del_mock.mock_calls)

    def test_dump_all_multi_model_iter_failure(self):
        client = FakeJujuClient(jes_enabled=True)
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                permanent=True, jes_enabled=True)
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                with patch.object(client, 'iter_model_clients',
                                  side_effect=Exception):
                    bs_manager.dump_all_logs()

        clients = dict((c[1][0].env.environment, c[1][0])
                       for c in del_mock.mock_calls)

        self.assertItemsEqual(
            [call(client, os.path.join(log_dir, 'name'), None, {}),
             call(clients['admin'], os.path.join(log_dir, 'admin'),
                  'foo/models/cache.yaml', {})],
            del_mock.mock_calls)

    def test_dump_all_logs_uses_known_hosts(self):
        client = FakeJujuClient()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                False, False)
            bs_manager.known_hosts['2'] = 'example.org'
            client.bootstrap()
            with patch('deploy_stack.dump_env_logs_known_hosts') as del_mock:
                bs_manager.dump_all_logs()
        del_mock.assert_called_once_with(
            client, os.path.join(log_dir, 'name'),
            'foo/environments/name.jenv', {
                '2': 'example.org',
                })

    def test_runtime_context_looks_up_host(self):
        client = FakeJujuClient()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            False, False)
        with patch.object(bs_manager, 'dump_all_logs', autospec=True):
            with bs_manager.runtime_context([]):
                self.assertEqual({
                    '0': '0.example.com'}, bs_manager.known_hosts)

    @patch('deploy_stack.dump_env_logs_known_hosts', autospec=True)
    def test_runtime_context_addable_machines_no_known_hosts(self, del_mock):
        client = FakeJujuClient()
        client.bootstrap()
        bs_manager = BootstrapManager(
            'foobar', client, client,
            None, [], None, None, None, None, client.env.juju_home, False,
            False, False)
        bs_manager.known_hosts = {}
        with patch.object(bs_manager.client, 'add_ssh_machines',
                          autospec=True) as ads_mock:
            with patch.object(bs_manager, 'dump_all_logs', autospec=True):
                with bs_manager.runtime_context(['baz']):
                    ads_mock.assert_called_once_with(['baz'])

    @patch('deploy_stack.BootstrapManager.dump_all_logs', autospec=True)
    def test_runtime_context_addable_machines_with_known_hosts(self, dal_mock):
        client = FakeJujuClient()
        client.bootstrap()
        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                False, False)
            bs_manager.known_hosts['0'] = 'example.org'
            with patch.object(bs_manager.client, 'add_ssh_machines',
                              autospec=True) as ads_mock:
                with bs_manager.runtime_context(['baz']):
                    ads_mock.assert_called_once_with(['baz'])

    def test_booted_context_handles_logged_exception(self):
        client = FakeJujuClient()
        with temp_dir() as root:
            log_dir = os.path.join(root, 'log-dir')
            os.mkdir(log_dir)
            bs_manager = BootstrapManager(
                'foobar', client, client,
                None, [], None, None, None, None, log_dir, False,
                False, False)
            juju_home = os.path.join(root, 'juju-home')
            os.mkdir(juju_home)
            client.env.juju_home = juju_home
            with self.assertRaises(SystemExit):
                with patch.object(bs_manager, 'dump_all_logs'):
                    with bs_manager.booted_context(False):
                        raise LoggedException()

    def test_booted_context_omits_supported(self):
        client = FakeJujuClient(jes_enabled=True)
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
            'tools-metadata-url': 'url',
            'type': 'foo',
            }, client.get_model_config())
        ue_mock.assert_called_with(client.env, 'bar', agent_url='url',
                                   region=None)
        wfp_mock.assert_called_once_with(
            'bootstrap.example.org', 22, timeout=120)


class TestBootContext(FakeHomeTestCase):

    def setUp(self):
        super(TestBootContext, self).setUp()
        self.addContext(patch('sys.stdout'))

    def addContext(self, cxt):
        """Enter context manager for the remainder of the test, then leave.

        :return: The value emitted by cxt.__enter__.
        """
        return use_context(self, cxt)

    @contextmanager
    def bc_context(self, client, log_dir=None, jes=None, keep_env=False):
        dal_mock = self.addContext(
            patch('deploy_stack.BootstrapManager.dump_all_logs'))
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo', autospec=True))
        c_mock = self.addContext(patch('subprocess.call', autospec=True,
                                 return_value=0))
        if jes:
            output = jes
            po_count = 0
        else:
            output = ''
            po_count = 2
        with patch('subprocess.Popen', autospec=True,
                   return_value=FakePopen(output, '', 0)) as po_mock:
            yield
        for help_index in range(po_count):
            assert_juju_call(self, po_mock, client, (
                'juju', '--show-log', 'help', 'commands'),
                call_index=help_index)
        self.assertEqual(po_count, po_mock.call_count)
        dal_mock.assert_called_once_with()
        if keep_env:
            tear_down_count = 1
        else:
            tear_down_count = 2
        for call_index in range(tear_down_count):
            if jes:
                assert_juju_call(
                    self, c_mock, client, get_timeout_prefix(600) + (
                        'juju', '--show-log', jes, 'bar', '-y'), call_index)
            else:
                assert_juju_call(
                    self, c_mock, client, get_timeout_prefix(600) + (
                        'juju', '--show-log', 'destroy-environment', 'bar',
                        '-y'), call_index)
        self.assertEqual(tear_down_count, c_mock.call_count)

    def test_bootstrap_context(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  'log_dir', keep_env=False,
                                  upload_tools=False):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'bar', 'paas/qux', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'show-status', '-m', 'bar',
            '--format', 'yaml'), 1)

    def test_bootstrap_context_non_jes(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir'):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--constraints',
            'mem=2G'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'status', '-e', 'bar',
            '--format', 'yaml'), 1)

    def test_keep_env(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, keep_env=True, jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  None, keep_env=True, upload_tools=False):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'bar', 'paas/qux', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'show-status', '-m', 'bar',
            '--format', 'yaml'), 1)

    def test_keep_env_non_jes(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, keep_env=True):
            with boot_context('bar', client, None, [], None, None, None, None,
                              keep_env=True, upload_tools=False):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--constraints',
            'mem=2G'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'status', '-e', 'bar',
            '--format', 'yaml'), 1)

    def test_upload_tools(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
        with self.bc_context(client, jes='kill-controller'):
            with observable_temp_file() as config_file:
                with boot_context('bar', client, None, [], None, None, None,
                                  None, keep_env=False, upload_tools=True):
                    pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '--upload-tools',
            '--constraints', 'mem=2G', 'bar', 'paas/qux', '--config',
            config_file.name, '--default-model', 'bar'), 0)

    def test_upload_tools_non_jes(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client):
            with boot_context('bar', client, None, [], None, None, None, None,
                              keep_env=False, upload_tools=True):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--upload-tools',
            '--constraints', 'mem=2G'), 0)

    def test_calls_update_env_2(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.23', 'path')
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
            'juju', '--show-log', 'bootstrap', '--constraints', 'mem=2G',
            'bar', 'paas/qux', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23',
            '--bootstrap-series', 'wacky'), 0)

    def test_calls_update_env_1(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        ue_mock = self.addContext(
            patch('deploy_stack.update_env', wraps=update_env))
        with self.bc_context(client):
            with boot_context('bar', client, None, [], 'wacky', 'url', 'devel',
                              None, keep_env=False, upload_tools=False):
                pass
        ue_mock.assert_called_with(
            client.env, 'bar', series='wacky', bootstrap_host=None,
            agent_url='url', agent_stream='devel', region=None)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar',
            '--constraints', 'mem=2G'), 0)

    def test_calls_update_env_non_jes(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        ue_mock = self.addContext(
            patch('deploy_stack.update_env', wraps=update_env))
        with self.bc_context(client):
            with boot_context('bar', client, None, [], 'wacky', 'url', 'devel',
                              None, keep_env=False, upload_tools=False):
                pass
        ue_mock.assert_called_with(
            client.env, 'bar', series='wacky', bootstrap_host=None,
            agent_url='url', agent_stream='devel', region=None)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar',
            '--constraints', 'mem=2G'), 0)

    def test_with_bootstrap_failure(self):

        class FakeException(Exception):
            """A sentry exception to be raised by bootstrap."""

        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas'}), '1.23', 'path')
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo'))
        self.addContext(patch('subprocess.check_call'))
        call_mock = self.addContext(patch('subprocess.call', return_value=0))
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
        timeout_path = get_timeout_path()
        assert_juju_call(self, call_mock, client, (
            sys.executable, timeout_path, '600.00', '--',
            'juju', '--show-log', 'kill-controller', 'bar', '-y'
            ), 0)
        assert_juju_call(self, call_mock, client, (
            sys.executable, timeout_path, '600.00', '--',
            'juju', '--show-log', 'kill-controller', 'bar', '-y'
            ), 1)
        self.assertEqual(2, call_mock.call_count)
        self.assertEqual(0, po_mock.call_count)

    def test_with_bootstrap_failure_non_jes(self):

        class FakeException(Exception):
            """A sentry exception to be raised by bootstrap."""

        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo'))
        self.addContext(patch('subprocess.check_call'))
        call_mock = self.addContext(patch('subprocess.call', return_value=0))
        po_mock = self.addContext(patch('subprocess.Popen', autospec=True,
                                        return_value=FakePopen('', '', 0)))
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
        timeout_path = get_timeout_path()
        assert_juju_call(self, call_mock, client, (
            sys.executable, timeout_path, '600.00', '--',
            'juju', '--show-log', 'destroy-environment', 'bar', '-y'
            ), 0)
        assert_juju_call(self, call_mock, client, (
            sys.executable, timeout_path, '600.00', '--',
            'juju', '--show-log', 'destroy-environment', 'bar', '-y'
            ), 1)
        self.assertEqual(2, call_mock.call_count)
        assert_juju_call(self, po_mock, client, (
            'juju', '--show-log', 'help', 'commands'), 0)
        assert_juju_call(self, po_mock, client, (
            'juju', '--show-log', 'help', 'commands'), 1)
        self.assertEqual(2, po_mock.call_count)

    def test_jes(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas', 'region': 'qux'}), '1.26', 'path')
        with self.bc_context(client, 'log_dir', jes=KILL_CONTROLLER):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False):
                pass

    def test_region(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = EnvJujuClient(JujuData(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes='kill-controller'):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False,
                              region='steve'):
                pass
        self.assertEqual('steve', client.env.config['region'])

    def test_region_non_jes(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = EnvJujuClient1X(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir'):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False,
                              region='steve'):
                pass
        self.assertEqual('steve', client.env.config['region'])

    def test_status_error_raises(self):
        """An error on final show-status propogates so an assess will fail."""
        error = subprocess.CalledProcessError(1, ['juju'], '')
        cc_mock = self.addContext(patch('subprocess.check_call', autospec=True,
                                        side_effect=[None, error]))
        client = EnvJujuClient(JujuData(
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
            'juju', '--show-log', 'bootstrap', '--constraints',
            'mem=2G', 'bar', 'paas/qux', '--config', config_file.name,
            '--default-model', 'bar', '--agent-version', '1.23'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'show-status', '-m', 'bar',
            '--format', 'yaml'), 1)


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
