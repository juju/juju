// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

const (
	// controllerRoleAssumePolicy describes the polciy for the controller roll
	// stating what principals can assume the role. We only allow ec2 instances
	// in this case.
	controllerRoleAssumePolicy = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
	  "Principal": {
	    "Service": "ec2.amazonaws.com"
	  },
	  "Action": "sts:AssumeRole"
	}
  ]
}
`
	// controllerRolePolicy is the AWS IAM policy used for controller role
	// permissions. This JSON document must be kept in line with the AWS
	// permissions used by Juju.
	controllerRolePolicy = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "JujuEC2Actions",
      "Effect": "Allow",
      "Action": [
        "ec2:AssociateIamInstanceProfile",
        "ec2:AttachVolume",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:CreateSecurityGroup",
        "ec2:CreateTags",
        "ec2:CreateVolume",
        "ec2:DeleteSecurityGroup",
        "ec2:DeleteVolume",
        "ec2:DescribeAccountAttributes",
        "ec2:DescribeAvailabilityZones",
        "ec2:DescribeIamInstanceProfileAssociations",
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceTypes",
        "ec2:DescribeInternetGateways",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeRouteTables",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSpotPriceHistory",
        "ec2:DescribeSubnets",
        "ec2:DescribeVolumes",
        "ec2:DescribeVpcs",
        "ec2:DetachVolume",
        "ec2:RevokeSecurityGroupIngress",
        "ec2:RunInstances",
        "ec2:TerminateInstances"
      ],
      "Resource": "*"
    },
    {
      "Sid": "JujuIAMActions",
      "Effect": "Allow",
      "Action": [
	    "iam:AddRoleToInstanceProfile",
        "iam:CreateInstanceProfile",
		"iam:CreateRole",
		"iam:DeleteInstanceProfile",
		"iam:DeleteRole",
		"iam:DeleteRolePolicy",
        "iam:GetInstanceProfile",
		"iam:GetRole",
		"iam:ListInstanceProfiles",
		"iam:ListRolePolicies",
		"iam:ListRoles",
		"iam:PassRole",
		"iam:PutRolePolicy",
		"iam:RemoveRoleFromInstanceProfile"
      ],
      "Resource": "*"
    },
    {
      "Sid": "JujuSSMActions",
      "Effect": "Allow",
      "Action": [
        "ssm:ListInstanceAssociations",
        "ssm:UpdateInstanceInformation"
      ],
      "Resource": "*"
    }
  ]
}
`
)
