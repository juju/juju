package ec2

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goamz/ec2"
)

type internalSuite struct{}

var _ = Suite(internalSuite{})

var samePermsTests = []struct {
	same bool
	p0   []ec2.IPPerm
	p1   []ec2.IPPerm
}{
	{true,
		nil, nil},
	{false,
		nil, []ec2.IPPerm{{}}},
	{true,
		[]ec2.IPPerm{{}}, []ec2.IPPerm{{}}},
	{true,
		[]ec2.IPPerm{{
			Protocol:  "a",
			FromPort:  1,
			ToPort:    2,
			SourceIPs: []string{"y", "x"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored0",
				OwnerId: "ignored0also",
				Name:    "g1",
			}},
		}, {
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}, {
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}},
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"z", "w"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "other0",
				OwnerId: "other1",
				Name:    "g2",
			}, {
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}},
		}, {
			Protocol:  "a",
			FromPort:  1,
			ToPort:    2,
			SourceIPs: []string{"x", "y"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "other2",
				OwnerId: "other3",
				Name:    "g1",
			}},
		}}},
	{false,
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}, {
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}},
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}}}}


// copyPerms makes a copy of the permissions
// so that samePerms won't change the original.
func copyPerms(ps []ec2.IPPerm) []ec2.IPPerm {
	rs := make([]ec2.IPPerm, len(ps))
	for i, p := range ps {
		r := &rs[i]
		*r = p
		r.SourceIPs = make([]string, len(p.SourceIPs))
		copy(r.SourceIPs, p.SourceIPs)
		r.SourceGroups = make([]ec2.UserSecurityGroup, len(p.SourceGroups))
		copy(r.SourceGroups, p.SourceGroups)
	}
	return rs
}
