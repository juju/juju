from datetime import (
    datetime,
    timedelta,
    )
from contextlib import contextmanager
import errno
import os
import socket

from mock import (
    patch,
    )

from tests import (
    TestCase,
    )
import jujupy.utility
from jujupy.utility import (
    is_ipv6_address,
    quote,
    scoped_environ,
    skip_on_missing_file,
    split_address_port,
    temp_dir,
    until_timeout,
    unqualified_model_name,
    qualified_model_name,
    )


class TestUntilTimeout(TestCase):

    def test_no_timeout(self):

        iterator = until_timeout(0)

        def now_iter():
            yield iterator.start
            yield iterator.start
            assert False

        with patch.object(iterator, 'now', lambda: next(now_iter())):
            for x in iterator:
                self.assertIs(None, x)
                break

    @contextmanager
    def patched_until(self, timeout, deltas):
        iterator = until_timeout(timeout)

        def now_iter():
            for d in deltas:
                yield iterator.start + d
            assert False
        now_iter_i = now_iter()
        with patch.object(iterator, 'now', lambda: next(now_iter_i)):
            yield iterator

    def test_timeout(self):
        with self.patched_until(
                5, [timedelta(), timedelta(0, 4), timedelta(0, 5)]) as until:
            results = list(until)
        self.assertEqual([5, 1], results)

    def test_long_timeout(self):
        deltas = [timedelta(), timedelta(4, 0), timedelta(5, 0)]
        with self.patched_until(86400 * 5, deltas) as until:
            self.assertEqual([86400 * 5, 86400], list(until))

    def test_start(self):
        now = datetime.now() + timedelta(days=1)
        now_iter = iter([now, now, now + timedelta(10)])
        with patch('utility.until_timeout.now',
                   side_effect=lambda: next(now_iter)):
            self.assertEqual(list(until_timeout(10, now - timedelta(10))), [])


class TestIsIPv6Address(TestCase):

    def test_hostname(self):
        self.assertIs(False, is_ipv6_address("name.testing"))

    def test_ipv4(self):
        self.assertIs(False, is_ipv6_address("127.0.0.2"))

    def test_ipv6(self):
        self.assertIs(True, is_ipv6_address("2001:db8::4"))

    def test_ipv6_missing_support(self):
        socket_error = jujupy.utility.socket.error
        with patch('jujupy.utility.socket', wraps=socket) as wrapped_socket:
            # Must not convert socket.error into a Mock, because Mocks don't
            # descend from BaseException
            wrapped_socket.error = socket_error
            del wrapped_socket.inet_pton
            result = is_ipv6_address("2001:db8::4")
        # Would use expectedFailure here, but instead just assert wrong result.
        self.assertIs(False, result)


class TestSplitAddressPort(TestCase):

    def test_hostname(self):
        self.assertEqual(
            ("name.testing", None), split_address_port("name.testing"))

    def test_ipv4(self):
        self.assertEqual(
            ("127.0.0.2", "17017"), split_address_port("127.0.0.2:17017"))

    def test_ipv6(self):
        self.assertEqual(
            ("2001:db8::7", "17017"), split_address_port("2001:db8::7:17017"))


class TestSkipOnMissingFile(TestCase):

    def test_skip_on_missing_file(self):
        """Test if skip_on_missing_file hides the proper exceptions."""
        with skip_on_missing_file():
            raise OSError(errno.ENOENT, 'should be hidden')
        with skip_on_missing_file():
            raise IOError(errno.ENOENT, 'should be hidden')

    def test_skip_on_missing_file_except(self):
        """Test if skip_on_missing_file ignores other types of exceptions."""
        with self.assertRaises(RuntimeError):
            with skip_on_missing_file():
                raise RuntimeError(errno.ENOENT, 'pass through')
        with self.assertRaises(IOError):
            with skip_on_missing_file():
                raise IOError(errno.EEXIST, 'pass through')


class TestQuote(TestCase):

    def test_quote(self):
        self.assertEqual(quote("arg"), "arg")
        self.assertEqual(quote("/a/file name"), "'/a/file name'")
        self.assertEqual(quote("bob's"), "'bob'\"'\"'s'")


class TestScopedEnviron(TestCase):

    def test_scoped_environ(self):
        old_environ = dict(os.environ)
        with scoped_environ():
            os.environ.clear()
            os.environ['foo'] = 'bar'
            self.assertNotEqual(old_environ, os.environ)
        self.assertEqual(old_environ, os.environ)

    def test_new_environ(self):
        new_environ = {'foo': 'bar'}
        with scoped_environ(new_environ):
            self.assertEqual(os.environ, new_environ)
        self.assertNotEqual(os.environ, new_environ)


class TestTempDir(TestCase):

    def test_temp_dir(self):
        with temp_dir() as d:
            self.assertTrue(os.path.isdir(d))
        self.assertFalse(os.path.exists(d))

    def test_temp_dir_contents(self):
        with temp_dir() as d:
            self.assertTrue(os.path.isdir(d))
            open(os.path.join(d, "a-file"), "w").close()
        self.assertFalse(os.path.exists(d))

    def test_temp_dir_parent(self):
        with temp_dir() as p:
            with temp_dir(parent=p) as d:
                self.assertTrue(os.path.isdir(d))
                self.assertEqual(p, os.path.dirname(d))
            self.assertFalse(os.path.exists(d))
        self.assertFalse(os.path.exists(p))

    def test_temp_dir_keep(self):
        with temp_dir() as p:
            with temp_dir(parent=p, keep=True) as d:
                self.assertTrue(os.path.isdir(d))
                open(os.path.join(d, "a-file"), "w").close()
            self.assertTrue(os.path.exists(d))
            self.assertTrue(os.path.exists(os.path.join(d, "a-file")))
        self.assertFalse(os.path.exists(p))


class TestUnqualifiedModelName(TestCase):

    def test_returns_just_model_name_when_passed_qualifed_full_username(self):
        self.assertEqual(
            unqualified_model_name('admin@local/default'),
            'default'
        )

    def test_returns_just_model_name_when_passed_just_username(self):
        self.assertEqual(
            unqualified_model_name('admin/default'),
            'default'
        )

    def test_returns_just_model_name_when_passed_no_username(self):
        self.assertEqual(
            unqualified_model_name('default'),
            'default'
        )


class TestQualifiedModelName(TestCase):

    def test_raises_valueerror_when_model_name_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('', 'admin@local')

    def test_raises_valueerror_when_owner_name_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('default', '')

    def test_raises_valueerror_when_owner_and_model_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('', '')

    def test_raises_valueerror_when_owner_name_doesnt_match_model_owner(self):
        with self.assertRaises(ValueError):
            qualified_model_name('test/default', 'admin')

        with self.assertRaises(ValueError):
            qualified_model_name('test@local/default', 'admin@local')

    def test_returns_qualified_model_name_with_plain_model_name(self):
        self.assertEqual(
            qualified_model_name('default', 'admin@local'),
            'admin@local/default'
        )

        self.assertEqual(
            qualified_model_name('default', 'admin'),
            'admin/default'
        )

    def test_returns_qualified_model_name_with_model_name_with_owner(self):
        self.assertEqual(
            qualified_model_name('admin@local/default', 'admin@local'),
            'admin@local/default'
        )

        self.assertEqual(
            qualified_model_name('admin/default', 'admin'),
            'admin/default'
        )
