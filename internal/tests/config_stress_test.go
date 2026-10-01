/*
** Copyright (c) 2026 Oracle and/or its affiliates.
** The Universal Permissive License (UPL), Version 1.0
 */

package tests

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func retentionConfigForTest() ResourceRetentionConfig {
	limit := int64(256)
	return ResourceRetentionConfig{Workers: 4, TotalOperations: 1024, MaxOpenConnections: 4, Timeout: "10m", MemoryLimitMiB: &limit}
}

func TestResourceRetentionConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*ResourceRetentionConfig)
		want string
	}{
		{"valid", func(*ResourceRetentionConfig) {}, ""},
		{"zero workers", func(c *ResourceRetentionConfig) { c.Workers = 0 }, "workers"},
		{"negative workers", func(c *ResourceRetentionConfig) { c.Workers = -1 }, "workers"},
		{"zero operations", func(c *ResourceRetentionConfig) { c.TotalOperations = 0 }, "total_operations"},
		{"negative operations", func(c *ResourceRetentionConfig) { c.TotalOperations = -1 }, "total_operations"},
		{"too few operations", func(c *ResourceRetentionConfig) { c.TotalOperations = 3 }, "total_operations"},
		{"zero connections", func(c *ResourceRetentionConfig) { c.MaxOpenConnections = 0 }, "max_open_connections"},
		{"negative connections", func(c *ResourceRetentionConfig) { c.MaxOpenConnections = -1 }, "max_open_connections"},
		{"too many connections", func(c *ResourceRetentionConfig) { c.MaxOpenConnections = 5 }, "max_open_connections"},
		{"missing timeout", func(c *ResourceRetentionConfig) { c.Timeout = "" }, "timeout"},
		{"bad timeout", func(c *ResourceRetentionConfig) { c.Timeout = "ten minutes" }, "timeout"},
		{"zero timeout", func(c *ResourceRetentionConfig) { c.Timeout = "0s" }, "timeout"},
		{"negative timeout", func(c *ResourceRetentionConfig) { c.Timeout = "-1s" }, "timeout"},
		{"duration overflow", func(c *ResourceRetentionConfig) { c.Timeout = "999999999999999999h" }, "timeout"},
		{"omitted memory", func(c *ResourceRetentionConfig) { c.MemoryLimitMiB = nil }, ""},
		{"zero memory", func(c *ResourceRetentionConfig) { *c.MemoryLimitMiB = 0 }, "memory_limit_mib"},
		{"negative memory", func(c *ResourceRetentionConfig) { *c.MemoryLimitMiB = -1 }, "memory_limit_mib"},
		{"memory overflow", func(c *ResourceRetentionConfig) { *c.MemoryLimitMiB = math.MaxInt64/(1<<20) + 1 }, "memory_limit_mib"},
		{"maximum memory", func(c *ResourceRetentionConfig) { *c.MemoryLimitMiB = math.MaxInt64 / (1 << 20) }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := retentionConfigForTest()
			tc.edit(&config)
			err := config.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error mentioning %s", err, tc.want)
			}
		})
	}
}

func TestResourceRetentionJSONCompatibility(t *testing.T) {
	valid, err := json.Marshal(retentionConfigForTest())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(valid, &fields); err != nil {
		t.Fatal(err)
	}
	profiles := map[string]string{
		"legacy":          "",
		"empty stress":    `,"stress":{}`,
		"valid retention": `,"stress":{"resource_retention":` + string(valid) + `}`,
		"empty retention": `,"stress":{"resource_retention":{}}`,
	}
	for name, memory := range map[string]string{"null memory": "null", "zero memory": "0", "negative memory": "-1", "overflow memory": "8796093022208"} {
		var profile map[string]json.RawMessage
		if err := json.Unmarshal(valid, &profile); err != nil {
			t.Fatal(err)
		}
		profile["memory_limit_mib"] = json.RawMessage(memory)
		encoded, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		profiles[name] = `,"stress":{"resource_retention":` + string(encoded) + `}`
	}
	for field := range fields {
		var incomplete map[string]json.RawMessage
		if err := json.Unmarshal(valid, &incomplete); err != nil {
			t.Fatal(err)
		}
		delete(incomplete, field)
		encoded, err := json.Marshal(incomplete)
		if err != nil {
			t.Fatal(err)
		}
		profiles["missing "+field] = `,"stress":{"resource_retention":` + string(encoded) + `}`
	}
	for name, section := range profiles {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(`[{"config_name":"test","enabled":true`+section+`}]`), 0600); err != nil {
				t.Fatal(err)
			}
			env, err := NewTestingEnvironment(path)
			valid := name == "legacy" || name == "empty stress" || name == "valid retention" || name == "missing memory_limit_mib" || name == "null memory"
			if !valid {
				if err == nil || !strings.Contains(err.Error(), "stress.resource_retention") {
					t.Fatalf("expected contextual validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			config, err := env.GetConfig("test")
			if err != nil {
				t.Fatal(err)
			}
			if name == "valid retention" && !reflect.DeepEqual(*config.Stress.ResourceRetention, retentionConfigForTest()) {
				t.Fatal("profile was not preserved")
			}
			if (name == "missing memory_limit_mib" || name == "null memory") && config.Stress.ResourceRetention.MemoryLimitMiB != nil {
				t.Fatal("omitted memory target gained a default")
			}
			if name == "legacy" && config.Stress != nil {
				t.Fatal("legacy configuration gained stress defaults")
			}
		})
	}
}

