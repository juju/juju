"""Access Juju CI artifacts and data."""

import json
import urllib2


def get_build_data(jenkins_url, job_name, build='lastSuccessfulBuild'):
    build_data = urllib2.urlopen(
        '%s/job/%s/%s/api/json' % (jenkins_url, job_name, build))
    build_data = json.load(build_data)
    return build_data
