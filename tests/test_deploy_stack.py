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
    patch,
)
import yaml

import deploy_stack
from deploy_stack import (
    archive_logs,
    assess_juju_run,
    boot_context,
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
    assess_upgrade,
    safe_print_status,
    retain_config,
    update_env,
)
from jujupy import (
    EnvJujuClient,
    get_timeout_prefix,
    get_timeout_path,
    SimpleEnvironment,
    Status,
)
from remote import (
    _Remote,
    remote_from_address,
)
from tests import (
    FakeHomeTestCase,
    use_context,
)
from test_jujupy import (
    assert_juju_call,
    FakePopen,
)
from utility import (
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
        client = EnvJujuClient(
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
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'manual'}), '1.234-76', None)
        with patch.object(client,
                          'destroy_environment', autospec=True) as de_mock:
            with patch('deploy_stack.destroy_job_instances',
                       autospec=True) as dji_mock:
                destroy_environment(client, 'foo')
        self.assertEqual(1, de_mock.call_count)
        dji_mock.assert_called_once_with('foo')

    def test_destroy_environment_with_manual_type_non_aws(self):
        client = EnvJujuClient(
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
        env = SimpleEnvironment('foo', {'type': 'nonlocal'})
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
        env = SimpleEnvironment('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, None, None)
        with patch.object(
                client, 'juju', autospec=True,
                side_effect=subprocess.CalledProcessError(
                    1, 'status', 'status error')
        ) as mock:
            safe_print_status(client)
        mock.assert_called_once_with('status', ())

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
        env = SimpleEnvironment('foo', {'type': 'bar'})
        client = EnvJujuClient(env, None, None)
        client.juju_timings = {("juju", "op1"): [1], ("juju", "op2"): [2]}
        expected = {"juju op1": [1], "juju op2": [2]}
        with temp_dir() as fake_dir:
            dump_juju_timings(client, fake_dir)
            with open(os.path.join(fake_dir,
                      'juju_command_times.json')) as out_file:
                file_data = json.load(out_file)
        self.assertEqual(file_data, expected)


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
                            env = SimpleEnvironment('foo',
                                                    {'type': 'nonlocal'})
                            client = EnvJujuClient(env, '1.234-76', None)
                            dump_env_logs(client, '10.10.0.1', artifacts_dir)
            al_mock.assert_called_once_with(artifacts_dir)
            self.assertEqual(
                ['machine-0', 'machine-1', 'machine-2'],
                sorted(os.listdir(artifacts_dir)))
        self.assertEqual(
            (client, '10.10.0.1'), gm_mock.call_args[0])
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
                            env = SimpleEnvironment('foo',
                                                    {'type': 'nonlocal'})
                            client = EnvJujuClient(env, '1.234-76', None)
                            dump_env_logs(client, '10.10.0.1', artifacts_dir)
            al_mock.assert_called_once_with(artifacts_dir)
            self.assertEqual(
                ['machine-2'],
                sorted(os.listdir(artifacts_dir)))
        self.assertEqual(
            (client, '10.10.0.1'), gm_mock.call_args[0])
        self.assertEqual(
            [(self.r2, '%s/machine-2' % artifacts_dir)],
            [cal[0] for cal in crl_mock.call_args_list])
        self.assertEqual(
            ['INFO No ssh, skipping logs for machine-0 using ' + repr(self.r0),
             'INFO No ssh, skipping logs for machine-1 using ' + repr(self.r1),
             'INFO Retrieving logs for machine-2 using ' + repr(self.r2)],
            self.log_stream.getvalue().splitlines())

    def test_dump_env_logs_local_env(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
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
                '10.10.0.1',
                'sudo chmod -Rf go+r /var/log/cloud-init*.log'
                ' /var/log/juju/*.log'
                ' /var/lib/juju/containers/juju-*-lxc-*/'),),
            cc_mock.call_args_list[0][0])
        self.assertEqual(
            (get_timeout_prefix(120) + (
                'scp', '-rC',
                '-o', 'User ubuntu',
                '-o', 'UserKnownHostsFile /dev/null',
                '-o', 'StrictHostKeyChecking no',
                '10.10.0.1:/var/log/cloud-init*.log',
                '10.10.0.1:/var/log/juju/*.log',
                '10.10.0.1:/var/lib/juju/containers/juju-*-lxc-*/',
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
            ['WARNING Could not allow access to the juju logs:',
             'WARNING None',
             'WARNING Could not retrieve some or all logs:',
             'WARNING None'],
            self.log_stream.getvalue().splitlines())

    def test_get_machines_for_logs(self):
        client = EnvJujuClient(
            SimpleEnvironment('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                dns-name: 10.11.12.13
              "1":
                dns-name: 10.11.12.14
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = get_remote_machines(client, None)
        self.assert_machines(
            {'0': '10.11.12.13', '1': '10.11.12.14'}, machines)

    def test_get_machines_for_logs_with_boostrap_host(self):
        client = EnvJujuClient(
            SimpleEnvironment('cloud', {'type': 'ec2'}), '1.23.4', None)
        status = Status.from_text("""\
            machines:
              "0":
                instance-id: pending
            """)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            machines = get_remote_machines(client, '10.11.111.222')
        self.assert_machines({'0': '10.11.111.222'}, machines)

    def test_get_machines_for_logs_with_no_addresses(self):
        client = EnvJujuClient(
            SimpleEnvironment('cloud', {'type': 'ec2'}), '1.23.4', None)
        with patch.object(client, 'get_status', autospec=True,
                          side_effect=Exception):
            machines = get_remote_machines(client, '10.11.111.222')
        self.assert_machines({'0': '10.11.111.222'}, machines)

    @patch('subprocess.check_call')
    def test_get_remote_machines_with_maas(self, cc_mock):
        config = {
            'type': 'maas',
            'name': 'foo',
            'maas-server': 'http://bar/MASS/',
            'maas-oauth': 'baz'}
        client = EnvJujuClient(
            SimpleEnvironment('cloud', config), '1.23.4', None)
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
                machines = get_remote_machines(client, 'node1.maas')
        self.assert_machines(
            {'0': '10.11.12.13', '1': '10.11.12.14'}, machines)

    def test_iter_remote_machines(self):
        client = EnvJujuClient(
            SimpleEnvironment('cloud', {'type': 'ec2'}), '1.23.4', None)
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
            SimpleEnvironment('cloud', {'type': 'ec2'}), '1.23.4', None)
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

    @patch('deploy_stack.check_token')
    def test_deploy_dummy_stack_sets_centos_constraints(self, ct_mock):
        env = SimpleEnvironment('foo', {'type': 'maas'})
        client = EnvJujuClient(env, None, '/foo/juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            with patch.object(EnvJujuClient, 'wait_for_started'):
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    deploy_dummy_stack(client, 'local:centos/foo')
        assert_juju_call(self, cc_mock, client,
                         ('juju', '--show-log', 'set-constraints', '-e', 'foo',
                          'tags=MAAS_NIC_1'), 0)
        self.assertEqual(ct_mock.call_count, 1)

    def test_deploy_dummy_stack(self):
        env = SimpleEnvironment('foo', {'type': 'nonlocal'})
        client = EnvJujuClient(env, None, '/foo/juju')
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
                ('status',): status,
                ('ssh', 'dummy-sink/0', GET_TOKEN_SCRIPT): 'fake-token',
            }
            return output[args]

        with patch.object(client, 'get_juju_output', side_effect=output,
                          autospec=True) as gjo_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('deploy_stack.get_random_string',
                           return_value='fake-token', autospec=True):
                    with patch('sys.stdout', autospec=True):
                        deploy_dummy_stack(client, 'bar-')
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-e', 'foo', 'bar-dummy-source'),
            0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'set', '-e', 'foo', 'dummy-source',
            'token=fake-token'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-e', 'foo', 'bar-dummy-sink'), 2)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-relation', '-e', 'foo',
            'dummy-source', 'dummy-sink'), 3)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'expose', '-e', 'foo', 'dummy-sink'), 4)
        self.assertEqual(cc_mock.call_count, 5)
        self.assertEqual(
            [
                call('status'),
                call('status'),
                call('ssh', 'dummy-sink/0', GET_TOKEN_SCRIPT, timeout=120),
            ],
            gjo_mock.call_args_list)


