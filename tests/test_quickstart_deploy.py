from mock import (
    ANY,
    MagicMock,
    patch,
)
from unittest import TestCase

from deploy_stack import BootstrapManager
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from quickstart_deploy import QuickstartTest
from tests import use_context
from utility import temp_dir


def make_bootstrap_manager(client, log_dir='log_dir'):
    return BootstrapManager(
        'env', client, None, [], 'series', 'agent_url',
        'agent_stream', 'region', log_dir, 'keep_env', 'permanent',
        'jes_enabled')


class TestQuickstartTest(TestCase):

    def test_run_finally(self):
        do_finally = MagicMock()

        def fake_iter_steps():
            try:
                yield {'bootstrap_host': 'foo'}
            finally:
                do_finally()

        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = make_bootstrap_manager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        with patch.object(quickstart, 'iter_steps',
                          side_effect=fake_iter_steps):
            quickstart.run()
        do_finally.assert_called_once_with()

    @patch('sys.stderr')
    def test_run_exception(self, se_mock):
        tear_down = MagicMock()

        def fake_iter_steps():
            try:
                yield {'bootstrap_host': 'foo'}
            except:
                tear_down()

        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = make_bootstrap_manager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        with patch.object(quickstart, 'iter_steps',
                          side_effect=fake_iter_steps):
            with self.assertRaises(BaseException):
                with patch('logging.info', side_effect=Exception):
                    quickstart.run()
        tear_down.assert_called_once_with()

    def test_iter_steps(self):
        log_dir = use_context(self, temp_dir())
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = make_bootstrap_manager(client, log_dir=log_dir)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        steps = quickstart.iter_steps()
        with patch.object(client, 'quickstart') as qs_mock:
            # Test first yield
            step = steps.next()
        qs_mock.assert_called_once_with('/tmp/bundle.yaml')
        expected = {'juju-quickstart': 'Returned from quickstart'}
        self.assertEqual(expected, step)
        with patch('deploy_stack.get_machine_dns_name',
                   return_value='mocked_name') as dns_mock:
            # Test second yield
            step = steps.next()
        dns_mock.assert_called_once_with(client, '0')
        self.assertEqual('mocked_name', step['bootstrap_host'])
        with patch.object(client, 'wait_for_deploy_started') as wds_mock:
            # Test third yield
            step = steps.next()
        wds_mock.assert_called_once_with(2)
        self.assertEqual('Deploy stated', step['deploy_started'])
        with patch.object(client, 'wait_for_started') as ws_mock:
            # Test forth yield
            step = steps.next()
        ws_mock.assert_called_once_with(ANY)
        self.assertEqual('All Agents started', step['agents_started'])
        with patch('deploy_stack.safe_print_status'):
            with patch('deploy_stack.tear_down'):
                with patch('deploy_stack.dump_env_logs'):
                    steps.close()
