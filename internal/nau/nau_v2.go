package nau

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// NauCalculatorV2 provides NAU calculation functionality for AWS VPC resources.
type NauCalculatorV2 interface {
	// CalculateNau computes NAU totals for all VPCs in the region.
	CalculateNau() (map[string]int64, error)
	// GetRegion returns the AWS region being processed.
	GetRegion() string
}

// eniJob represents work to be processed by worker pool.
type eniJob struct {
	eni    ec2Types.NetworkInterface
	vpcId  string
	region string
}

// eniResult contains the output from processing an ENI.
type eniResult struct {
	records []NauRecord
	err     error
}

// nauCalculatorV2Impl implements NAU calculation using EC2 network interface analysis.
type nauCalculatorV2Impl struct {
	ctx      context.Context     // Context for AWS API calls
	ec2      ec2client.Ec2Client // EC2 client for resource discovery
	nauStore AccountNauStore     // Storage for NAU calculations
	wt       NauWeightTable      // Weight table for resource types
	logger   logger.Logger       // Logger instance

	region string // AWS region being processed

	// Worker pool components
	jobs        chan eniJob
	results     chan eniResult
	workerCount int
	workerWg    sync.WaitGroup
	collectorWg sync.WaitGroup   // WaitGroup for result collector
}

// NewNauCalculatorV2 creates a new NAU calculator instance for the specified region.
func NewNauCalculatorV2(ctx context.Context, ec2 ec2client.Ec2Client, nauStore AccountNauStore, logger logger.Logger) NauCalculatorV2 {
	calc := &nauCalculatorV2Impl{
		ctx:         ctx,
		ec2:         ec2,
		nauStore:    nauStore,
		logger:      logger,
		wt:          NewNauWeightTable(),
		region:      ec2.GetRegion(),
		workerCount: 20,
		jobs:        make(chan eniJob, 100),
		results:     make(chan eniResult, 1000),
	}

	calc.startWorkerPool()
	return calc
}

// startWorkerPool initializes and starts the worker goroutines.
func (n *nauCalculatorV2Impl) startWorkerPool() {
	// Start workers
	for i := 0; i < n.workerCount; i++ {
		n.workerWg.Add(1)
		go n.worker()
	}
	
	// Start result collector
	n.collectorWg.Add(1)
	go n.collectResults()
}

// worker processes ENI jobs from the jobs channel.
func (n *nauCalculatorV2Impl) worker() {
	defer n.workerWg.Done()
	for job := range n.jobs {
		convertedEniType := toNetworkInterfaceType(job.eni.InterfaceType, n.logger)
		records, err := calculateNauForEni(job.eni, convertedEniType, n.wt, job.region)
		
		n.results <- eniResult{
			records: records,
			err:     err,
		}
	}
}

// collectResults processes results from workers and adds records to the store.
func (n *nauCalculatorV2Impl) collectResults() {
	defer n.collectorWg.Done()
	for result := range n.results {
		if result.err != nil {
			if errors.Is(result.err, errUnsupportedEni) {
				logUnsupportedEniType(result.err, n.logger)
				continue
			}
			if errors.Is(result.err, errNonAttachedEni) {
				logNonAttachedEni(result.err, n.logger)
				continue
			}
			n.handleError(result.err)
			continue
		}
		
		// Add records to store - manifest handles thread-safety internally
		for _, record := range result.records {
			n.nauStore.AddRecord(record)
		}
	}
}

// GetRegion returns the AWS region this calculator is processing.
func (n *nauCalculatorV2Impl) GetRegion() string {
	return n.region
}

// CalculateNau performs NAU calculation for all VPCs in the region.
// Returns a map of VPC ID to total NAU count, or an error if calculation fails.
func (n *nauCalculatorV2Impl) CalculateNau() (map[string]int64, error) {
	client := n.ec2
	p := ec2.NewDescribeVpcsPaginator(client, &ec2.DescribeVpcsInput{}, func(o *ec2.DescribeVpcsPaginatorOptions) {
		o.Limit = paginationLimit
	})

	for p.HasMorePages() {
		output, err := p.NextPage(n.ctx)
		if err != nil {
			return nil, n.handleError(err)
		}
		for _, vpc := range output.Vpcs {
			if vpc.VpcId != nil {
				err := n.calculateNauPerVpc(aws.ToString(vpc.VpcId))
				if err != nil {
					logFailedVpc(aws.ToString(vpc.VpcId), err, n.logger)
				}
			}
		}
	}

	// Wait for all workers to complete
	close(n.jobs)
	n.workerWg.Wait()
	close(n.results)
	// Wait for result collector to finish processing all results
	n.collectorWg.Wait()

	n.logger.Info("Completed calculating Network Address Units for all VPC's in %s", n.region)
	totals := make(map[string]int64)
	n.nauStore.RangeVPCs(func(vpcId string, vpc *VPCNAU) bool {
		var sum int64
		vpc.Range(func(_ ResourceKey, stats *ResourceStats) bool {
			sum += stats.Weight.Load()
			return true
		})
		totals[vpcId] = sum
		n.logger.Info("%s Network Address Unit total = %d", vpcId, sum)
		return true
	})

	return totals, nil
}