def fake_SimpleEnvironment(name):
    return SimpleEnvironment(name, {})


def fake_EnvJujuClient(env, path=None, debug=None):
    return EnvJujuClient(env=env, version='1.2.3.4', full_path=path)


class TestDeployJob(FakeHomeTestCase):

    @contextmanager
    def ds_cxt(self):
        env = fake_SimpleEnvironment('foo')
        client = fake_EnvJujuClient(env)
        bc_cxt = patch('jujupy.EnvJujuClient.by_version',
                       return_value=client)
        fc_cxt = patch('jujupy.SimpleEnvironment.from_config',
                       return_value=env)
        boot_cxt = patch('deploy_stack.boot_context', autospec=True)
        juju_cxt = patch('deploy_stack.EnvJujuClient.juju', autospec=True)
        ajr_cxt = patch('deploy_stack.assess_juju_run', autospec=True)
        dds_cxt = patch('deploy_stack.deploy_dummy_stack', autospec=True)
        with bc_cxt, fc_cxt, boot_cxt, juju_cxt, ajr_cxt, dds_cxt:
            yield client

    @skipIf(sys.platform in ('win32', 'darwin'),
            'Not supported on Windown and OS X')
    def test_background_chaos_used(self):
        with self.ds_cxt():
            with patch('deploy_stack.background_chaos',
                       autospec=True) as bc_mock:
                _deploy_job('foo', None, None, '', None, None, None,
                            'log', None, None, None, None, None, None,
                            1, False, False, None)
        self.assertEqual(bc_mock.mock_calls[0][1][0], 'foo')
        self.assertEqual(bc_mock.mock_calls[0][1][2], 'log')
        self.assertEqual(bc_mock.mock_calls[0][1][3], 1)

    @skipIf(sys.platform in ('win32', 'darwin'),
            'Not supported on Windown and OS X')
    def test_background_chaos_not_used(self):
        with self.ds_cxt():
            with patch('deploy_stack.background_chaos',
                       autospec=True) as bc_mock:
                _deploy_job('foo', None, None, '', None, None, None, None,
                            None, None, None, None, None, None, 0, False,
                            False, None)
        self.assertEqual(bc_mock.call_count, 0)

    def test_region(self):
        with self.ds_cxt() as client:
            bc_mock = deploy_stack.boot_context
            _deploy_job('foo', None, None, '', None, None, None, None,
                        None, None, None, None, None, None, 0, False,
                        False, 'region-foo')
        bc_mock.assert_called_once_with(
            'foo', client, None, None, None, None, None, None, None, None,
            permanent=False, region='region-foo')

    def test_deploy_job_changes_series_with_win(self):
        args = Namespace(
            series='windows', temp_env_name=None, env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            pre_destroy=None, region=None, verbose=None)
        with patch('deploy_stack.deploy_job_parse_args', return_value=args,
                   autospec=True):
            with patch('deploy_stack._deploy_job', autospec=True) as ds_mock:
                deploy_job()
        call_args = ds_mock.call_args[0]
        self.assertEqual(call_args[3], 'local:windows/')
        self.assertEqual(call_args[6], 'trusty')

    def test_deploy_job_changes_series_with_centos(self):
        args = Namespace(
            series='centos', temp_env_name=None, env=None, upgrade=None,
            charm_prefix=None, bootstrap_host=None, machine=None, logs=None,
            debug=None, juju_bin=None, agent_url=None, agent_stream=None,
            keep_env=None, upload_tools=None, with_chaos=None, jes=None,
            pre_destroy=None, region=None, verbose=None)
        with patch('deploy_stack.deploy_job_parse_args', return_value=args,
                   autospec=True):
            with patch('deploy_stack._deploy_job', autospec=True) as ds_mock:
                deploy_job()
        call_args = ds_mock.call_args[0]
        self.assertEqual(call_args[3], 'local:centos/')
        self.assertEqual(call_args[6], 'trusty')


