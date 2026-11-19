package session

import (
	"bytes"
	"context"
	"github.com/oracle/go-oracledb/v26/oracle/sharding"
	"strings"
	"testing"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
)

// TestShardingRedirectConnectData verifies address-only redirects retain SK/SSK
// connect data and server-provided replacement data remains authoritative.
func TestShardingRedirectConnectData(t *testing.T) {
	original := []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=global)(SHARDING_KEY_B64=1 0 1,YWJj)(SUPER_SHARDING_KEY_B64=1 0 1,SU5ESUE=)))")
	replacement := []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=shard-service)(SHARDING_KEY_B64=1 0 1,YWJj)))")
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "address-only", true: "server-connect-data"}[replace], func(t *testing.T) {
			mock := &mockNTAdapter{}
			ns := newNetworkSession()
			ns.ntAdapter = mock
			ns.sAtts = newSessionAtts("test")
			ns.sAtts.sdu = 8192
			ns.cData = original
			packet := &redirectPacket{hdr: &header{}, dataBuf: []byte("(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))")}
			want := original
			if replace {
				packet.hdr.flags = byte(NSPFRDS)
				packet.dataBuf = append(append(packet.dataBuf, 0), replacement...)
				want = replacement
			}
			err := ns.handleRedirect(context.Background(), packet, transport.Address{Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "127.0.0.1", Port: 1521}})
			if err != nil {
				t.Fatal(err)
			}
			sent := bytes.Join(mock.sentData, nil)
			if !bytes.Contains(sent, want) {
				t.Fatal("redirect lost expected connect data")
			}
			if replace && bytes.Contains(sent, original) {
				t.Fatal("redirect ignored server-provided data")
			}
		})
	}
}

// TestShardingConnectDataLengthLimit verifies oversized final connect data
// is rejected before dialing instead of overflowing the 16-bit packet length field.
func TestShardingConnectDataLengthLimit(t *testing.T) {
	option := &naming.ConnectionOption{
		Address:       naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "127.0.0.1", Port: 1},
		ConnectString: "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=test)(SHARDING_KEY_B64=" + strings.Repeat("A", 65535) + ")))",
	}
	_, err := ConnectToOption(context.Background(), option)
	if err == nil || !strings.Contains(err.Error(), "protocol limit") {
		t.Fatalf("expected length rejection before dial, got %v", err)
	}
}

// TestAppendShardingKeys verifies SK and optional SSK are encoded into descriptor
// nodes, delimiter-containing values are encoded safely, and duplicate routing is rejected.
func TestAppendShardingKeys(t *testing.T) {
	key, err := sharding.NewKey(sharding.Subkey{Type: sharding.VARCHAR, Value: "a)(b=c"})
	if err != nil {
		t.Fatal(err)
	}
	super, err := sharding.NewKey(sharding.Subkey{Type: sharding.VARCHAR, Value: "INDIA"})
	if err != nil {
		t.Fatal(err)
	}
	for _, withSSK := range []bool{false, true} {
		request := sharding.ShardingRequest{ShardingKey: key}
		if withSSK {
			request.SuperShardingKey = &super
		}
		ctx, err := sharding.WithSharding(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		data := &naming.Node{Name: "CONNECT_DATA", Children: []naming.Node{{Name: "SERVICE_NAME", Value: "global"}}}
		if err := appendShardingKeys(ctx, data); err != nil {
			t.Fatal(err)
		}
		got, err := data.GetValue("CONNECT_DATA/SHARDING_KEY_B64")
		if err != nil || got != "1 0 1,YSkoYj1j" {
			t.Fatalf("SK: %q %v", got, err)
		}
		got, err = data.GetValue("CONNECT_DATA/SUPER_SHARDING_KEY_B64")
		if withSSK && (err != nil || got != "1 0 1,SU5ESUE=") {
			t.Fatalf("SSK: %q %v", got, err)
		}
		if !withSSK && err == nil {
			t.Fatal("unexpected SSK")
		}
		if err := appendShardingKeys(ctx, data); err == nil {
			t.Fatal("duplicate routing accepted")
		}
	}
}

// TestAppendShardingKeysConflictsAndKeyless verifies descriptor-specified routing
// conflicts with context routing, while a plain context leaves connect data unchanged.
func TestAppendShardingKeysConflictsAndKeyless(t *testing.T) {
	key, err := sharding.NewKey(sharding.Subkey{Type: sharding.NUMBER, Value: 120})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := sharding.WithSharding(context.Background(), sharding.ShardingRequest{ShardingKey: key})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SHARDING_KEY", "SUPER_SHARDING_KEY", "SHARDING_KEY_B64", "super_sharding_key_b64"} {
		data := &naming.Node{Name: "CONNECT_DATA", Children: []naming.Node{{Name: name, Value: "existing"}}}
		before := data.ToString()
		if err := appendShardingKeys(context.Background(), data); err != nil || data.ToString() != before {
			t.Fatal("keyless request changed")
		}
		if err := appendShardingKeys(ctx, data); err == nil {
			t.Fatalf("accepted conflict %s", name)
		}
		if data.ToString() != before {
			t.Fatal("conflict modified node")
		}
	}
}

// TestAppendShardingKeysDefersEncoding verifies invalid SK or SSK values fail
// at descriptor encoding time without partially modifying CONNECT_DATA.
func TestAppendShardingKeysDefersEncoding(t *testing.T) {
	valid, err := sharding.NewKey(sharding.Subkey{Type: sharding.NUMBER, Value: 120})
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := sharding.NewKey(sharding.Subkey{Type: sharding.NUMBER, Value: "invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []sharding.ShardingRequest{
		{ShardingKey: invalid},
		{ShardingKey: valid, SuperShardingKey: &invalid},
	} {
		ctx, err := sharding.WithSharding(context.Background(), request)
		if err != nil {
			t.Fatal("context attempted encoding", err)
		}
		data := &naming.Node{Name: "CONNECT_DATA", Children: []naming.Node{{Name: "SERVICE_NAME", Value: "global"}}}
		if err := appendShardingKeys(ctx, data); err == nil {
			t.Fatal("invalid value encoded")
		}
		if len(data.Children) != 1 || data.Children[0].Name != "SERVICE_NAME" || data.Children[0].Value != "global" {
			t.Fatal("failed encoding modified descriptor")
		}
	}
}