// handleError logs and returns the provided error.
func (n *nauCalculatorV2Impl) handleError(err error) error {
	n.logger.Error(err.Error())
	return err
}

// calculateNauPerVpc processes all network interfaces in a specific VPC
// by sending them to the worker pool for parallel processing.
func (n *nauCalculatorV2Impl) calculateNauPerVpc(vpcId string) error {
	p := ec2.NewDescribeNetworkInterfacesPaginator(n.ec2, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcId}},
			{Name: aws.String("status"), Values: []string{"in-use"}},
		},
	}, func(o *ec2.DescribeNetworkInterfacesPaginatorOptions) {
		o.Limit = paginationLimit
	})

	for p.HasMorePages() {
		output, err := p.NextPage(n.ctx)
		if err != nil {
			return err
		}

		// Send ENIs to worker pool for parallel processing
		for _, eni := range output.NetworkInterfaces {
			n.jobs <- eniJob{
				eni:    eni,
				vpcId:  vpcId,
				region: n.region,
			}
		}
	}
	return nil
}

// calculateNauForEni determines the NAU records for a specific network interface
// based on its type and configuration.
func calculateNauForEni(eni ec2Types.NetworkInterface, convertedEniType NetworkInterfaceType, wt NauWeightTable, region string) ([]NauRecord, error) {

	switch convertedEniType {

	case NetworkInterfaceTypeEfs:
		records := makeNauRecords(eni, EFSMountTarget, wt, region)
		return records, nil

	case NetworkInterfaceTypeBranch:
		records := makeNauRecords(eni, EKSPod, wt, region)
		return records, nil

	case NetworkInterfaceTypeInterface:
		if eni.Attachment == nil {
			return nil, fmt.Errorf("%w, id=%s, type=%s, description=%s", errNonAttachedEni, aws.ToString(eni.NetworkInterfaceId), eni.InterfaceType, aws.ToString(eni.Description))
		}

		var ipNauRecords []NauRecord
		var prefixNauRecords []NauRecord
		var eniNauRecords []NauRecord

		// primary EC2 Eni
		if eni.Attachment.DeviceIndex != nil && aws.ToInt32(eni.Attachment.DeviceIndex) == 0 {
			ipNauRecords = makeNauRecords(eni, IPv4IPv6Address, wt, region)
			return ipNauRecords, nil
		} else {
			// additional network interfaces
			eniNauRecords = makeNauRecords(eni, ENI, wt, region)
			if (len(eni.Ipv4Prefixes) > 0) || len(eni.Ipv6Prefixes) > 0 {
				prefixNauRecords = makeNauRecords(eni, PrefixAssignedToENI, wt, region)
			}
			totalRecords := make([]NauRecord, 0, len(eniNauRecords)+len(prefixNauRecords))
			totalRecords = append(totalRecords, eniNauRecords...)
			totalRecords = append(totalRecords, prefixNauRecords...)
			return totalRecords, nil
		}

	case NetworkInterfaceTypeTrunk,
		NetworkInterfaceTypeLoadBalancer,
		NetworkInterfaceTypeGlobalAcceleratorManaged:
		eniNauRecords := makeNauRecords(eni, ENI, wt, region)
		return eniNauRecords, nil

	case NetworkInterfaceTypeEfa, NetworkInterfaceTypeEfaOnly:
		records := makeNauRecords(eni, EFAInterface, wt, region)
		return records, nil

	case NetworkInterfaceTypeLambda:
		records := makeNauRecords(eni, LambdaFunction, wt, region)
		return records, nil

	case NetworkInterfaceTypeNatGateway:
		records := makeNauRecords(eni, NATGateway, wt, region)
		return records, nil

	case NetworkInterfaceTypeNetworkLoadBalancer:
		records := makeNauRecords(eni, NetworkLoadBalancerPerAZ, wt, region)
		return records, nil

	case NetworkInterfaceTypeGatewayLoadBalancer:
		records := makeNauRecords(eni, GatewayLoadBalancerPerAZ, wt, region)
		return records, nil

	case NetworkInterfaceTypeVpcEndpoint,
		NetworkInterfaceTypeApiGatewayManaged,
		NetworkInterfaceTypeAwsCodestarConnectionsManaged,
		NetworkInterfaceTypeIotRulesManaged,
		NetworkInterfaceTypeQuicksight,
		NetworkInterfaceTypeGatewayLoadBalancerEndpoint,
		NetworkInterfaceTypeEvs,
		NetworkInterfaceTypeEc2InstanceConnect:

		records := makeNauRecords(eni, VPCEndpointPerAZ, wt, region)
		return records, nil

	case NetworkInterfaceTypeTransitGateway:
		records := makeNauRecords(eni, TransitGatewayAttachment, wt, region)
		return records, nil

	default:
		return nil, fmt.Errorf("%w id=%s, type=%s, description=%s", errUnsupportedEni, aws.ToString(eni.NetworkInterfaceId), eni.InterfaceType, aws.ToString(eni.Description))
	}
}

