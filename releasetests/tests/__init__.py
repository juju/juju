from StringIO import StringIO
from unittest import TestCase

from mock import patch


def autopatch(target, **kwargs):
    return patch(target, autospec=True, **kwargs)


class QuietTestCase(TestCase):

    def setUp(self):
        super(QuietTestCase, self).setUp()
        self.stdout = StringIO()
        patcher = patch('sys.stdout', self.stdout)
        patcher.start()
        self.addCleanup(patcher.stop)
