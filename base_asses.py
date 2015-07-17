from unittest import FunctionTestCase


def toUnitTest(testCase):
    def unitTestify(*args, **kwargs):
        tc = FunctionTestCase(testCase)
        largs = list(args)
        largs.insert(1, tc)
        args = tuple(largs)
        return testCase(*args, **kwargs)

    return unitTestify