// makeNauRecords creates NAU records for an ENI based on the specified resource key.
func makeNauRecords(eni ec2Types.NetworkInterface, key ResourceKey, wt NauWeightTable, region string) []NauRecord {
	w := wt.Get(key)
	meta := createResourceMetadataForEni(eni, key, w, region)

	switch key {
	case IPv4IPv6Address:
		return buildIpRecords(eni, w, meta)
	case PrefixAssignedToENI:
		return buildPrefixRecords(eni, w, meta)
	default:
		return []NauRecord{
			{
				VpcID:       aws.ToString(eni.VpcId),
				ResourceKey: key,
				Weight:      w,
				Metadata:    meta,
			}}
	}
}

// buildIpRecords creates individual NAU records for each IP address on an ENI.
func buildIpRecords(eni ec2Types.NetworkInterface, weight int64, meta ResourceMetadata) []NauRecord {
	totalRecords := make([]NauRecord, 0)
	ipv4Ips := fetchIpv4Ips(eni.PrivateIpAddresses)
	ipv6Ips := fetchIpv6Ips(eni.Ipv6Addresses)

	for _, ip := range ipv4Ips {
		record := NauRecord{
			VpcID:       aws.ToString(eni.VpcId),
			ResourceKey: IPv4IPv6Address,
			Weight:      weight,
		}
		copy := copyMetadata(meta)
		copy.Ipv4Address = ip
		record.Metadata = copy
		totalRecords = append(totalRecords, record)
	}

	for _, ip := range ipv6Ips {
		record := NauRecord{
			VpcID:       aws.ToString(eni.VpcId),
			ResourceKey: IPv4IPv6Address,
			Weight:      weight,
		}
		copy := copyMetadata(meta)
		copy.Ipv6Address = ip
		record.Metadata = copy
		totalRecords = append(totalRecords, record)
	}
	return totalRecords
}

// buildPrefixRecords creates NAU records for CIDR prefixes assigned to an ENI.
func buildPrefixRecords(eni ec2Types.NetworkInterface, weight int64, meta ResourceMetadata) []NauRecord {
	ipv4Prefixes := fetchIpv4Prefixes(eni.Ipv4Prefixes)
	ipv6Prefixes := fetchIpv6Prefixes(eni.Ipv6Prefixes)
	totalRecords := make([]NauRecord, 0, len(ipv4Prefixes)+len(ipv6Prefixes))

	for _, prefix := range ipv4Prefixes {
		record := NauRecord{
			VpcID:       aws.ToString(eni.VpcId),
			ResourceKey: PrefixAssignedToENI,
			Weight:      weight,
		}
		copy := copyMetadata(meta)
		copy.Ipv4Prefix = prefix
		record.Metadata = copy
		totalRecords = append(totalRecords, record)
	}

	for _, prefix := range ipv6Prefixes {
		record := NauRecord{
			VpcID:       aws.ToString(eni.VpcId),
			ResourceKey: PrefixAssignedToENI,
			Weight:      weight,
		}
		copy := copyMetadata(meta)
		copy.Ipv6Prefix = prefix
		record.Metadata = copy
		totalRecords = append(totalRecords, record)
	}
	return totalRecords
}

// createResourceMetadataForEni extracts metadata from an ENI for NAU record creation.
func createResourceMetadataForEni(eni ec2Types.NetworkInterface, nauResourceType ResourceKey, weight int64, region string) ResourceMetadata {
	return ResourceMetadata{
		NauResourceType:  nauResourceType,
		NauWeight:        weight,
		Id:               aws.ToString(eni.NetworkInterfaceId),
		ResourceType:     string(eni.InterfaceType),
		VpcId:            aws.ToString(eni.VpcId),
		Region:           region,
		AvailabilityZone: aws.ToString(eni.AvailabilityZone),
		SubnetId:         aws.ToString(eni.SubnetId),
		Description:      aws.ToString(eni.Description),
	}
}

