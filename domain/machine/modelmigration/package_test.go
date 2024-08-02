package modelmigration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/machine/modelmigration Coordinator,ImportService,ExportService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
