from mock import (
    ANY,
    MagicMock,
    patch,
)

from deploy_stack import BootstrapManager
from jujupy import (
    EnvJujuClient,
    JujuData,
    )
from quickstart_deploy import QuickstartTest
from tests import (
    FakeHomeTestCase,
    use_context,
)
from tests.test_deploy_stack import FakeBootstrapManager
from tests.test_jujupy import fake_juju_client
from utility import temp_dir


def make_bootstrap_manager(client, log_dir='log_dir'):
    return BootstrapManager(
        'env', client, client, None, [], 'series', 'agent_url',
        'agent_stream', 'region', log_dir, 'keep_env', 'permanent',
        'jes_enabled')


class TestQuickstartTest(FakeHomeTestCase):

    def test_run_finally(self):
        do_finally = MagicMock()

        def fake_iter_steps():
            try:
                yield {'bootstrap_host': 'foo'}
            finally:
                do_finally()

        client = EnvJujuClient(
            JujuData('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = make_bootstrap_manager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        with patch.object(quickstart, 'iter_steps',
                          side_effect=fake_iter_steps):
            quickstart.run()
        do_finally.assert_called_once_with()

    def test_run_exception(self):
        tear_down = MagicMock()

        def fake_iter_steps():
            try:
                yield {'bootstrap_host': 'foo'}
            except:
                tear_down()

        client = EnvJujuClient(
            JujuData('foo', {'type': 'local'}), '1.234-76', None)
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
            JujuData('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = make_bootstrap_manager(client, log_dir=log_dir)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        steps = quickstart.iter_steps()
        with patch.object(client, 'quickstart') as qs_mock:
            # Test first yield
            with patch('jujupy.check_free_disk_space', autospec=True):
                with patch('deploy_stack.tear_down', autospec=True) as td_mock:
                    step = steps.next()
        td_mock.assert_called_once_with(client, 'jes_enabled', try_jes=True)
        qs_mock.assert_called_once_with('/tmp/bundle.yaml')
        expected = {'juju-quickstart': 'Returned from quickstart'}
        self.assertEqual(expected, step)
        with patch('deploy_stack.get_machine_dns_name',
                   return_value='mocked_name') as dns_mock:
            # Test second yield
            with patch.object(client, 'get_admin_client') as gac_mock:
                step = steps.next()
        dns_mock.assert_called_once_with(gac_mock.return_value, '0')
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
                with patch('quickstart_deploy.BootstrapManager.dump_all_logs'):
                    with patch('jujupy.EnvJujuClient.iter_model_clients',
                               return_value=[]):
                        steps.close()

    def test_iter_steps_context(self):
        client = fake_juju_client()
        bs_manager = FakeBootstrapManager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        step_iter = quickstart.iter_steps()
        self.assertIs(False, bs_manager.entered_top)
        self.assertIs(False, bs_manager.exited_top)
        self.assertIs(False, bs_manager.entered_bootstrap)
        self.assertIs(False, bs_manager.exited_bootstrap)
        step_iter.next()
        models = client._backend.controller_state.models
        backing_state = models[client.model_name]
        self.assertEqual('/tmp/bundle.yaml', backing_state.current_bundle)
        self.assertIs(True, bs_manager.entered_top)
        self.assertIs(True, bs_manager.entered_bootstrap)
        self.assertIs(False, bs_manager.exited_bootstrap)
        self.assertIs(False, bs_manager.entered_runtime)
        self.assertIs(False, bs_manager.exited_runtime)
        step_iter.next()
        self.assertIs(True, bs_manager.exited_bootstrap)
        self.assertIs(True, bs_manager.entered_runtime)
        self.assertIs(False, bs_manager.exited_runtime)
        with patch.object(client, 'wait_for_deploy_started') as wfds_mock:
            step_iter.next()
        wfds_mock.assert_called_once_with(2)
        self.assertIs(False, bs_manager.exited_runtime)
        with patch.object(client, 'wait_for_started') as wfs_mock:
            step_iter.next()
        wfs_mock.assert_called_once_with(3600)
        self.assertIs(False, bs_manager.exited_runtime)
        with self.assertRaises(StopIteration):
            step_iter.next()
        self.assertIs(True, bs_manager.exited_runtime)
        self.assertIs(True, bs_manager.exited_top)

    def test_iter_steps_quickstart_fail(self):
        client = EnvJujuClient(
            JujuData('foo', {'type': 'local'}), '1.234-76', None)
        bs_manager = FakeBootstrapManager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        step_iter = quickstart.iter_steps()
        with patch.object(client, 'quickstart', side_effect=Exception):
            with self.assertRaises(Exception):
                step_iter.next()
        self.assertIs(False, bs_manager.entered_runtime)
        self.assertIs(True, bs_manager.exited_bootstrap)
        self.assertIs(True, bs_manager.exited_top)

    def test_iter_steps_wait_fail(self):
        client = fake_juju_client()
        bs_manager = FakeBootstrapManager(client)
        quickstart = QuickstartTest(bs_manager, '/tmp/bundle.yaml', 2)
        step_iter = quickstart.iter_steps()
        step_iter.next()
        step_iter.next()
        with patch.object(client, 'wait_for_deploy_started',
                          side_effect=Exception):
            with self.assertRaises(Exception):
                step_iter.next()
        self.assertIs(True, bs_manager.exited_runtime)
        self.assertIs(True, bs_manager.exited_bootstrap)
        self.assertIs(True, bs_manager.exited_top)
