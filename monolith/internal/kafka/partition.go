package kafka

import (
	"crypto/sha256"
	"fmt"
	"hash/crc32"

	"monolith/internal/models"
)

// PartitionStrategy defines how messages are routed to partitions
type PartitionStrategy interface {
	GetPartition(event *models.OutboxEvent, numPartitions int32) int32
}

// HashPartitionStrategy uses hash-based partitioning for consistent routing
type HashPartitionStrategy struct{}

func (h *HashPartitionStrategy) GetPartition(event *models.OutboxEvent, numPartitions int32) int32 {
	if numPartitions <= 0 {
		return 0
	}

	var key string
	if event.PartitionKey != nil && *event.PartitionKey != "" {
		key = *event.PartitionKey
	} else {
		// Fallback to aggregate ID for consistent partitioning
		key = event.AggregateID
	}

	// Use CRC32 hash for consistent partitioning (similar to Kafka's default)
	hash := crc32.ChecksumIEEE([]byte(key))
	return int32(hash) % numPartitions
}

// UserIDPartitionStrategy partitions based on user ID hash
type UserIDPartitionStrategy struct{}

func (u *UserIDPartitionStrategy) GetPartition(event *models.OutboxEvent, numPartitions int32) int32 {
	if numPartitions <= 0 {
		return 0
	}

	// Use SHA256 for better distribution
	hasher := sha256.New()
	hasher.Write([]byte(event.AggregateID))
	hash := hasher.Sum(nil)

	// Convert first 4 bytes to int32
	hashInt := int32(hash[0])<<24 | int32(hash[1])<<16 | int32(hash[2])<<8 | int32(hash[3])
	if hashInt < 0 {
		hashInt = -hashInt
	}

	return hashInt % numPartitions
}

// RoundRobinPartitionStrategy distributes messages evenly across partitions
type RoundRobinPartitionStrategy struct {
	counter int32
}

func (r *RoundRobinPartitionStrategy) GetPartition(event *models.OutboxEvent, numPartitions int32) int32 {
	if numPartitions <= 0 {
		return 0
	}

	partition := r.counter % numPartitions
	r.counter++
	return partition
}

// CustomPartitionStrategy allows custom partition logic based on event type
type CustomPartitionStrategy struct {
	strategies map[string]PartitionStrategy
	default_   PartitionStrategy
}

func NewCustomPartitionStrategy() *CustomPartitionStrategy {
	return &CustomPartitionStrategy{
		strategies: make(map[string]PartitionStrategy),
		default_:   &HashPartitionStrategy{},
	}
}

func (c *CustomPartitionStrategy) SetStrategyForEventType(eventType string, strategy PartitionStrategy) {
	c.strategies[eventType] = strategy
}

func (c *CustomPartitionStrategy) SetDefaultStrategy(strategy PartitionStrategy) {
	c.default_ = strategy
}

func (c *CustomPartitionStrategy) GetPartition(event *models.OutboxEvent, numPartitions int32) int32 {
	if strategy, exists := c.strategies[event.EventType]; exists {
		return strategy.GetPartition(event, numPartitions)
	}
	return c.default_.GetPartition(event, numPartitions)
}

// TenantPartitionStrategy partitions based on tenant information
// This assumes partition key contains tenant information
type TenantPartitionStrategy struct {
	tenantsPerPartition int32
}

func NewTenantPartitionStrategy(tenantsPerPartition int32) *TenantPartitionStrategy {
	return &TenantPartitionStrategy{
		tenantsPerPartition: tenantsPerPartition,
	}
}

func (t *TenantPartitionStrategy) GetPartition(event *models.OutboxEvent, numPartitions int32) int32 {
	if numPartitions <= 0 {
		return 0
	}

	var tenantID string
	if event.PartitionKey != nil && *event.PartitionKey != "" {
		tenantID = *event.PartitionKey
	} else {
		// Extract tenant from aggregate ID or use default
		tenantID = event.AggregateID
	}

	// Simple hash-based tenant to partition mapping
	hash := crc32.ChecksumIEEE([]byte(tenantID))
	return int32(hash) % numPartitions
}

// GetOptimalPartitionCount suggests partition count based on expected load
func GetOptimalPartitionCount(expectedEventsPerSecond int, targetEventsPerPartitionPerSecond int) int {
	if targetEventsPerPartitionPerSecond <= 0 {
		targetEventsPerPartitionPerSecond = 1000 // Default target
	}

	partitions := expectedEventsPerSecond / targetEventsPerPartitionPerSecond
	if partitions < 1 {
		partitions = 1
	}

	// Round up to next power of 2 for better distribution
	nextPowerOf2 := 1
	for nextPowerOf2 < partitions {
		nextPowerOf2 *= 2
	}

	return nextPowerOf2
}

// PartitionAnalyzer helps analyze partition distribution
type PartitionAnalyzer struct {
	partitionCounts map[int32]int
	strategy        PartitionStrategy
}

func NewPartitionAnalyzer(strategy PartitionStrategy) *PartitionAnalyzer {
	return &PartitionAnalyzer{
		partitionCounts: make(map[int32]int),
		strategy:        strategy,
	}
}

func (p *PartitionAnalyzer) AnalyzeEvent(event *models.OutboxEvent, numPartitions int32) {
	partition := p.strategy.GetPartition(event, numPartitions)
	p.partitionCounts[partition]++
}

func (p *PartitionAnalyzer) GetDistribution() map[int32]int {
	return p.partitionCounts
}

func (p *PartitionAnalyzer) GetDistributionStats() string {
	total := 0
	for _, count := range p.partitionCounts {
		total += count
	}

	if total == 0 {
		return "No events analyzed"
	}

	result := fmt.Sprintf("Total events: %d\nDistribution:\n", total)
	for partition, count := range p.partitionCounts {
		percentage := float64(count) / float64(total) * 100
		result += fmt.Sprintf("  Partition %d: %d events (%.2f%%)\n", partition, count, percentage)
	}

	return result
}

func (p *PartitionAnalyzer) Reset() {
	p.partitionCounts = make(map[int32]int)
}
