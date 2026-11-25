/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestConnectionAttemptIteratorPrioritizesHealthyDescriptionsAndTargets(t *testing.T) {
	const (
		downDescriptionFirstIP  = "192.0.2.11"
		downDescriptionSecondIP = "192.0.2.12"
		healthyDescriptionIP    = "192.0.2.13"
		downTargetIP            = "192.0.2.14"
	)
	markDownHostsForAttemptTest(t,
		downDescriptionFirstIP,
		downDescriptionSecondIP,
		downTargetIP,
	)

	plans := newAttemptTestPlans(
		newAttemptTestDescription(downDescriptionFirstIP, downDescriptionSecondIP),
		newAttemptTestDescription(healthyDescriptionIP, downTargetIP),
	)
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "ordering-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}

	var got []string
	for {
		attempt, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed: %v", nextErr)
		}
		got = append(got, attempt.GetHost())
	}

	want := []string{
		healthyDescriptionIP,
		downTargetIP,
		downDescriptionFirstIP,
		downDescriptionSecondIP,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attempt hosts = %v, want %v", got, want)
	}
}

func TestConnectionAttemptIteratorRotatesTargetsOnRetry(t *testing.T) {
	plans := newAttemptTestPlans(newAttemptTestDescription(
		"192.0.2.21", "192.0.2.22", "192.0.2.23",
	))
	plans.BaseConfig.ConnectionProperties.LoadBalance = true
	loadBalance := true
	retryCount := 1
	plans.Plan.Descriptions[0].LoadBalance = &loadBalance
	plans.Plan.Descriptions[0].RetryCount = &retryCount

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "retry-order-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}

	firstCycle := make([]string, 3)
	secondCycle := make([]string, 3)
	for i := range firstCycle {
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for first cycle target %d failed: %v", i, err)
		}
		firstCycle[i] = attempt.GetHost()
	}
	for i := range secondCycle {
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for retry target %d failed: %v", i, err)
		}
		secondCycle[i] = attempt.GetHost()
	}

	wantSecondCycle := []string{firstCycle[1], firstCycle[2], firstCycle[0]}
	if !reflect.DeepEqual(secondCycle, wantSecondCycle) {
		t.Fatalf("retry hosts = %v, want left rotation %v of first cycle %v", secondCycle, wantSecondCycle, firstCycle)
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after retry cycles = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorResolvesDescriptionConfigWhenReached(t *testing.T) {
	plans := newAttemptTestPlans(
		newAttemptTestDescription("192.0.2.31"),
		newAttemptTestDescription("192.0.2.32"),
	)
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "lazy-config-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	if len(iterator.descriptionCursors) != 2 {
		t.Fatalf("prepared description count = %d, want 2", len(iterator.descriptionCursors))
	}
	for i, cursor := range iterator.descriptionCursors {
		if cursor.effectiveConfig != nil {
			t.Fatalf("description %d effective config was built before it was reached", i)
		}
	}

	if _, err := iterator.Next(); err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if iterator.descriptionCursors[0].effectiveConfig == nil {
		t.Fatal("first description effective config was not built when reached")
	}
	if iterator.descriptionCursors[1].effectiveConfig != nil {
		t.Fatal("second description effective config was built before it was reached")
	}
}

func TestConnectionAttemptIteratorAppliesDescriptionFailover(t *testing.T) {
	t.Parallel()
	plans := newAttemptTestPlans(
		newAttemptTestDescription("192.0.2.51"),
		newAttemptTestDescription("192.0.2.52"),
	)
	failover := false
	plans.Plan.Failover = &failover

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "failover-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if attempt.GetHost() != "192.0.2.51" {
		t.Fatalf("first attempt host = %q, want 192.0.2.51", attempt.GetHost())
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after disabled failover = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorAppliesAddressGroupFailover(t *testing.T) {
	t.Parallel()
	groupFailover := false
	plans := newAttemptTestPlans(ConnectionDescription{
		AddressGroups: []ConnectionAddressGroup{
			{
				Failover: &groupFailover,
				AccessPoints: []RemoteAccessPoint{
					{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.121", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.122", Port: 1521},
				},
			},
			{
				AccessPoints: []RemoteAccessPoint{
					{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.123", Port: 1521},
				},
			},
		},
		ConnectData: &ConnectData{ServiceName: "group-failover-service"},
	})

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "group-failover-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	var got []string
	for {
		attempt, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed: %v", nextErr)
		}
		got = append(got, attempt.GetHost())
	}
	want := []string{"192.0.2.121", "192.0.2.123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group failover attempt order = %v, want %v", got, want)
	}
}

