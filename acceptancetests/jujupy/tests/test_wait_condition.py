from datetime import (
    datetime,
    timedelta,
    )
from mock import (
    Mock,
    patch,
    )

from jujupy import (
    fake_juju_client,
    )
from jujupy.status import (
    Status,
    )
from jujupy.wait_condition import (
    BaseCondition,
    CommandComplete,
    CommandTime,
    ConditionList,
    MachineDown,
    NoopCondition,
    WaitApplicationNotPresent,
    WaitMachineNotPresent,
    )

from tests import (
    TestCase,
    )


class TestBaseCondition(TestCase):

    def test_timeout(self):
        condition = BaseCondition()
        self.assertEqual(300, condition.timeout)
        condition = BaseCondition(600)
        self.assertEqual(600, condition.timeout)


class TestConditionList(TestCase):

    def test_uses_max_timeout(self):
        conditions = ConditionList([Mock(timeout=300), Mock(timeout=400)])
        self.assertEqual(400, conditions.timeout)

    def test_empty_timeout(self):
        conditions = ConditionList([])
        self.assertEqual(300, conditions.timeout)

    def test_iter_blocking_state(self):
        mock_ab = Mock(timeout=0)
        mock_ab.iter_blocking_state.return_value = [('a', 'b')]
        mock_cd = Mock(timeout=0)
        mock_cd.iter_blocking_state.return_value = [('c', 'd')]
        conditions = ConditionList([mock_ab, mock_cd])
        self.assertEqual([('a', 'b'), ('c', 'd')],
                         list(conditions.iter_blocking_state(None)))


class TestNoopCondition(TestCase):

    def test_iter_blocking_state_is_noop(self):
        condition = NoopCondition()
        called = False
        for _ in condition.iter_blocking_state({}):
            called = True
        self.assertFalse(called)

    def test_do_raise_raises_Exception(self):
        condition = NoopCondition()
        with self.assertRaises(Exception):
            condition.do_raise('model_name', {})


class TestWaitMachineNotPresent(TestCase):

    def test_iter_blocking_state(self):
        not_present = WaitMachineNotPresent('0')
        client = fake_juju_client()
        client.bootstrap()
        self.assertItemsEqual(
            [], not_present.iter_blocking_state(client.get_status()))
        client.juju('add-machine', ())
        self.assertItemsEqual(
            [('0', 'still-present')],
            not_present.iter_blocking_state(client.get_status()))
        client.juju('remove-machine', ('0'))
        self.assertItemsEqual(
            [], not_present.iter_blocking_state(client.get_status()))

    def test_do_raise(self):
        not_present = WaitMachineNotPresent('0')
        with self.assertRaisesRegexp(
                Exception, 'Timed out waiting for machine removal 0'):
            not_present.do_raise('', None)


class TestWaitApplicationNotPresent(TestCase):

    def test_iter_blocking_state(self):
        not_present = WaitApplicationNotPresent('foo')
        client = fake_juju_client()
        client.bootstrap()
        self.assertItemsEqual(
            [], not_present.iter_blocking_state(client.get_status()))
        client.deploy('foo')
        self.assertItemsEqual(
            [('foo', 'still-present')],
            not_present.iter_blocking_state(client.get_status()))
        client.remove_service('foo')
        self.assertItemsEqual(
            [], not_present.iter_blocking_state(client.get_status()))

    def test_do_raise(self):
        not_present = WaitApplicationNotPresent('foo')
        with self.assertRaisesRegexp(
                Exception, 'Timed out waiting for application removal foo'):
            not_present.do_raise('', None)


class TestMachineDown(TestCase):

    def test_iter_blocking_state(self):
        down = MachineDown('0')
        client = fake_juju_client()
        client.bootstrap()
        client.juju('add-machine', ())
        self.assertItemsEqual(
            [('0', 'idle')],
            down.iter_blocking_state(client.get_status()))
        status = Status({'machines': {'0': {'juju-status': {
            'current': 'down',
            }}}}, '')
        self.assertItemsEqual(
            [], down.iter_blocking_state(status))

    def test_do_raise(self):
        down = MachineDown('0')
        with self.assertRaisesRegexp(
                Exception,
                'Timed out waiting for juju to determine machine 0 down.'):
            down.do_raise('', None)


