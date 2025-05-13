// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package domain implements domain services.
// Each domain service is responsible for providing APIs pertaining
// to a logical domain. Domains may cross entity boundaries; the
// partitioning is done based on packaging cohesive functional behaviour,
// not individual entity concerns.
//
// # Package layout and conventions
//
// Each domain service package has several key artefacts:
//   - the service providing public APIs called by API server facades.
//   - the state providing functionality to read and write persistent data.
//   - params structs which are used as arguments/results for service API calls.
//   - arg structs which are used as arguments/results for state calls.
//   - type structs which are used a sqlair in/out parameters.
//
// The layout of a domain package providing a service is as follows:
//
//		domain/foo/
//		 |- types.go [1]
//		 |- params.go [2]
//		 |- service/
//		   |- service.go
//		 |- state/
//		   |- state.gp
//		   |- types.go [3]
//		 |- errors/
//		   |- errors.go
//		 |- bootstrap/
//		   |- bootstrap.go
//		 |- modelmigration/
//		   |- import.go
//		   |- export.go
//
//	 [1] contains DTOs used as arguments/results for state calls
//	 [2] optional - contains structs used as arguments/results for service API calls
//	 [3] contains package private structs which act as in/out params for sqlair.
//
// At the time of writing, many domain entity related structs are defined in
// juju/core. Where the sole use of these structs is to fulfil operational
// requirements of the relevant domain package, and there's no use outside
// the package, they can be moved down from core.
//
// To avoid name clashes and promote consistency of implementation, a naming
// convention is used when defining structs used as method args and results.
// Some key conventions are as follows:
//   - structs used as service API call args are named xxxParams.
//   - structs used as state call args are named xxxArgs.
//
// eg
//
//	func(s *Service) DoWork(ctx context.Context, p WorkParams) error {
//	    args := foo.ProgressArgs{
//	        StartedAt: time.Now(),
//	    }
//	    return s.st.RecordStart(ctx, args)
//	}
//
// # Testing
//
// For the state layer, test suites embed a base suite depending on
// whether they are operating on a model or controller database.
//
// eg
//
//	type applicationStateSuite struct {
//	    domaintesting.ModelSuite
//	}
//
// Tests that need to seed database rows or read rows to
// evaluate results may do so using a sql txn. Note that
// any test assertions must be done outside the transaction.
//
// eg
//
//	var gotFoo string
//	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
//	    err := tx.QueryRowContext(ctx, `SELECT foo FROM table`).Scan(&gotFoo)
//	    return err
//	})
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(gotFoo, tc.Equals, "bar")
//
// Tests are implemented by invoking business methods being tested
// on a state instance created from the base suite txn runner factory.
//
// eg
//
//	st = NewFooState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
//	err := st.Foo(context.Background())
//
// Service layer tests are implemented using mocks which are set up via package_test.
//
// eg
//
//	//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/foo/service FooState
//
// # Implementation notes
//
// Implicit in the package and file structure defined above are the following
// rules which must be observed:
//   - service and state packages must remain decoupled and not import from each other.
//
// Package bootstrap provides methods called by [github.com/juju/juju/agent/agentbootstrap.Initialize] to
// seed the controller database with data required for a fully initialised controller.
//
// Package modelmigration provides methods called by
// [github.com/juju/juju/domain/modelmigration.RegisterExport] and
// [github.com/juju/juju/domain/modelmigration.RegisterImport] in order to implement
// the export and import of model artefacts for migration.
package domain
