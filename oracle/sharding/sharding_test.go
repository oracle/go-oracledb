package sharding_test

import (
	"context"
	"testing"
	"time"

	internalSharding "github.com/oracle/go-oracledb/v26/internal/sharding"
	publicSharding "github.com/oracle/go-oracledb/v26/oracle/sharding"
)

func testShardKey(t *testing.T, value string) publicSharding.ShardingKey {
	t.Helper()
	k, e := publicSharding.NewKey(publicSharding.Subkey{Type: publicSharding.VARCHAR, Value: value})
	if e != nil {
		t.Fatal(e)
	}
	return k
}

// TestWithShardingContext verifies SK/SSK snapshots survive derived contexts,
// extraction cannot replace stored keys, and invalid requests are rejected.
func TestWithShardingContext(t *testing.T) {
	key := testShardKey(t, "customer")
	super := testShardKey(t, "INDIA")
	ctx, err := publicSharding.WithSharding(context.Background(), publicSharding.ShardingRequest{ShardingKey: key, SuperShardingKey: &super})
	if err != nil {
		t.Fatal(err)
	}
	super = testShardKey(t, "EUROPE") // changing caller's pointed-to value must not affect ctx
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, b := internalSharding.Keys[publicSharding.ShardingKey](ctx)
	if b.Subkeys()[0].Value != "INDIA" {
		t.Fatalf("SSK was not copied: %s", b)
	}
	// Internal extraction must not expose the context's stored SSK pointer.
	*b = testShardKey(t, "changed")
	_, encoded := internalSharding.Keys[publicSharding.ShardingKey](ctx)
	if encoded.Subkeys()[0].Value != "INDIA" {
		t.Fatal("extractor exposed mutable request")
	}
	if key, _ := internalSharding.Keys[publicSharding.ShardingKey](context.Background()); key != nil {
		t.Fatal("routing leaked")
	}
	if _, err := publicSharding.WithSharding(nil, publicSharding.ShardingRequest{ShardingKey: key}); err == nil {
		t.Fatal("nil ctx accepted")
	}
	if _, err := publicSharding.WithSharding(ctx, publicSharding.ShardingRequest{SuperShardingKey: &super}); err == nil {
		t.Fatal("SSK without SK accepted")
	}
	zero := publicSharding.ShardingKey{}
	if _, err := publicSharding.WithSharding(ctx, publicSharding.ShardingRequest{ShardingKey: key, SuperShardingKey: &zero}); err == nil {
		t.Fatal("invalid SSK accepted")
	}
}
