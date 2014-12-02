from mock import patch
from unittest import TestCase

from candidate import (
    find_publish_revision_number,
)


class CandidateTestCase(TestCase):

    @staticmethod
    def make_publish_revision_build_data(*args, **kwargs):
        if kwargs['build'] == 'lastSuccessfulBuild':
            number = 1235
        else:
            number = kwargs['build']
        return {
            'number': number,
            'description': 'Revision build: %s' % number,
        }

    def test_find_publish_revision_number(self):
        with patch('candidate.get_build_data',
                   side_effect=self.make_publish_revision_build_data) as mock:
            found_number = find_publish_revision_number(1234, limit=2)
        self.assertEqual(1234, found_number)
        self.assertEqual(2, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        self.assertEqual(
            ('http://juju-ci.vapour.ws:8080', 'publish-revision'), args)
        self.assertEqual('lastSuccessfulBuild', kwargs['build'])
        output, args, kwargs = mock.mock_calls[1]
        self.assertEqual(
            ('http://juju-ci.vapour.ws:8080', 'publish-revision'), args)
        self.assertEqual(1234, kwargs['build'])

    def test_find_publish_revision_number_no_match(self):
        with patch('candidate.get_build_data',
                   side_effect=self.make_publish_revision_build_data) as mock:
            found_number = find_publish_revision_number(1, limit=2)
        self.assertEqual(None, found_number)
        self.assertEqual(2, mock.call_count)

    def test_find_publish_revision_number_no_build_data(self):
        with patch('candidate.get_build_data', return_value=None) as mock:
            found_number = find_publish_revision_number(1, limit=5)
        self.assertEqual(None, found_number)
        self.assertEqual(1, mock.call_count)
