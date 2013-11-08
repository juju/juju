__metaclass__ = type

from unittest import TestCase

from jujupy import ErroredUnit


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('foo', 'bar', 'baz')
        self.assertEqual('<foo> bar is in state baz', str(e))



