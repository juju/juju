from mock import patch
import os
from unittest import TestCase

from deploy_stack import (
    dump_env_logs,
    dump_logs,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import temp_dir


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
