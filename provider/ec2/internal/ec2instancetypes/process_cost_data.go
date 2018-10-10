// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils"
)

const (
	baseURL      = `https://pricing.us-east-1.amazonaws.com`
	ec2IndexPath = `/offers/v1.0/aws/AmazonEC2/current/index.json`
)

var (
	nowish = time.Now()
)

func Main() (int, error) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-o outfile] [json-file]:\n", os.Args[0])
	}

	var outfilename string
	flag.StringVar(&outfilename, "o", "-", "Name of a file to write the output to")
	flag.Parse()

	var infilename string
	var fin *os.File
	switch flag.NArg() {
	case 0:
		infilename = "<stdin>"
		fin = os.Stdin
	case 1:
		var err error
		infilename = flag.Arg(0)
		fin, err = os.Open(infilename)
		if err != nil {
			return -1, err
		}
		defer fin.Close()
	default:
		fmt.Println(flag.Args())
		flag.Usage()
		return 2, nil
	}

	fout := os.Stdout
	if outfilename != "-" {
		var err error
		fout, err = os.Create(outfilename)
		if err != nil {
			return -1, err
		}
		defer fout.Close()
	}

	tmpl := template.Must(template.New("instanceTypes").Parse(`
// Copyright {{.Year}} Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/environs/instances"
)

var (
	paravirtual = "pv"
	hvm         = "hvm"
	amd64       = []string{arch.AMD64}
	both        = []string{arch.AMD64, arch.I386}
)

// Version: {{.Meta.Version}}
// Publication date: {{.Meta.PublicationDate}}
//
// {{.Meta.Disclaimer}}

var allInstanceTypes = map[string][]instances.InstanceType{
{{range $region, $instanceTypes := .InstanceTypes}}
{{printf "%q: {" $region}}
{{range $index, $instanceType := $instanceTypes}}{{with $instanceType}}
  // SKU: {{.SKU}}
  // Instance family: {{.InstanceFamily}}
  // Storage: {{.Storage}}
  {
    Name:       {{printf "%q" .Name}},
    Arches:     {{.Arches}},
    CpuCores:   {{.CpuCores}},
    CpuPower:   instances.CpuPower({{.CpuPower}}),
    Mem:        {{.Mem}},
    VirtType:   &{{.VirtType}},
    Cost:       {{.Cost}},
    {{if .Deprecated}}Deprecated: true,{{end}}
  },
{{end}}{{end}}
},
{{end}}
}`))

	fmt.Fprintln(os.Stderr, "Processing", infilename)
	instanceTypes, meta, err := process(fin)
	if err != nil {
		return -1, err
	}

	templateData := struct {
		Year          int
		InstanceTypes map[string][]instanceType
		Meta          metadata
	}{
		nowish.Year(),
		instanceTypes,
		meta,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return -1, err
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return -1, err
	}
	if _, err := fout.Write(formatted); err != nil {
		return -1, err
	}

	return 0, nil
}

