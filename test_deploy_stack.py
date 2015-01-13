import logging
from mock import patch
import os
from StringIO import StringIO
import subprocess
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


logger = logging.getLogger()
logger.level = logging.DEBUG


def make_logs(log_dir):
    def write_dumped_files(*args):
        with open(os.path.join(log_dir, 'cloud.log'), 'w') as l:
            l.write('fake log')
        with open(os.path.join(log_dir, 'extra'), 'w') as l:
            l.write('not compressed')
    return write_dumped_files


def get_machine_addresses():
    return {
        '0': '10.10.0.1',
        '1': '10.10.0.11',
        '2': '10.10.0.22',
    }


class DumpEnvLogsTestCase(TestCase):

    def setUp(self):
        self.log = logging.getLogger()
        self.old_handlers = self.log.handlers
        for handler in self.log.handlers:
            self.log.removeHandler(handler)
        self.stream = StringIO()
        self.handler = logging.StreamHandler(self.stream)
        self.log.addHandler(self.handler)
        self.handler.setLevel(logging.DEBUG)

        def reset_logger():
            self.log.removeHandler(self.handler)
            self.handler.close()
            for handler in self.old_handlers:
                self.log.addHandler(handler)

        self.addCleanup(reset_logger)

    def test_dump_env_logs_non_local_env(self):
        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_machines_for_logs', autospec=True,
                       return_value=get_machine_addresses()) as gm_mock:
                with patch('deploy_stack.dump_logs', autospec=True) as dl_mock:
                    client = EnvJujuClient(
                        SimpleEnvironment(
                            'foo', {'type': 'nonlocal'}), '1.234-76', None)
                    dump_env_logs(client, '10.10.0.1', artifacts_dir)
            self.assertEqual(
                ['0', '1', '2'], sorted(os.listdir(artifacts_dir)))
        self.assertEqual(
            (client, '10.10.0.1'), gm_mock.call_args[0])
        call_list = sorted((cal[0], cal[1]) for cal in dl_mock.call_args_list)
        self.assertEqual(
            [((client, '10.10.0.1', '%s/0' % artifacts_dir),
              {'local_state_server': False}),
             ((client, '10.10.0.11', '%s/1' % artifacts_dir),
              {'local_state_server': False}),
             ((client, '10.10.0.22', '%s/2' % artifacts_dir),
              {'local_state_server': False})],
            call_list)
        self.assertEqual(
            ['Retrieving logs for machine-0', 'Retrieving logs for machine-1',
             'Retrieving logs for machine-2'],
            sorted(self.stream.getvalue().splitlines()))

    def test_dump_env_logs_local_env(self):
        with temp_dir() as artifacts_dir:
            with patch('deploy_stack.get_machines_for_logs', autospec=True,
                       return_value=get_machine_addresses()):
                with patch('deploy_stack.dump_logs', autospec=True) as dl_mock:
                    client = EnvJujuClient(
                        SimpleEnvironment(
                            'foo', {'type': 'local'}), '1.234-76', None)
                    dump_env_logs(client, '10.10.0.1', artifacts_dir)
        call_list = sorted((cal[0], cal[1]) for cal in dl_mock.call_args_list)
        self.assertEqual(
            [((client, '10.10.0.1', '%s/0' % artifacts_dir),
              {'local_state_server': True}),
             ((client, '10.10.0.11', '%s/1' % artifacts_dir),
              {'local_state_server': False}),
             ((client, '10.10.0.22', '%s/2' % artifacts_dir),
              {'local_state_server': False})],
            call_list)

    def test_dump_logs_with_local_state_server_false(self):
        # copy_remote_logs is called for non-local envs.
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'nonlocal'}), '1.234-76', None)
        with temp_dir() as log_dir:
            with patch('deploy_stack.copy_local_logs',
                       autospec=True) as cll_mock:
                with patch('deploy_stack.copy_remote_logs', autospec=True,
                           side_effect=make_logs(log_dir)) as crl_mock:
                    dump_logs(client, '10.10.0.1', log_dir,
                              local_state_server=False)
            self.assertEqual(['cloud.log.gz', 'extra'], os.listdir(log_dir))
        self.assertEqual(0, cll_mock.call_count)
        self.assertEqual(('10.10.0.1', log_dir), crl_mock.call_args[0])

    def test_dump_logs_with_local_state_server_true(self):
        # copy_local_logs is called for machine 0 in a local env.
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with temp_dir() as log_dir:
            with patch('deploy_stack.copy_local_logs', autospec=True,
                       side_effect=make_logs(log_dir)) as cll_mock:
                with patch('deploy_stack.copy_remote_logs',
                           autospec=True) as crl_mock:
                    dump_logs(client, '10.10.0.1', log_dir,
                              local_state_server=True)
            self.assertEqual(['cloud.log.gz', 'extra'], os.listdir(log_dir))
        self.assertEqual((log_dir, client), cll_mock.call_args[0])
        self.assertEqual(0, crl_mock.call_count)

    def test_copy_remote_logs(self):
        # To get the logs, their permissions must be updated first,
        # then downloaded in the order that they will be created
        # to ensure errors do not prevent some logs from being retrieved.
        with patch('deploy_stack.wait_for_port', autospec=True):
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

    def test_copy_remote_logs_with_errors(self):
        # Ssh and scp errors will happen when /var/log/juju doesn't exist yet,
        # but we log the case anc continue to retrieve as much as we can.
        def remote_op(*args, **kwargs):
            if 'ssh' in args:
                raise subprocess.CalledProcessError('ssh error', 'output')
            else:
                raise subprocess.CalledProcessError('scp error', 'output')

        with patch('subprocess.check_call', side_effect=remote_op) as cc_mock:
            with patch('deploy_stack.wait_for_port', autospec=True):
                copy_remote_logs('10.10.0.1', '/foo')
        self.assertEqual(2, cc_mock.call_count)
        self.assertEqual(
            ['Could not change the permission of the juju logs:',
             'None',
             'Could not retrieve some or all logs:',
             'None'],
            self.stream.getvalue().splitlines())
