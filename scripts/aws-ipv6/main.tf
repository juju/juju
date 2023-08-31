terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "~> 4.65.0"
    }
  }
}

provider "aws" {
}

resource "aws_vpc" "v6_only" {
  enable_dns_support = true
  enable_dns_hostnames = true
}