func TestConnectionAttemptIteratorKeepsConnectDataPerDescription(t *testing.T) {
	t.Parallel()
	first := newAttemptTestDescription("192.0.2.61")
	first.ConnectData.ServiceName = "service-a"
	second := newAttemptTestDescription("192.0.2.62")
	second.ConnectData.ServiceName = "service-b"
	plans := newAttemptTestPlans(first, second)

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "connect-data-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	for _, want := range []string{"service-a", "service-b"} {
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if got := attempt.GetConnectData().ServiceName; got != want {
			t.Errorf("attempt connect-data service = %q, want %q", got, want)
		}
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after all descriptions = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorSharesConnectDataAcrossTargets(t *testing.T) {
	t.Parallel()
	description := newAttemptTestDescription("192.0.2.71", "192.0.2.72")
	description.ConnectData.ServiceName = "shared-service"
	plans := newAttemptTestPlans(description)

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "shared-connect-data-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	for _, wantHost := range []string{"192.0.2.71", "192.0.2.72"} {
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if got := attempt.GetHost(); got != wantHost {
			t.Errorf("attempt host = %q, want %q", got, wantHost)
		}
		if got := attempt.GetConnectData().ServiceName; got != "shared-service" {
			t.Errorf("service name = %q, want shared-service", got)
		}
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after all targets = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorRetryDelayHonorsContext(t *testing.T) {
	t.Parallel()
	plans := newAttemptTestPlans(newAttemptTestDescription("192.0.2.91"))
	retryCount := 1
	retryDelay := time.Second
	plans.Plan.Descriptions[0].RetryCount = &retryCount
	plans.Plan.Descriptions[0].RetryDelay = &retryDelay

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	iterator, err := plans.NewConnectionAttemptIterator(ctx, "retry-cancel-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	if _, err := iterator.Next(); err != nil {
		t.Fatalf("first iterator.Next failed: %v", err)
	}
	if _, err := iterator.Next(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("iterator.Next during retry delay = %v, want context deadline exceeded", err)
	}
}

func TestConnectionAttemptIteratorRetryCountRepeatsTargets(t *testing.T) {
	t.Parallel()
	description := newAttemptTestDescription("192.0.2.101", "192.0.2.102")
	retryCount := 2
	description.RetryCount = &retryCount
	plans := newAttemptTestPlans(description)

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "retry-count-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	var got []string
	for {
		attempt, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed: %v", nextErr)
		}
		got = append(got, attempt.GetHost())
	}
	want := []string{
		"192.0.2.101", "192.0.2.102",
		"192.0.2.101", "192.0.2.102",
		"192.0.2.101", "192.0.2.102",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retry attempt order = %v, want %v", got, want)
	}
}

func TestConnectionAttemptIteratorHonorsZeroRetryCount(t *testing.T) {
	t.Parallel()
	description := newAttemptTestDescription("192.0.2.103")
	retryCount := 0
	description.RetryCount = &retryCount
	plans := newAttemptTestPlans(description)
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "zero-retry-count-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	if _, err := iterator.Next(); err != nil {
		t.Fatalf("first iterator.Next failed: %v", err)
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after zero retries = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorReordersDownHostsOnRetry(t *testing.T) {
	t.Parallel()
	const (
		downHost    = "192.0.2.111"
		healthyHost = "192.0.2.112"
	)
	plans := newAttemptTestPlans(newAttemptTestDescription(downHost, healthyHost))
	retryCount := 1
	plans.Plan.Descriptions[0].RetryCount = &retryCount
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "down-host-retry-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	for _, wantHost := range []string{downHost, healthyHost} {
		attempt, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("initial iterator.Next failed: %v", nextErr)
		}
		if attempt.GetHost() != wantHost {
			t.Fatalf("initial host = %q, want %q", attempt.GetHost(), wantHost)
		}
	}
	driverCommon.MarkDownHost(downHost)
	t.Cleanup(func() { driverCommon.RemoveDownHost(downHost) })

	for _, wantHost := range []string{healthyHost, downHost} {
		attempt, nextErr := iterator.Next()
		if nextErr != nil {
			t.Fatalf("retry iterator.Next failed: %v", nextErr)
		}
		if attempt.GetHost() != wantHost {
			t.Fatalf("retry host = %q, want %q", attempt.GetHost(), wantHost)
		}
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after retries = %v, want io.EOF", err)
	}
}

