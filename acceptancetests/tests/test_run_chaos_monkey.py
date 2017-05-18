from datetime import (
    datetime,
    timedelta,
)
from unittest import TestCase

from mock import (
    patch
    )

from chaos import MonkeyRunner
from jujupy import (
    ModelClient,
    JujuData,
    )
from run_chaos_monkey import (
    get_args,
    run_while_healthy_or_timeout,
    )


class TestRunChaosMonkey(TestCase):

    def test_get_args(self):
        args = get_args(['foo', 'bar', 'baz'])
        self.assertItemsEqual(['env', 'service', 'health_checker',
                               'enablement_timeout', 'pause_timeout',
                               'total_timeout'],
                              [a for a in dir(args) if not a.startswith('_')])
        self.assertEqual(args.env, 'foo')
        self.assertEqual(args.service, 'bar')
        self.assertEqual(args.health_checker, 'baz')

    def test_run_while_healthy_or_timeout(self):
        client = ModelClient(JujuData('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'bar', 'script', client, total_timeout=60)
        runner.expire_time = (datetime.now() - timedelta(seconds=1))
        with patch.object(runner, 'is_healthy', autospec=True,
                          return_value=True):
            with patch.object(runner, 'unleash_once', autospec=True) as u_mock:
                with patch.object(runner, 'wait_for_chaos',
                                  autospec=True) as wait_mock:
                    run_while_healthy_or_timeout(runner)
        u_mock.assert_called_once_with()
        wait_mock.assert_called_once_with()

    def test_run_while_healthy_or_timeout_exits_non_zero(self):
        client = ModelClient(JujuData('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'bar', 'script', client, total_timeout=60)
        with patch.object(runner, 'is_healthy', autospec=True,
                          return_value=False):
            with patch('run_chaos_monkey.sys.exit') as se_mock:
                with patch('logging.error') as le_mock:
                    run_while_healthy_or_timeout(runner)
        se_mock.assert_called_once_with(1)
        self.assertRegexpMatches(
            le_mock.call_args[0][0], 'The health check reported an error:')