class TestCommandTime(TestCase):

    def test_default_values(self):
        full_args = ['juju', '--showlog', 'bootstrap']
        utcnow = datetime(2017, 3, 22, 23, 36, 52, 530631)
        with patch('jujupy.wait_condition.datetime', autospec=True) as m_dt:
            m_dt.utcnow.return_value = utcnow
            ct = CommandTime('bootstrap', full_args)
            self.assertEqual(ct.cmd, 'bootstrap')
            self.assertEqual(ct.full_args, full_args)
            self.assertEqual(ct.envvars, None)
            self.assertEqual(ct.start, utcnow)
            self.assertEqual(ct.end, None)

    def test_set_start_time(self):
        ct = CommandTime('cmd', [], start='abc')
        self.assertEqual(ct.start, 'abc')

    def test_set_envvar(self):
        details = {'abc': 123}
        ct = CommandTime('cmd', [], envvars=details)
        self.assertEqual(ct.envvars, details)

    def test_actual_completion_sets_default(self):
        utcnow = datetime(2017, 3, 22, 23, 36, 52, 530631)
        ct = CommandTime('cmd', [])
        with patch('jujupy.wait_condition.datetime', autospec=True) as m_dt:
            m_dt.utcnow.return_value = utcnow
            ct.actual_completion()
        self.assertEqual(ct.end, utcnow)

    def test_actual_completion_idempotent(self):
        ct = CommandTime('cmd', [])
        ct.actual_completion(end='a')
        ct.actual_completion(end='b')
        self.assertEqual(ct.end, 'a')

    def test_actual_completion_set_value(self):
        utcnow = datetime(2017, 3, 22, 23, 36, 52, 530631)
        ct = CommandTime('cmd', [])
        ct.actual_completion(end=utcnow)
        self.assertEqual(ct.end, utcnow)

    def test_total_seconds_returns_None_when_not_complete(self):
        ct = CommandTime('cmd', [])
        self.assertEqual(ct.total_seconds, None)

    def test_total_seconds_returns_seconds_taken_to_complete(self):
        utcstart = datetime(2017, 3, 22, 23, 36, 52, 530631)
        utcend = utcstart + timedelta(seconds=1)
        with patch('jujupy.wait_condition.datetime', autospec=True) as m_dt:
            m_dt.utcnow.side_effect = [utcstart, utcend]
            ct = CommandTime('cmd', [])
            ct.actual_completion()
        self.assertEqual(ct.total_seconds, 1)


class TestCommandComplete(TestCase):

    def test_default_values(self):
        ct = CommandTime('cmd', [])
        base_condition = BaseCondition()
        cc = CommandComplete(base_condition, ct)

        self.assertEqual(cc.timeout, 300)
        self.assertEqual(cc.already_satisfied, False)
        self.assertEqual(cc._real_condition, base_condition)
        self.assertEqual(cc.command_time, ct)
        # actual_completion shouldn't be set as the condition is not already
        # satisfied.
        self.assertEqual(cc.command_time.end, None)

    def test_sets_total_seconds_when_already_satisfied(self):
        base_condition = BaseCondition(already_satisfied=True)
        ct = CommandTime('cmd', [])
        cc = CommandComplete(base_condition, ct)

        self.assertIsNotNone(cc.command_time.total_seconds)

    def test_calls_wrapper_condition_iter(self):
        class TestCondition(BaseCondition):
            def iter_blocking_state(self, status):
                yield 'item', status

        ct = CommandTime('cmd', [])
        cc = CommandComplete(TestCondition(), ct)

        k, v = next(cc.iter_blocking_state('status_obj'))
        self.assertEqual(k, 'item')
        self.assertEqual(v, 'status_obj')

    def test_sets_actual_completion_when_complete(self):
        """When the condition hits success must set actual_completion."""
        class TestCondition(BaseCondition):
            def __init__(self):
                super(TestCondition, self).__init__()
                self._already_called = False

            def iter_blocking_state(self, status):
                if not self._already_called:
                    self._already_called = True
                    yield 'item', status

        ct = CommandTime('cmd', [])
        cc = CommandComplete(TestCondition(), ct)

        next(cc.iter_blocking_state('status_obj'))
        self.assertIsNone(cc.command_time.end)
        next(cc.iter_blocking_state('status_obj'), None)
        self.assertIsNotNone(cc.command_time.end)

    def test_raises_exception_with_command_details(self):
        ct = CommandTime('cmd', ['cmd', 'arg1', 'arg2'])
        cc = CommandComplete(BaseCondition(), ct)

        with self.assertRaises(RuntimeError) as ex:
            cc.do_raise('status')
        self.assertEqual(
            str(ex.exception),
            'Timed out waiting for "cmd" command to complete: "cmd arg1 arg2"')
