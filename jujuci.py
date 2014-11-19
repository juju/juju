"""Access Juju CI artifacts and data."""

import fnmatch
import json
import urllib2


def get_build_data(jenkins_url, job_name, build='lastSuccessfulBuild'):
    """Return a dict of the build data for a job build number."""
    build_data = urllib2.urlopen(
        '%s/job/%s/%s/api/json' % (jenkins_url, job_name, build))
    build_data = json.load(build_data)
    return build_data


def list_files(build_data, glob='*'):
    found = []
    for artifact in build_data['artifacts']:
        file_name = artifact['fileName']
        if fnmatch.fnmatch(file_name, glob):
            found.append(file_name)
    return found