func TestConnectionAttemptIteratorHandlesManyTargets(t *testing.T) {
	t.Parallel()
	const targetCount = 100
	hosts := make([]string, targetCount)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("192.0.2.%d", i+1)
	}
	plans := newAttemptTestPlans(newAttemptTestDescription(hosts...))
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "many-targets-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	count := 0
	for {
		_, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed after %d attempts: %v", count, nextErr)
		}
		count++
	}
	if count != targetCount {
		t.Fatalf("yielded attempts = %d, want %d", count, targetCount)
	}

	const descriptionCount = 50
	descriptions := make([]ConnectionDescription, descriptionCount)
	for i := range descriptions {
		descriptions[i] = newAttemptTestDescription(fmt.Sprintf("198.51.100.%d", i+1))
	}
	plans = newAttemptTestPlans(descriptions...)
	iterator, err = plans.NewConnectionAttemptIterator(context.Background(), "many-descriptions-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator for many descriptions failed: %v", err)
	}
	count = 0
	for {
		_, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed after %d description attempts: %v", count, nextErr)
		}
		count++
	}
	if count != descriptionCount {
		t.Fatalf("yielded description attempts = %d, want %d", count, descriptionCount)
	}

	const retryCount = 999
	description := newAttemptTestDescription("203.0.113.111")
	description.RetryCount = intPointer(retryCount)
	plans = newAttemptTestPlans(description)
	iterator, err = plans.NewConnectionAttemptIterator(context.Background(), "large-retry-count-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator for repeated attempts failed: %v", err)
	}
	count = 0
	for {
		_, nextErr := iterator.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("iterator.Next failed after %d retry attempts: %v", count, nextErr)
		}
		count++
	}
	if count != retryCount+1 {
		t.Fatalf("yielded attempts with %d retries = %d, want %d", retryCount, count, retryCount+1)
	}
}

func intPointer(value int) *int { return &value }

func TestConnectionAttemptIteratorNormalizesNegativeRetryCount(t *testing.T) {
	t.Parallel()
	description := newAttemptTestDescription("192.0.2.131")
	retryCount := -1
	description.RetryCount = &retryCount
	plans := newAttemptTestPlans(description)
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "negative-retry-count-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed before validating selected DESCRIPTION: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if got := attempt.GetRetryCount(); got != 0 {
		t.Fatalf("attempt retry count = %d, want 0", got)
	}
	if _, err := iterator.Next(); err != io.EOF {
		t.Fatalf("iterator.Next after one attempt = %v, want io.EOF", err)
	}
}

func newAttemptTestPlans(descriptions ...ConnectionDescription) *ConnectionPlans {
	failover := true
	loadBalance := false
	return &ConnectionPlans{
		BaseConfig: func() *OracleDriverConfig {
			config := NewOracleDriverConfig()
			config.ConnectionProperties.Failover = true
			config.ConnectionProperties.LoadBalance = false
			return config
		}(),
		Plan: ConnectionDescriptionPlan{
			Failover:     &failover,
			LoadBalance:  &loadBalance,
			Descriptions: descriptions,
		},
	}
}

func newAttemptTestDescription(hosts ...string) ConnectionDescription {
	accessPoints := make([]RemoteAccessPoint, len(hosts))
	for i, host := range hosts {
		accessPoints[i] = RemoteAccessPoint{
			Protocol: driverCommon.ProtocolTCP,
			Host:     host,
			Port:     1521,
		}
	}
	return ConnectionDescription{
		AddressGroups: []ConnectionAddressGroup{{AccessPoints: accessPoints}},
		ConnectData:   &ConnectData{ServiceName: "test-service"},
	}
}

func markDownHostsForAttemptTest(t *testing.T, hosts ...string) {
	t.Helper()
	for _, host := range hosts {
		driverCommon.MarkDownHost(host)
	}
	t.Cleanup(func() {
		for _, host := range hosts {
			driverCommon.RemoveDownHost(host)
		}
	})
}