func process(in io.Reader) (map[string][]instanceType, metadata, error) {
	var index indexFile
	if err := json.NewDecoder(in).Decode(&index); err != nil {
		return nil, metadata{}, err
	}
	meta := metadata{
		Version:         index.Version,
		Disclaimer:      index.Disclaimer,
		PublicationDate: index.PublicationDate,
	}
	instanceTypes := make(map[string][]instanceType)
	skus := set.NewStrings()
	for sku := range index.Products {
		skus.Add(sku)
	}
	for _, sku := range skus.SortedValues() {
		productInfo := index.Products[sku]
		if productInfo.ProductFamily != "Compute Instance" {
			continue
		}
		if productInfo.OperatingSystem != "Linux" {
			// We don't care about absolute cost, so we don't need
			// to include the cost of OS.
			continue
		}
		if productInfo.Tenancy != "Shared" {
			continue
		}
		if productInfo.PreInstalledSW != "NA" {
			// We don't care about instances with pre-installed
			// software.
			continue
		}
		fmt.Fprintf(os.Stderr, "- Processing %q\n", sku)

		// Some instance types support both 32-bit and 64-bit, some
		// only support 64-bit.
		arches := "amd64"
		if productInfo.ProcessorArchitecture == "32-bit or 64-bit" {
			arches = "both"
		}

		// NOTE(axw) it's not really either/or. Some instance types are
		// capable of launching either HVM or PV images (e.g. T1, C3).
		// HVM is preferred, though, so we err on that side.
		virtType := "hvm"
		if isParavirtualOnly(productInfo) {
			virtType = "paravirtual"
		}

		memMB, err := parseMem(productInfo.Memory)
		if err != nil {
			return nil, metadata{}, errors.Annotate(err, "parsing mem")
		}

		cpuPower, err := calculateCPUPower(productInfo)
		if err != nil {
			return nil, metadata{}, errors.Annotate(err, "calculating CPU power")
		}

		instanceType := instanceType{
			Name:     productInfo.InstanceType,
			Arches:   arches,
			CpuCores: productInfo.VCPU,
			CpuPower: cpuPower,
			Mem:      memMB,
			VirtType: virtType,

			// Extended information
			SKU:            sku,
			InstanceFamily: productInfo.InstanceFamily,
			Storage:        productInfo.Storage,
		}
		if strings.ToLower(productInfo.CurrentGeneration) == "no" {
			instanceType.Deprecated = true
		}

		// Get cost information. We only support on-demand.
		for skuOfferTermCode, skuTerms := range index.Terms.OnDemand[sku] {
			if !skuTerms.EffectiveDate.Before(nowish) {
				continue
			}
			fmt.Fprintf(os.Stderr, "-- Processing offer %q\n", skuOfferTermCode)
			for skuOfferTermCodeRateCode, pricingDetails := range skuTerms.PriceDimensions {
				fmt.Fprintf(os.Stderr, "--- Processing rate code %q\n", skuOfferTermCodeRateCode)
				fmt.Fprintf(os.Stderr, "     Description: %s\n", pricingDetails.Description)
				fmt.Fprintf(os.Stderr, "     Cost: $%f/%s\n",
					pricingDetails.PricePerUnit.USD, pricingDetails.Unit,
				)
				instanceType.Cost = uint64(pricingDetails.PricePerUnit.USD * 1000)
				break
			}
		}

		region, ok := locationToRegion(productInfo.Location)
		if !ok {
			return nil, metadata{}, errors.Errorf("unknown location %q", productInfo.Location)
		}
		if !supported(region, instanceType.Name) {
			continue
		}

		regionInstanceTypes := instanceTypes[region]
		regionInstanceTypes = append(regionInstanceTypes, instanceType)
		instanceTypes[region] = regionInstanceTypes
	}
	return instanceTypes, meta, nil
}

// It appears that instances sometimes show up in the offer list which aren't
// available in actual regions. e.g. m3.medium is in the offer list for
// ap-northeast-2, but attempting to launch one returns an unsupported error.
// See: https://bugs.launchpad.net/juju/+bug/1663047
func supported(region, instanceType string) bool {
	switch region {
	case "ap-northeast-2":
		switch instanceType[:2] {
		case "c3", "g2", "m3":
			return false
		default:
			return true
		}
	default:
		return true
	}
}

func calculateCPUPower(info productInfo) (uint64, error) {
	// T-class instances have burstable CPU. This is not captured
	// in the pricing information, so we have to hard-code it. We
	// will have to update this list when T3 instances come along.
	switch info.InstanceType {
	case "t1.micro":
		return 20, nil
	case "t2.nano":
		return 5, nil
	case "t2.micro":
		return 10, nil
	case "t2.small":
		return 20, nil
	case "t2.medium":
		return 40, nil
	case "t2.large":
		return 60, nil
	}
	if info.ClockSpeed == "" {
		return info.VCPU * 100, nil
	}

	// If the information includes a clock speed, we use that
	// to estimate the ECUs. The pricing information does not
	// include the ECUs, but they're only estimates anyway.
	// Amazon moved to "vCPUs" quite some time ago.
	// To date, info.ClockSpeed can have the form "Up to <float> GHz" or
	// "<float> GHz", so look for a float match.
	validSpeed := regexp.MustCompile(`[0-9]+\.?[0-9]*`)
	clock, err := strconv.ParseFloat(validSpeed.FindString(info.ClockSpeed), 64)
	if err != nil {
		return 0, errors.Annotate(err, "parsing clock speed")
	}
	return uint64(clock * 1.4 * 100 * float64(info.VCPU)), nil
}

