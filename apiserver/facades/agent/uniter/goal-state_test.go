// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

// uniterSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type uniterGoalStateSuite struct{}

func TestUniterGoalStateSuite(t *stdtesting.T) { tc.Run(t, &uniterGoalStateSuite{}) }
func (s *uniterGoalStateSuite) TestStub(c *tc.C) {
	c.Skip(`
Given the initial state where:
- 3 machines exist
- A wordpress charm is deployed with a single unit to machine 0, with an unset status
- A mysql charm is deployed with a single unit to machine 1, with an unset status
- A logging charm is deployed with a single unit to machine 2, with an unset status
- An authoriser is congured to mock a logged in mysql unit

This suite is missing tests for the following scenarios:
- TestGoalStatesNoRelation: when no relations exist,
  - GoalStates with the mysql/0 unit tag returns a "waiting" status for the unit only
  - GoalStates with the wordpress/0 unit tag returns an unauthorized error

- TestPeerUnitsNoRelation: when another mysql unit is deplouye to machine 1,
  - GoalStates with the mysql/0 unit tag returns a "waiting" status for both mysql units
  - GoalStates with the wordpress/0 unit tag returns an unauthorized error

- TestGoalStatesSingleRelation: when a relation is added between the wordpress and mysql units,
  - GoalStates with the mysql/0 unit tag returns a "waiting" status for the unit AND
    (a waiting status for the wordpress unit AND a joining status for the relation between the units
    indexed under the correct relation name)
  - GoalStates with the wordpress/0 unit tag returns an unauthorized error

- TestGoalStatesDeadUnitsExcluded: when a unit is destroyed, it's status is no longer
  included in the GoalStates result

- TestGoalStatesSingleRelationDyingUnits: when a unit is dying, it's status is included
  in the GoalStates result, but it's goal status is switched to dying

- TestGoalStatesCrossModelRelation: when a relation is added between the mysql unit and a cross-model
  relation is established, but relations are included in the GoalStates result with the URL as the key
  for the cmr relation (where previously the application name was used)

- TestGoalStatesMultipleRelations: when:
  - a second wordpress unit is added to machine 1
  - a second wordpress application is deployed with a single unit to machine 1
  - a second mysql application is deployed with a single unit to machine 2
  - both wordpress applications are related to the original mysql application
  - both mysql applications are related to the logging application
then:
  - GoalStates with the mysql/0 unit tag returns a "waiting" status for the unit AND (
    the two related wordpress applications AND their three units have the expected statuses indexed
    under the endpoint key AND the related loggin application and unit have the expected statuses
    indexed under a different endpoint key
  - GoalStates with the wordpress/0 unit tag returns an unauthorized error
    )
`)
}
