/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package driver

import (
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

type factoryTestNetworkSession struct{}

func (factoryTestNetworkSession) WriteByteWithContext(context.Context, byte) error { return nil }

func (factoryTestNetworkSession) WriteBytesWithContext(context.Context, []byte) error { return nil }

func (factoryTestNetworkSession) ReadByteWithContext(context.Context) (byte, error) { return 0, nil }

func (factoryTestNetworkSession) ReadBytesWithContext(_ context.Context, n int32) (*[]byte, error) {
	buf := make([]byte, int(n))
	return &buf, nil
}

func (factoryTestNetworkSession) Flush(context.Context) error { return nil }

func (factoryTestNetworkSession) Disconnect(context.Context, int) error { return nil }

func (factoryTestNetworkSession) CancelOperation(context.Context) error { return nil }

func (factoryTestNetworkSession) CheckInbandNotification() bool { return false }

func (factoryTestNetworkSession) GetRemoteAddress() string { return "" }

func (factoryTestNetworkSession) GetRemotePort() int { return 0 }

func TestGetConnectionInstantiator(t *testing.T) {
	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "localhost:1521/service"
	cfg.Credentials.User = "scott"
	cfg.Credentials.Password = "tiger"

	var ns driverCommon.NetworkSession = factoryTestNetworkSession{}
	instantiator, err := GetConnectionInstantiator(cfg, ns, common.NewProviderRegistry())
	if err != nil {
		t.Fatalf("GetConnectionInstantiator returned error: %v", err)
	}
	if instantiator == nil {
		t.Fatal("GetConnectionInstantiator returned nil")
	}
}
