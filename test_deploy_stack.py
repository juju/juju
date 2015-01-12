from mock import patch
import os
from unittest import TestCase

from deploy_stack import (
    copy_remote_logs,
    dump_env_logs,
    dump_logs,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import temp_dir


def make_logs(log_dir):
    def write_dumped_files(*args):
        with open(os.path.join(log_dir, 'cloud.log'), 'w') as l:
            l.write('fake log')
        with open(os.path.join(log_dir, 'extra'), 'w') as l:
            l.write('not compressed')
    return write_dumped_files


class DumpEnvLogsTestCase(TestCase):

    def test_dump_env_logs(self):
        machine_addresses = {
            '0': '10.10.0.1',
            '1': '10.10.0.11',
            '2': '10.10.0.22',
        }

        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_machines_for_logs',
                       return_value=machine_addresses) as gm_mock:
                with patch('deploy_stack.dump_logs') as dl_mock:
                    client = object()
                    dump_env_logs(client, '10.10.0.1', artifacts_dir)
            self.assertEqual(
                ['0', '1', '2'], sorted(os.listdir(artifacts_dir)))
        self.assertEqual(
            (client, '10.10.0.1'), gm_mock.call_args[0])
        call_list = sorted((cal[0], cal[1]) for cal in dl_mock.call_args_list)
        self.assertEqual(
            [((client, '10.10.0.1', '%s/0' % artifacts_dir),
              {'machine_id': '0'}),
             ((client, '10.10.0.11', '%s/1' % artifacts_dir),
              {'machine_id': '1'}),
             ((client, '10.10.0.22', '%s/2' % artifacts_dir),
              {'machine_id': '2'})],
            call_list)

    def test_dump_logs_with_nonlocal_env(self):
        # copy_remote_logs is called for non-local envs.
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'nonlocal'}), '1.234-76', None)
        with temp_dir() as log_dir:
            with patch('deploy_stack.copy_local_logs') as cll_mock:
                with patch('deploy_stack.copy_remote_logs',
                           side_effect=make_logs(log_dir)) as crl_mock:
                    dump_logs(client, '10.10.0.1', log_dir, machine_id='0')
            self.assertEqual(['cloud.log.gz', 'extra'], os.listdir(log_dir))
        self.assertEqual(0, cll_mock.call_count)
        self.assertEqual(('10.10.0.1', log_dir), crl_mock.call_args[0])

    def test_dump_logs_with_local_0_env(self):
        # copy_local_logs is called for machine 0 in a local env.
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with temp_dir() as log_dir:
            with patch('deploy_stack.copy_local_logs',
                       side_effect=make_logs(log_dir)) as cll_mock:
                with patch('deploy_stack.copy_remote_logs') as crl_mock:
                    dump_logs(client, '10.10.0.1', log_dir, machine_id='0')
            self.assertEqual(['cloud.log.gz', 'extra'], os.listdir(log_dir))
        self.assertEqual((log_dir, client), cll_mock.call_args[0])
        self.assertEqual(0, crl_mock.call_count)

    def test_dump_logs_with_local_not_0_env(self):
        # copy_remote_logs is called for machine 1+ in a local env.
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with temp_dir() as log_dir:
            with patch('deploy_stack.copy_local_logs') as cll_mock:
                with patch('deploy_stack.copy_remote_logs',
                           side_effect=make_logs(log_dir)) as crl_mock:
                    dump_logs(client, '10.10.0.1', log_dir, machine_id='1')
            self.assertEqual(['cloud.log.gz', 'extra'], os.listdir(log_dir))
        self.assertEqual(0, cll_mock.call_count)
        self.assertEqual(('10.10.0.1', log_dir), crl_mock.call_args[0])

    def test_copy_remote_logs(self):
        with patch('deploy_stack.wait_for_port'):
            with patch('subprocess.check_call') as cc_mock:
                copy_remote_logs('10.10.0.1', '/foo')
        self.assertEqual(
            (['timeout', '5m', 'ssh',
              '-o', 'UserKnownHostsFile /dev/null',
              '-o', 'StrictHostKeyChecking no',
              'ubuntu@10.10.0.1',
              'sudo chmod go+r /var/log/juju/*'], ),
            cc_mock.call_args_list[0][0])
        self.assertEqual(
            (['timeout', '5m', 'scp', '-C',
              '-o', 'UserKnownHostsFile /dev/null',
              '-o', 'StrictHostKeyChecking no',
              'ubuntu@10.10.0.1:/var/log/{cloud-init*.log,juju/*.log}',
              '/foo'],),
            cc_mock.call_args_list[1][0])
