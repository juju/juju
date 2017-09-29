import os

from datetime import (
    datetime,
    timedelta,
    )
from mock import (
    patch,
    )

from jujupy.backend import (
    JUJU_DEV_FEATURE_FLAGS,
    JujuBackend,
)
from jujupy.exceptions import (
    NoActiveModel,
    SoftDeadlineExceeded,
    )
from jujupy.utility import (
    get_timeout_prefix,
    scoped_environ,
    )

from tests import (
    FakePopen,
    TestCase,
    )


class TestJujuBackend(TestCase):

    test_environ = {'PATH': 'foo:bar'}

    def test_juju2_backend(self):
        backend = JujuBackend('/bin/path', '2.0', set(), False)
        self.assertEqual('/bin/path', backend.full_path)
        self.assertEqual('2.0', backend.version)

    def test_clone_retains_soft_deadline(self):
        soft_deadline = object()
        backend = JujuBackend(
            '/bin/path', '2.0', feature_flags=set(),
            debug=False, soft_deadline=soft_deadline)
        cloned = backend.clone(full_path=None, version=None, debug=None,
                               feature_flags=None)
        self.assertIsNot(cloned, backend)
        self.assertIs(soft_deadline, cloned.soft_deadline)

    def test_cloned_backends_share_juju_timings(self):
        backend = JujuBackend('/bin/path', '2.0', set(), False)
        cloned = backend.clone(
            full_path=None, version=None, debug=None, feature_flags=None)
        self.assertIs(cloned.juju_timings, backend.juju_timings)

    def test__check_timeouts(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=datetime(2015, 1, 2, 3, 4, 5))
        with patch('jujupy.JujuBackend._now',
                   return_value=backend.soft_deadline):
            with backend._check_timeouts():
                pass
        now = backend.soft_deadline + timedelta(seconds=1)
        with patch('jujupy.JujuBackend._now', return_value=now):
            with self.assertRaisesRegexp(
                    SoftDeadlineExceeded,
                    'Operation exceeded deadline.'):
                with backend._check_timeouts():
                    pass

    def test__check_timeouts_no_deadline(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=None)
        now = datetime(2015, 1, 2, 3, 4, 6)
        with patch('jujupy.JujuBackend._now', return_value=now):
            with backend._check_timeouts():
                pass

    def test_ignore_soft_deadline_check_timeouts(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=datetime(2015, 1, 2, 3, 4, 5))
        now = backend.soft_deadline + timedelta(seconds=1)
        with patch('jujupy.JujuBackend._now', return_value=now):
            with backend.ignore_soft_deadline():
                with backend._check_timeouts():
                    pass
            with self.assertRaisesRegexp(SoftDeadlineExceeded,
                                         'Operation exceeded deadline.'):
                with backend._check_timeouts():
                    pass

    def test_shell_environ_feature_flags(self):
        backend = JujuBackend(
            '/bin/path', '2.0', {'may', 'june'},
            debug=False, soft_deadline=None)
        env = backend.shell_environ({'april', 'june'}, 'fake-home')
        self.assertEqual('june', env[JUJU_DEV_FEATURE_FLAGS])

    def test_shell_environ_feature_flags_environmental(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=None)
        with scoped_environ():
            os.environ[JUJU_DEV_FEATURE_FLAGS] = 'run-test'
            env = backend.shell_environ(set(), 'fake-home')
        self.assertEqual('run-test', env[JUJU_DEV_FEATURE_FLAGS])

    def test_shell_environ_feature_flags_environmental_union(self):
        backend = JujuBackend(
            '/bin/path', '2.0', {'june'}, debug=False,
            soft_deadline=None)
        with scoped_environ():
            os.environ[JUJU_DEV_FEATURE_FLAGS] = 'run-test'
            env = backend.shell_environ({'june'}, 'fake-home')
        # The feature_flags are combined in alphabetic order.
        self.assertEqual('june,run-test', env[JUJU_DEV_FEATURE_FLAGS])

    def test_full_args(self):
        backend = JujuBackend('/bin/path/juju', '2.0', set(), False, None)
        full = backend.full_args('help', ('commands',), None, None)
        self.assertEqual(('juju', '--show-log', 'help', 'commands'), full)

    def test_full_args_debug(self):
        backend = JujuBackend('/bin/path/juju', '2.0', set(), True, None)
        full = backend.full_args('help', ('commands',), None, None)
        self.assertEqual(('juju', '--debug', 'help', 'commands'), full)

    def test_full_args_model(self):
        backend = JujuBackend('/bin/path/juju', '2.0', set(), False, None)
        full = backend.full_args('help', ('commands',), 'test', None)
        self.assertEqual(('juju', '--show-log', 'help', '-m', 'test',
                          'commands'), full)

    def test_full_args_timeout(self):
        backend = JujuBackend('/bin/path/juju', '2.0', set(), False, None)
        full = backend.full_args('help', ('commands',), None, 600)
        self.assertEqual(get_timeout_prefix(600, backend._timeout_path) +
                         ('juju', '--show-log', 'help', 'commands'), full)

    def test_juju_checks_timeouts(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=datetime(2015, 1, 2, 3, 4, 5))
        with patch('subprocess.check_call'):
            with patch('jujupy.JujuBackend._now',
                       return_value=backend.soft_deadline):
                backend.juju('cmd', ('args',), [], 'home')
            now = backend.soft_deadline + timedelta(seconds=1)
            with patch('jujupy.JujuBackend._now', return_value=now):
                with self.assertRaisesRegexp(SoftDeadlineExceeded,
                                             'Operation exceeded deadline.'):
                    backend.juju('cmd', ('args',), [], 'home')

    def test_juju_async_checks_timeouts(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=datetime(2015, 1, 2, 3, 4, 5))
        with patch('subprocess.Popen') as mock_popen:
            mock_popen.return_value.wait.return_value = 0
            with patch('jujupy.JujuBackend._now',
                       return_value=backend.soft_deadline):
                with backend.juju_async('cmd', ('args',), [], 'home'):
                    pass
            now = backend.soft_deadline + timedelta(seconds=1)
            with patch('jujupy.JujuBackend._now', return_value=now):
                with self.assertRaisesRegexp(SoftDeadlineExceeded,
                                             'Operation exceeded deadline.'):
                    with backend.juju_async('cmd', ('args',), [], 'home'):
                        pass

    def test_get_juju_output_checks_timeouts(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=datetime(2015, 1, 2, 3, 4, 5))
        with patch('subprocess.Popen') as mock_popen:
            mock_popen.return_value.returncode = 0
            mock_popen.return_value.communicate.return_value = ('', '')
            with patch('jujupy.JujuBackend._now',
                       return_value=backend.soft_deadline):
                backend.get_juju_output('cmd', ('args',), [], 'home')
            now = backend.soft_deadline + timedelta(seconds=1)
            with patch('jujupy.JujuBackend._now', return_value=now):
                with self.assertRaisesRegexp(SoftDeadlineExceeded,
                                             'Operation exceeded deadline.'):
                    backend.get_juju_output('cmd', ('args',), [], 'home')

    def test_get_active_model(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=None)
        with patch('subprocess.Popen') as mock_popen:
            mock_popen.return_value.communicate.return_value = (
                b'{"current-model": "model"}', b'')
            mock_popen.return_value.returncode = 0
            result = backend.get_active_model('/foo/bar')
        self.assertEqual(('model'), result)

    def test_get_active_model_none(self):
        backend = JujuBackend(
            '/bin/path', '2.0', set(), debug=False,
            soft_deadline=None)
        with patch('subprocess.Popen', autospec=True, return_value=FakePopen(
                   '{"models": {}}', '', 0)):
            with self.assertRaises(NoActiveModel):
                backend.get_active_model('/foo/bar')