func parseMem(s string) (uint64, error) {
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, ",", "", -1) // e.g. 1,952 -> 1952

	// Sometimes it's GiB, sometimes Gib. We don't like Gib.
	s = strings.Replace(s, "Gib", "GiB", 1)
	return utils.ParseSize(s)
}

func locationToRegion(loc string) (string, bool) {
	regions := map[string]string{
		"US East (N. Virginia)":      "us-east-1",
		"US East (Ohio)":             "us-east-2",
		"US West (N. California)":    "us-west-1",
		"US West (Oregon)":           "us-west-2",
		"Canada (Central)":           "ca-central-1",
		"EU (Frankfurt)":             "eu-central-1",
		"EU (Ireland)":               "eu-west-1",
		"EU (London)":                "eu-west-2",
		"EU (Paris)":                 "eu-west-3",
		"Asia Pacific (Tokyo)":       "ap-northeast-1",
		"Asia Pacific (Seoul)":       "ap-northeast-2",
		"Asia Pacific (Osaka-Local)": "ap-northeast-3",
		"Asia Pacific (Singapore)":   "ap-southeast-1",
		"Asia Pacific (Sydney)":      "ap-southeast-2",
		"Asia Pacific (Mumbai)":      "ap-south-1",
		"South America (Sao Paulo)":  "sa-east-1",
		"AWS GovCloud (US)":          "us-gov-west-1",

		// NOTE(axw) at the time of writing, there is no
		// pricing information for cn-north-1.
		"China (Beijing)": "cn-north-1",
	}
	region, ok := regions[loc]
	return region, ok
}

func isParavirtualOnly(info productInfo) bool {
	// Only very old instance types are restricted to paravirtual.
	switch strings.SplitN(info.InstanceType, ".", 2)[0] {
	case "t1", "m1", "c1", "m2":
		return true
	}
	return false
}

type metadata struct {
	Version         string
	PublicationDate time.Time
	Disclaimer      string
}

type indexFile struct {
	Version         string                 `json:"version"`
	PublicationDate time.Time              `json:"publicationDate"`
	Disclaimer      string                 `json:"disclaimer"`
	Products        map[string]productInfo `json:"products"`
	Terms           terms                  `json:"terms"`
}

type productInfo struct {
	ProductFamily     string `json:"productFamily"` // Compute Instance
	ProductAttributes `json:"attributes"`
}

type ProductAttributes struct {
	Location              string `json:"location"`              // e.g. US East (N. Virginia)
	InstanceType          string `json:"instanceType"`          // e.g. t2.nano
	CurrentGeneration     string `json:"currentGeneration"`     // Yes|No (or missing)
	InstanceFamily        string `json:"instanceFamily"`        // e.g. Storage optimised
	Storage               string `json:"storage"`               // e.g. 24 x 2000
	VCPU                  uint64 `json:"vcpu,string"`           // e.g. 16
	ClockSpeed            string `json:"clockSpeed"`            // e.g. 2.5 GHz
	Memory                string `json:"memory"`                // N.NN GiB
	OperatingSystem       string `json:"operatingSystem"`       // Windows|RHEL|SUSE|Linux
	Tenancy               string `json:"tenancy"`               // Dedicated|Host|Shared
	ProcessorArchitecture string `json:"processorArchitecture"` // (32-bit or )?64-bit
	PreInstalledSW        string `json:"preInstalledSw"`        // e.g. NA|SQL Web|SQL Std
}

type terms struct {
	OnDemand map[string]map[string]skuTerms `json:"OnDemand"`
}

type skuTerms struct {
	EffectiveDate   time.Time                 `json:"effectiveDate"`
	PriceDimensions map[string]pricingDetails `json:"priceDimensions"`
}

type pricingDetails struct {
	Description  string `json:"description"`
	Unit         string `json:"unit"`
	PricePerUnit struct {
		USD float64 `json:"USD,string"`
	} `json:"pricePerUnit"`
}

type instanceType struct {
	Name       string
	Arches     string // amd64|both
	CpuCores   uint64
	Mem        uint64
	Cost       uint64 // paravirtual|hvm
	VirtType   string
	CpuPower   uint64
	Deprecated bool // i.e. not current generation

	// extended information, for comments
	SKU            string
	InstanceFamily string
	Storage        string
}

func main() {
	rc, err := Main()
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(rc)
}
