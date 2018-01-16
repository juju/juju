from tests import (
    TestCase
)

from jujupy.exceptions import (
    AppError,
    MachineError,
    ProvisioningError,
    StuckAllocatingError,
    StatusError,
    UnitError,
)


class TestStatusErrorTree(TestCase):
    """TestCase for StatusError and the tree of exceptions it roots."""

    def test_priority(self):
        pos = len(StatusError.ordering) - 1
        self.assertEqual(pos, StatusError.priority())

    def test_priority_mass(self):
        for index, error_type in enumerate(StatusError.ordering):
            self.assertEqual(index, error_type.priority())

    def test_priority_children_first(self):
        for index, error_type in enumerate(StatusError.ordering, 1):
            for second_error in StatusError.ordering[index:]:
                self.assertFalse(issubclass(second_error, error_type))

    def test_priority_pairs(self):
        self.assertLess(MachineError.priority(), UnitError.priority())
        self.assertLess(UnitError.priority(), AppError.priority())
        self.assertLess(StuckAllocatingError.priority(),
                        MachineError.priority())
        self.assertLess(ProvisioningError.priority(),
                        StuckAllocatingError.priority())