func TestResourceRetentionConfigCloneAndMerge(t *testing.T) {
	profile := retentionConfigForTest()
	source := &TestConfig{Stress: &StressConfig{ResourceRetention: &profile}}
	clone := source.Clone()
	if !reflect.DeepEqual(*clone.Stress.ResourceRetention, profile) || clone.Stress == source.Stress || clone.Stress.ResourceRetention == source.Stress.ResourceRetention || clone.Stress.ResourceRetention.MemoryLimitMiB == source.Stress.ResourceRetention.MemoryLimitMiB {
		t.Fatal("clone must preserve values without aliasing either pointer")
	}
	clone.Stress.ResourceRetention.Workers = 2
	*clone.Stress.ResourceRetention.MemoryLimitMiB = 128
	if source.Stress.ResourceRetention.Workers != 4 {
		t.Fatal("clone changed source")
	}
	if *source.Stress.ResourceRetention.MemoryLimitMiB != 256 {
		t.Fatal("clone changed source memory target")
	}
	destination := &TestConfig{}
	destination.MergeWith(source)
	if !reflect.DeepEqual(*destination.Stress.ResourceRetention, profile) || destination.Stress == source.Stress || destination.Stress.ResourceRetention == source.Stress.ResourceRetention || destination.Stress.ResourceRetention.MemoryLimitMiB == source.Stress.ResourceRetention.MemoryLimitMiB {
		t.Fatal("merge must deep-copy profile")
	}
	source.Stress.ResourceRetention.TotalOperations = 10
	*source.Stress.ResourceRetention.MemoryLimitMiB = 512
	if destination.Stress.ResourceRetention.TotalOperations != 1024 {
		t.Fatal("merge aliases source")
	}
	if *destination.Stress.ResourceRetention.MemoryLimitMiB != 256 {
		t.Fatal("merge aliases source memory target")
	}
	destination.MergeWith(&TestConfig{})
	destination.MergeWith(&TestConfig{Stress: &StressConfig{}})
	if destination.Stress.ResourceRetention.TotalOperations != 1024 {
		t.Fatal("omitted profile must not clear destination")
	}
	if (&TestConfig{}).Clone().Stress != nil {
		t.Fatal("clone introduced defaults")
	}
	if (&TestConfig{Stress: &StressConfig{}}).Clone().Stress.ResourceRetention != nil {
		t.Fatal("empty stress gained retention profile")
	}
	baseline := retentionConfigForTest()
	baseline.MemoryLimitMiB = nil
	destination.MergeWith(&TestConfig{Stress: &StressConfig{ResourceRetention: &baseline}})
	if destination.Stress.ResourceRetention.MemoryLimitMiB != nil || destination.Clone().Stress.ResourceRetention.MemoryLimitMiB != nil {
		t.Fatal("baseline clone/merge gained a memory target")
	}
}

func TestConnectionLifecycleConfigValidation(t *testing.T) {
	cases := []struct {
		name   string
		config ConnectionLifecycleConfig
		want   string
	}{
		{"valid", ConnectionLifecycleConfig{Workers: 4, CyclesPerWorker: 64, Timeout: "10m"}, ""},
		{"zero workers", ConnectionLifecycleConfig{CyclesPerWorker: 64, Timeout: "10m"}, "workers"},
		{"zero cycles", ConnectionLifecycleConfig{Workers: 4, Timeout: "10m"}, "cycles_per_worker"},
		{"invalid timeout", ConnectionLifecycleConfig{Workers: 4, CyclesPerWorker: 64, Timeout: "ten minutes"}, "timeout"},
		{"zero timeout", ConnectionLifecycleConfig{Workers: 4, CyclesPerWorker: 64, Timeout: "0s"}, "timeout"},
		{"overflow", ConnectionLifecycleConfig{Workers: int(^uint(0) >> 1), CyclesPerWorker: 2, Timeout: "10m"}, "overflows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestConnectionLifecycleConfigJSONAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`[{"config_name":"test","enabled":true,"stress":{"connection_lifecycle":{"workers":4,"cycles_per_worker":64,"timeout":"10m"}}}]`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	env, err := NewTestingEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := env.GetConfig("test")
	if err != nil {
		t.Fatal(err)
	}
	profile := loaded.Stress.ConnectionLifecycle
	if profile.Workers != 4 || profile.CyclesPerWorker != 64 || profile.Timeout != "10m" {
		t.Fatalf("incorrect lifecycle profile: %+v", profile)
	}
	clone := loaded.Clone()
	if clone.Stress.ConnectionLifecycle == profile || !reflect.DeepEqual(*clone.Stress.ConnectionLifecycle, *profile) {
		t.Fatal("clone must deep-copy the lifecycle profile")
	}

	retention := retentionConfigForTest()
	destination := &TestConfig{Stress: &StressConfig{ResourceRetention: &retention}}
	destination.MergeWith(loaded)
	if destination.Stress.ResourceRetention == nil || destination.Stress.ConnectionLifecycle == nil {
		t.Fatal("merging lifecycle profile discarded the retention profile")
	}
	destination.Stress.ConnectionLifecycle.Workers = 2
	if profile.Workers != 4 {
		t.Fatal("merge aliases source lifecycle profile")
	}
	otherRetention := retentionConfigForTest()
	otherRetention.Workers = 2
	destination.MergeWith(&TestConfig{Stress: &StressConfig{ResourceRetention: &otherRetention}})
	if destination.Stress.ConnectionLifecycle.Workers != 2 || destination.Stress.ResourceRetention.Workers != 2 {
		t.Fatal("merging retention profile discarded or changed lifecycle profile")
	}

	invalid := []byte(`[{"config_name":"test","enabled":true,"stress":{"connection_lifecycle":{"workers":0,"cycles_per_worker":64,"timeout":"10m"}}}]`)
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTestingEnvironment(path); err == nil || !strings.Contains(err.Error(), "stress.connection_lifecycle") {
		t.Fatalf("got %v, want contextual lifecycle validation error", err)
	}
}