class TestTestUpgrade(FakeHomeTestCase):

    RUN_UNAME = (
        'juju', '--show-log', 'run', '-e', 'foo', '--format', 'json',
        '--service', 'dummy-source,dummy-sink', 'uname')
    STATUS = ('juju', '--show-log', 'status', '-e', 'foo')
    GET_ENV = ('juju', '--show-log', 'get-env', '-e', 'foo',
               'tools-metadata-url')

    @classmethod
    def upgrade_output(cls, args, **kwargs):
        status = yaml.safe_dump({
            'machines': {'0': {
                'agent-state': 'started',
                'agent-version': '1.38'}},
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
                                   side_effect=lambda cls: '1.38'):
                            yield (co_mock, cc_mock)

    def test_assess_upgrade(self):
        env = SimpleEnvironment('foo', {'type': 'foo'})
        old_client = EnvJujuClient(env, None, '/foo/juju')
        with self.upgrade_mocks() as (co_mock, cc_mock):
            assess_upgrade(old_client, '/bar/juju')
        new_client = EnvJujuClient(env, None, '/bar/juju')
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'upgrade-juju', '-e', 'foo', '--version',
            '1.38'), 0)
        assert_juju_call(self, cc_mock, new_client, (
            'juju', '--show-log', 'set', '-e', 'foo', 'dummy-source',
            'token=FAKETOKEN'), 1)
        self.assertEqual(cc_mock.call_count, 2)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 0)
        assert_juju_call(self, co_mock, new_client, self.GET_ENV, 1)
        assert_juju_call(self, co_mock, new_client, self.STATUS, 2)
        assert_juju_call(self, co_mock, new_client, self.RUN_UNAME, 3)
        self.assertEqual(co_mock.call_count, 4)

    def test_mass_timeout(self):
        config = {'type': 'foo'}
        old_client = EnvJujuClient(SimpleEnvironment('foo', config),
                                   None, '/foo/juju')
        with self.upgrade_mocks():
            with patch.object(EnvJujuClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
            wfv_mock.assert_called_once_with('1.38', 600)
            config['type'] = 'maas'
            with patch.object(EnvJujuClient, 'wait_for_version') as wfv_mock:
                assess_upgrade(old_client, '/bar/juju')
        wfv_mock.assert_called_once_with('1.38', 1200)


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
    def bc_context(self, client, log_dir=None, jes=False, keep_env=False):
        dl_mock = self.addContext(patch('deploy_stack.dump_env_logs'))
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo', autospec=True))
        c_mock = self.addContext(patch('subprocess.call', autospec=True))
        if jes:
            output = 'system'
        else:
            output = ''
        with patch('subprocess.Popen', autospec=True,
                   return_value=FakePopen(output, '', 0)) as po_mock:
            yield
        assert_juju_call(self, po_mock, client, (
            'juju', '--show-log', 'help', 'commands'))
        if jes:
            runtime_config = os.path.join(client.juju_home, 'environments',
                                          'cache.yaml')
        else:
            runtime_config = os.path.join(client.juju_home, 'environments',
                                          'bar.jenv')
        dl_mock.assert_called_once_with(
            client, 'foo', log_dir, runtime_config=runtime_config)
        if keep_env:
            self.assertEqual(c_mock.call_count, 0)
        else:
            if jes:
                assert_juju_call(
                    self, c_mock, client, get_timeout_prefix(600) + (
                        'juju', '--show-log', 'system', 'kill', 'bar', '-y'))
            else:
                assert_juju_call(
                    self, c_mock, client, get_timeout_prefix(600) + (
                        'juju', '--show-log', 'destroy-environment', 'bar',
                        '--force', '-y'))

    def test_bootstrap_context(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir'):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--constraints',
            'mem=2G'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'status', '-e', 'bar'), 1)

    def test_keep_env(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, keep_env=True):
            with boot_context('bar', client, None, [], None, None, None, None,
                              keep_env=True, upload_tools=False):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--constraints',
            'mem=2G'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'status', '-e', 'bar'), 1)

    def test_upload_tools(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client):
            with boot_context('bar', client, None, [], None, None, None, None,
                              keep_env=False, upload_tools=True):
                pass
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'bar', '--upload-tools',
            '--constraints', 'mem=2G'), 0)

    def test_calls_update_env(self):
        cc_mock = self.addContext(patch('subprocess.check_call'))
        client = EnvJujuClient(SimpleEnvironment(
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

        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        self.addContext(patch('deploy_stack.get_machine_dns_name',
                              return_value='foo'))
        self.addContext(patch('subprocess.check_call'))
        call_mock = self.addContext(patch('subprocess.call'))
        po_mock = self.addContext(patch('subprocess.Popen', autospec=True,
                                        return_value=FakePopen('', '', 0)))
        self.addContext(patch('deploy_stack.wait_for_port'))
        self.addContext(patch.object(client, 'bootstrap',
                                     side_effect=FakeException))
        crl_mock = self.addContext(patch('deploy_stack.copy_remote_logs'))
        al_mock = self.addContext(patch('deploy_stack.archive_logs'))
        with self.assertRaises(FakeException):
            with boot_context('bar', client, 'baz', [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=True):
                pass
        self.assertEqual(crl_mock.call_count, 1)
        call_args = crl_mock.call_args[0]
        self.assertIsInstance(call_args[0], _Remote)
        self.assertEqual(call_args[0].get_address(), 'baz')
        self.assertEqual(call_args[1], 'log_dir')
        al_mock.assert_called_once_with('log_dir')
        timeout_path = get_timeout_path()
        assert_juju_call(self, call_mock, client, (
            sys.executable, timeout_path, '600.00', '--',
            'juju', '--show-log', 'destroy-environment', 'bar', '--force',
            '-y'
            ))
        assert_juju_call(self, po_mock, client, (
            'juju', '--show-log', 'help', 'commands'))

    def test_jes(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes=True):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False):
                pass

    def test_region(self):
        self.addContext(patch('subprocess.check_call', autospec=True))
        client = EnvJujuClient(SimpleEnvironment(
            'foo', {'type': 'paas'}), '1.23', 'path')
        with self.bc_context(client, 'log_dir', jes=True):
            with boot_context('bar', client, None, [], None, None, None,
                              'log_dir', keep_env=False, upload_tools=False,
                              region='steve'):
                pass
        self.assertEqual('steve', client.env.config['region'])


class TestDeployJobParseArgs(FakeHomeTestCase):

    def test_deploy_job_parse_args(self):
        args = deploy_job_parse_args(['foo', 'bar', 'baz', 'qux'])
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
            juju_bin='bar',
            series=None,
            upgrade=False,
            verbose=logging.INFO,
            upload_tools=False,
            with_chaos=0,
            jes=False,
            pre_destroy=False,
            region=None,
        ))

    def test_upload_tools(self):
        args = deploy_job_parse_args(
            ['foo', 'bar', 'baz', 'qux', '--upload-tools'])
        self.assertEqual(args.upload_tools, True)

    def test_agent_stream(self):
        args = deploy_job_parse_args(
            ['foo', 'bar', 'baz', 'qux', '--agent-stream', 'wacky'])
        self.assertEqual('wacky', args.agent_stream)

    def test_jes(self):
        args = deploy_job_parse_args(
            ['foo', 'bar', 'baz', 'qux', '--jes'])
        self.assertIs(args.jes, True)
