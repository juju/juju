/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pool

import (
	"flag"
	"strconv"
	"strings"

	"github.com/juju/govmomi/vim25/types"
)

type sharesInfo types.SharesInfo

func (s *sharesInfo) String() string {
	return string(s.Level)
}

func (s *sharesInfo) Set(val string) error {
	switch val {
	case string(types.SharesLevelNormal), string(types.SharesLevelLow), string(types.SharesLevelHigh):
		s.Level = types.SharesLevel(val)
	default:
		n, err := strconv.Atoi(val)
		if err != nil {
			return err
		}

		s.Level = types.SharesLevelCustom
		s.Shares = n
	}

	return nil
}

func NewResourceConfigSpecFlag() *ResourceConfigSpecFlag {
	f := new(ResourceConfigSpecFlag)
	f.SetAllocation(func(a *types.ResourceAllocationInfo) {
		a.Shares = new(types.SharesInfo)
	})
	return f
}

type ResourceConfigSpecFlag struct {
	types.ResourceConfigSpec
}

func (s *ResourceConfigSpecFlag) Process() error { return nil }

func (s *ResourceConfigSpecFlag) Register(f *flag.FlagSet) {
	opts := []struct {
		name  string
		units string
		*types.ResourceAllocationInfo
	}{
		{"CPU", "MHz", &s.CpuAllocation},
		{"Memory", "MB", &s.MemoryAllocation},
	}

	for _, opt := range opts {
		prefix := strings.ToLower(opt.name)[:3]
		shares := (*sharesInfo)(opt.Shares)

		expandableReservation := false
		if v := opt.ExpandableReservation; v != nil {
			expandableReservation = *v
		}

		f.Int64Var(&opt.Limit, prefix+".limit", 0, opt.name+" limit in "+opt.units)
		f.Int64Var(&opt.Reservation, prefix+".reservation", 0, opt.name+" reservation in "+opt.units)
		f.BoolVar(opt.ExpandableReservation, prefix+".expandable", expandableReservation, opt.name+" expandable reservation")
		f.Var(shares, prefix+".shares", opt.name+" shares level or number")
	}
}

func (s *ResourceConfigSpecFlag) SetAllocation(f func(*types.ResourceAllocationInfo)) {
	for _, a := range []*types.ResourceAllocationInfo{&s.CpuAllocation, &s.MemoryAllocation} {
		f(a)
	}
}
