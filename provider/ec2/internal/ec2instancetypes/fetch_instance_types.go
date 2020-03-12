// Copyright 2020 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build ignore

package main

import (
	"io/ioutil"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

type Doc struct {
	Regions map[string]Region `yaml:"regions"`
}

type Region struct {
	AvailabilityZones map[string]AvailabilityZone `yaml:"availability-zones"`
}

type AvailabilityZone struct {
	InstanceTypes []string `yaml:"instance-types"`
}

func main() {
	sess := session.Must(session.NewSession())
	svc := ec2.New(sess, &aws.Config{
		Region: aws.String(endpoints.ApSoutheast1RegionID),
	})

	doc, err := resolve(sess, svc)
	if err != nil {
		log.Fatal(err)
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile("data.yaml", data, 0600)
	if err != nil {
		log.Fatal(err)
	}
}

func resolve(sess *session.Session, svc *ec2.EC2) (Doc, error) {
	log.Println("resolving regions...")
	regions, err := svc.DescribeRegions(&ec2.DescribeRegionsInput{
		AllRegions: boolPtr(true),
	})
	if err != nil {
		return Doc{}, errors.Trace(err)
	}

	doc := Doc{
		Regions: make(map[string]Region),
	}
	for _, v := range regions.Regions {
		regionID := *v.RegionName
		region, err := resolveRegion(ec2.New(sess, &aws.Config{
			Region: aws.String(regionID),
		}), regionID)
		if err != nil {
			return Doc{}, errors.Annotatef(err, "processing region %s", regionID)
		}
		doc.Regions[regionID] = region
	}

	return doc, nil
}

func resolveRegion(svc *ec2.EC2, regionID string) (Region, error) {
	log.Printf("resolving regions %q...\n", regionID)
	azs, err := svc.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{
		AllAvailabilityZones: boolPtr(true),
		Filters: []*ec2.Filter{
			{
				Name:   stringPtr("region-name"),
				Values: []*string{stringPtr(regionID)},
			},
		},
	})
	if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "AuthFailure" {
		return Region{}, nil
	} else if err != nil {
		return Region{}, errors.Trace(err)
	}

	region := Region{
		AvailabilityZones: make(map[string]AvailabilityZone),
	}
	for _, az := range azs.AvailabilityZones {
		azID := *az.ZoneId
		az, err := resolveAvailabilityZone(svc, regionID, azID)
		if err != nil {
			return Region{}, errors.Annotatef(err, "failed resolving az %s", azID)
		}
		region.AvailabilityZones[azID] = az
	}

	return region, nil
}

func resolveAvailabilityZone(svc *ec2.EC2, regionID string, azID string) (AvailabilityZone, error) {
	log.Printf("resolving availability zone %q...\n", azID)
	az := AvailabilityZone{}
	req := &ec2.DescribeInstanceTypeOfferingsInput{
		Filters: []*ec2.Filter{
			{
				Name:   stringPtr("location"),
				Values: []*string{stringPtr(azID)},
			},
		},
		LocationType: stringPtr("availability-zone-id"),
		MaxResults:   int64Ptr(1000),
	}
	more := true
	for more {
		res, err := svc.DescribeInstanceTypeOfferings(req)
		if err != nil {
			return AvailabilityZone{}, errors.Trace(err)
		}

		for _, v := range res.InstanceTypeOfferings {
			az.InstanceTypes = append(az.InstanceTypes, *v.InstanceType)
		}

		more = res.NextToken != nil
		req.NextToken = res.NextToken
	}

	return az, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}
