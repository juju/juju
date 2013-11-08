__metaclass__ = type

from datetime import timedelta
from mock import patch
from unittest import TestCase

from jujupy import (
    ErroredUnit,
    until_timeout,
)


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('foo', 'bar', 'baz')
        self.assertEqual('<foo> bar is in state baz', str(e))


class TestUntilTimeout(TestCase):

    def test_no_timeout(self):

        iterator = until_timeout(0)

        def now_iter():
            yield iterator.start
            yield iterator.start
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            for x in iterator:
                self.assertIs(None, x)
                break

    def test_timeout(self):
        iterator = until_timeout(5)

        def now_iter():
            yield iterator.start
            yield iterator.start + timedelta(0, 4)
            yield iterator.start + timedelta(0, 5)
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            results = list(iterator)
        self.assertEqual([None, None], results)