// fetchIpv4Ips extracts all IPv4 addresses (private and public) from an ENI.
func fetchIpv4Ips(ips []ec2Types.NetworkInterfacePrivateIpAddress) []string {
	records := make([]string, 0)
	for _, ipv4Ip := range ips {
		records = append(records, aws.ToString(ipv4Ip.PrivateIpAddress))
		if ipv4Ip.Association != nil && ipv4Ip.Association.PublicIp != nil {
			records = append(records, aws.ToString(ipv4Ip.Association.PublicIp))
		}
	}
	return records
}

// fetchIpv6Ips extracts all IPv6 addresses from an ENI.
func fetchIpv6Ips(ips []ec2Types.NetworkInterfaceIpv6Address) []string {
	records := make([]string, 0, len(ips))
	for _, ipv6Ip := range ips {
		records = append(records, aws.ToString(ipv6Ip.Ipv6Address))
	}
	return records
}

// fetchIpv4Prefixes extracts all IPv4 CIDR prefixes from an ENI.
func fetchIpv4Prefixes(prefixes []ec2Types.Ipv4PrefixSpecification) []string {
	records := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		records = append(records, aws.ToString(prefix.Ipv4Prefix))
	}
	return records
}

// fetchIpv6Prefixes extracts all IPv6 CIDR prefixes from an ENI.
func fetchIpv6Prefixes(prefixes []ec2Types.Ipv6PrefixSpecification) []string {
	records := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		records = append(records, aws.ToString(prefix.Ipv6Prefix))
	}
	return records
}

// logFailedVpc logs a warning when VPC NAU calculation fails.
func logFailedVpc(vpcId string, err error, log logger.Logger) {
	log.Warn("failed to calculate nau for vpc: %s, %v", vpcId, err)
}

// logUnsupportedEniType logs an error for unsupported ENI types.
func logUnsupportedEniType(err error, log logger.Logger) {
	log.Error(err.Error())
}

// logNonAttachedEni logs an error for non-attached ENIs.
func logNonAttachedEni(err error, log logger.Logger) {
	log.Error(err.Error())
}

// copyMetadata creates a deep copy of ResourceMetadata for record creation.
func copyMetadata(meta ResourceMetadata) ResourceMetadata {
	return ResourceMetadata{
		NauResourceType:  meta.NauResourceType,
		NauWeight:        meta.NauWeight,
		Id:               meta.Id,
		ResourceType:     meta.ResourceType,
		VpcId:            meta.VpcId,
		Region:           meta.Region,
		AvailabilityZone: meta.AvailabilityZone,
		SubnetId:         meta.SubnetId,
		Ipv4Address:      meta.Ipv4Address,
		Ipv6Address:      meta.Ipv6Address,
		Ipv4Prefix:       meta.Ipv4Prefix,
		Ipv6Prefix:       meta.Ipv6Prefix,
	}
}

// toNetworkInterfaceType converts EC2 SDK interface types to internal constants.
// Logs unsupported types and returns them as-is for future compatibility.
func toNetworkInterfaceType(raw ec2Types.NetworkInterfaceType, log logger.Logger) NetworkInterfaceType {
	s := string(raw)
	switch s {
	case string(NetworkInterfaceTypeApiGatewayManaged),
		string(NetworkInterfaceTypeAwsCodestarConnectionsManaged),
		string(NetworkInterfaceTypeBranch),
		string(NetworkInterfaceTypeEc2InstanceConnect),
		string(NetworkInterfaceTypeEfa),
		string(NetworkInterfaceTypeEfaOnly),
		string(NetworkInterfaceTypeEfs),
		string(NetworkInterfaceTypeEvs),
		string(NetworkInterfaceTypeGatewayLoadBalancer),
		string(NetworkInterfaceTypeGatewayLoadBalancerEndpoint),
		string(NetworkInterfaceTypeGlobalAcceleratorManaged),
		string(NetworkInterfaceTypeInterface),
		string(NetworkInterfaceTypeIotRulesManaged),
		string(NetworkInterfaceTypeLambda),
		string(NetworkInterfaceTypeLoadBalancer),
		string(NetworkInterfaceTypeNatGateway),
		string(NetworkInterfaceTypeNetworkLoadBalancer),
		string(NetworkInterfaceTypeQuicksight),
		string(NetworkInterfaceTypeTransitGateway),
		string(NetworkInterfaceTypeTrunk),
		string(NetworkInterfaceTypeVpcEndpoint):
		return NetworkInterfaceType(s)
	default:
		// if AWS adds a brand-new interface-type you haven’t defined,
		// you’ll still see it as a string here
		logUnsupportedEniType(fmt.Errorf("%w, %s", errUnsupportedEni, s), log)
		return NetworkInterfaceType(s)
	}
}
