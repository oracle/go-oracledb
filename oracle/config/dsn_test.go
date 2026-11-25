/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package config

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestHandleDsnBuildsConnectionPlans verifies that EZConnect and TNS inputs
// become normalized plans with DSN-wide settings merged into an owned base
// configuration and target-scoped settings resolved by the iterator.
func TestHandleDsnBuildsConnectionPlans(t *testing.T) {
	t.Parallel()

	t.Run("EZConnect with base config and query settings", func(t *testing.T) {
		t.Parallel()
		baseConfig := NewOracleDriverConfig()
		baseConfig.ConnectionProperties.SendBufSize = 1024
		baseConfig.ConnectionProperties.ConnectTimeout = 2
		baseConfig.ConnectionProperties.SSLServerDNMatch = true

		plans, err := HandleDsn("tcps://db-ez.example.com:2484/orders.example.com?connect_timeout=9&send_buf_size=4096&ssl_server_dn_match=false", baseConfig)
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		if plans.BaseConfig != baseConfig {
			t.Fatal("HandleDsn should use the caller-owned working configuration")
		}
		if baseConfig.ConnectionProperties.SendBufSize != 4096 || baseConfig.ConnectionProperties.ConnectTimeout != 9 || baseConfig.ConnectionProperties.SSLServerDNMatch {
			t.Fatalf("DSN query settings were not merged into the working configuration: %+v", baseConfig.ConnectionProperties)
		}
		if len(plans.Plan.Descriptions) != 1 {
			t.Fatalf("description count = %d, want 1", len(plans.Plan.Descriptions))
		}
		description := plans.Plan.Descriptions[0]
		if len(description.AddressGroups) != 1 || len(description.AddressGroups[0].AccessPoints) != 1 {
			t.Fatalf("expected one normalized address group and access point, got %+v", description.AddressGroups)
		}
		accessPoint := description.AddressGroups[0].AccessPoints[0]
		if accessPoint.Protocol != driverCommon.ProtocolTCPS || accessPoint.Host != "db-ez.example.com" || accessPoint.Port != 2484 {
			t.Fatalf("unexpected EZConnect access point: %+v", accessPoint)
		}
		if description.ConnectData == nil || description.ConnectData.ServiceName != "orders.example.com" {
			t.Fatalf("unexpected EZConnect connect data: %+v", description.ConnectData)
		}

		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "ez-test-id")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if attempt.GetHost() != "db-ez.example.com" || attempt.GetPort() != 2484 || attempt.GetProtocol() != driverCommon.ProtocolTCPS {
			t.Fatalf("unexpected EZConnect attempt target: %s://%s:%d", attempt.GetProtocol(), attempt.GetHost(), attempt.GetPort())
		}
		if attempt.GetConnectTimeout() != 9*time.Second {
			t.Fatalf("connect timeout = %v, want 9s", attempt.GetConnectTimeout())
		}
		if attempt.GetSendBufferSize() != 4096 {
			t.Fatalf("send buffer size = %d, want 4096", attempt.GetSendBufferSize())
		}
		if attempt.GetSecurity().SSLServerDNMatch {
			t.Fatal("query setting ssl_server_dn_match=false should override the base configuration")
		}
		if attempt.GetConnectionID() != "ez-test-id" {
			t.Fatalf("connection ID = %q, want ez-test-id", attempt.GetConnectionID())
		}
		if got := attempt.GetEffectiveConfig().ConnectionProperties.SendBufSize; got != 4096 {
			t.Fatalf("effective config send buffer size = %d, want 4096", got)
		}
	})

	t.Run("TNS DESCRIPTION_LIST with base config and query settings", func(t *testing.T) {
		t.Parallel()
		baseConfig := NewOracleDriverConfig()
		baseConfig.ConnectionProperties.SendBufSize = 1024
		baseConfig.ConnectionProperties.ConnectTimeout = 2
		baseConfig.ConnectionProperties.SSLServerDNMatch = true
		baseConfig.ConnectionProperties.Failover = false
		baseConfig.ConnectionProperties.LoadBalance = true

		dsn := `(DESCRIPTION_LIST=(FAILOVER=on)(LOAD_BALANCE=off)` +
			`(DESCRIPTION=(CONNECT_TIMEOUT=5)(RETRY_COUNT=2)(SEND_BUF_SIZE=8192)` +
			`(ADDRESS=(PROTOCOL=tcps)(HOST=db-primary.example.com)(PORT=2484))` +
			`(CONNECT_DATA=(SERVICE_NAME=orders.example.com))` +
			`(SECURITY=(SSL_SERVER_DN_MATCH=no)(SSL_SERVER_CERT_DN="CN=orders-primary, OU=DB")))` +
			`(DESCRIPTION=(CONNECT_TIMEOUT=30)(RETRY_COUNT=0)` +
			`(ADDRESS=(PROTOCOL=tcps)(HOST=db-dr.example.com)(PORT=2484))` +
			`(CONNECT_DATA=(SERVICE_NAME=orders.example.com))` +
			`(SECURITY=(SSL_SERVER_CERT_DN="CN=orders-dr, OU=DB"))))` +
			`?send_buf_size=4096&recv_timeout=500ms`
		plans, err := HandleDsn(dsn, baseConfig)
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		if plans.BaseConfig != baseConfig {
			t.Fatal("HandleDsn should use the caller-owned working configuration")
		}
		if baseConfig.ConnectionProperties.SendBufSize != 4096 || baseConfig.ConnectionProperties.ConnectTimeout != 2 || !baseConfig.ConnectionProperties.SSLServerDNMatch || baseConfig.ConnectionProperties.Failover || !baseConfig.ConnectionProperties.LoadBalance || baseConfig.ConnectionProperties.ReceiveTimeout != "500ms" {
			t.Fatalf("DSN-wide settings were not merged without changing descriptor-scoped policy: %+v", baseConfig.ConnectionProperties)
		}
		if plans.Plan.Failover == nil || !*plans.Plan.Failover {
			t.Fatalf("plan failover = %v, want true", plans.Plan.Failover)
		}
		if plans.Plan.LoadBalance == nil || *plans.Plan.LoadBalance {
			t.Fatalf("plan load balance = %v, want false", plans.Plan.LoadBalance)
		}
		if len(plans.Plan.Descriptions) != 2 {
			t.Fatalf("description count = %d, want 2", len(plans.Plan.Descriptions))
		}
		if plans.BaseConfig.ConnectionProperties.SendBufSize != 4096 || plans.BaseConfig.ConnectionProperties.ReceiveTimeout != "500ms" {
			t.Fatalf("DSN query settings were not merged into base config: %+v", plans.BaseConfig.ConnectionProperties)
		}
		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "tns-test-id")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		primary, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for primary failed: %v", err)
		}
		if primary.GetHost() != "db-primary.example.com" || primary.GetConnectTimeout() != 5*time.Second || primary.GetRetryCount() != 2 {
			t.Fatalf("unexpected primary attempt: host=%q timeout=%v retryCount=%d", primary.GetHost(), primary.GetConnectTimeout(), primary.GetRetryCount())
		}
		if primary.GetSendBufferSize() != 8192 {
			t.Fatalf("primary send buffer size = %d, want DESCRIPTION override 8192", primary.GetSendBufferSize())
		}
		if primary.GetReceiveTimeout() != 500*time.Millisecond {
			t.Fatalf("primary receive timeout = %v, want 500ms", primary.GetReceiveTimeout())
		}
		if security := primary.GetSecurity(); security.SSLServerDNMatch || security.SSLServerCertDN != "CN=orders-primary, OU=DB" {
			t.Fatalf("unexpected primary security settings: %+v", security)
		}
		if data := primary.GetConnectData(); data.ServiceName != "orders.example.com" {
			t.Fatalf("primary service name = %q, want orders.example.com", data.ServiceName)
		}
		if primary.GetConnectionID() != "tns-test-id" {
			t.Fatalf("primary connection ID = %q, want tns-test-id", primary.GetConnectionID())
		}

		// RETRY_COUNT=2 yields two more attempts against the primary target
		// before failover advances the iterator to the second DESCRIPTION.
		for retry := 0; retry < 2; retry++ {
			retryAttempt, err := iterator.Next()
			if err != nil {
				t.Fatalf("iterator.Next for primary retry %d failed: %v", retry+1, err)
			}
			if retryAttempt.GetHost() != "db-primary.example.com" {
				t.Fatalf("primary retry %d host = %q, want db-primary.example.com", retry+1, retryAttempt.GetHost())
			}
		}

		disasterRecovery, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next for disaster-recovery target failed: %v", err)
		}
		if disasterRecovery.GetHost() != "db-dr.example.com" || disasterRecovery.GetConnectTimeout() != 30*time.Second || disasterRecovery.GetRetryCount() != 0 {
			t.Fatalf("unexpected disaster-recovery attempt: host=%q timeout=%v retryCount=%d", disasterRecovery.GetHost(), disasterRecovery.GetConnectTimeout(), disasterRecovery.GetRetryCount())
		}
		if disasterRecovery.GetSendBufferSize() != 4096 {
			t.Fatalf("disaster-recovery send buffer size = %d, want query setting 4096", disasterRecovery.GetSendBufferSize())
		}
		if security := disasterRecovery.GetSecurity(); !security.SSLServerDNMatch || security.SSLServerCertDN != "CN=orders-dr, OU=DB" {
			t.Fatalf("unexpected disaster-recovery security settings: %+v", security)
		}
	})
}

// TestHandleDsnNormalizesEZConnectTargets verifies EZConnect inputs through the
// public DSN planning path, including endpoint grouping and CONNECT_DATA fields.
func TestHandleDsnNormalizesEZConnectTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		dsn               string
		protocol          driverCommon.Protocol
		groups            [][]RemoteAccessPoint
		serviceName       string
		server            string
		instanceName      string
		wantLoadBalancing bool
	}{
		{
			name:        "explicit port and service",
			dsn:         "localhost:1521/mydb",
			protocol:    driverCommon.ProtocolTCP,
			groups:      [][]RemoteAccessPoint{{{Host: "localhost", Port: 1521}}},
			serviceName: "mydb",
		},
		{
			name:        "default port",
			dsn:         "myhost/orcl",
			protocol:    driverCommon.ProtocolTCP,
			groups:      [][]RemoteAccessPoint{{{Host: "myhost", Port: 1521}}},
			serviceName: "orcl",
		},
		{
			name:     "host without service",
			dsn:      "myhost",
			protocol: driverCommon.ProtocolTCP,
			groups:   [][]RemoteAccessPoint{{{Host: "myhost", Port: 1521}}},
		},
		{
			name:              "multiple hosts enable load balancing",
			dsn:               "host1:1521,host2:1522/mydb",
			protocol:          driverCommon.ProtocolTCP,
			groups:            [][]RemoteAccessPoint{{{Host: "host1", Port: 1521}, {Host: "host2", Port: 1522}}},
			serviceName:       "mydb",
			wantLoadBalancing: true,
		},
		{
			name:         "TCPS groups IPv6 and connect data",
			dsn:          "tcps://host1:2484,[2001:db8::1]:2485;host3:2486/orders:dedicated/inst1",
			protocol:     driverCommon.ProtocolTCPS,
			groups:       [][]RemoteAccessPoint{{{Host: "host1", Port: 2484}, {Host: "2001:db8::1", Port: 2485}}, {{Host: "host3", Port: 2486}}},
			serviceName:  "orders",
			server:       "DEDICATED",
			instanceName: "inst1",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plans, err := HandleDsn(test.dsn, NewOracleDriverConfig())
			if err != nil {
				t.Fatalf("HandleDsn failed: %v", err)
			}
			if len(plans.Plan.Descriptions) != 1 {
				t.Fatalf("description count = %d, want 1", len(plans.Plan.Descriptions))
			}
			description := plans.Plan.Descriptions[0]
			if len(description.AddressGroups) != len(test.groups) {
				t.Fatalf("address group count = %d, want %d", len(description.AddressGroups), len(test.groups))
			}
			for groupIndex, wantGroup := range test.groups {
				gotGroup := description.AddressGroups[groupIndex]
				if len(gotGroup.AccessPoints) != len(wantGroup) {
					t.Fatalf("group %d access point count = %d, want %d", groupIndex, len(gotGroup.AccessPoints), len(wantGroup))
				}
				for accessPointIndex, want := range wantGroup {
					got := gotGroup.AccessPoints[accessPointIndex]
					if got.Host != want.Host || got.Port != want.Port || got.Protocol != test.protocol {
						t.Errorf("group %d access point %d = %+v, want host=%q port=%d protocol=%s", groupIndex, accessPointIndex, got, want.Host, want.Port, test.protocol)
					}
				}
			}
			if got := description.ConnectData; test.serviceName == "" {
				if got != nil {
					t.Fatalf("connect data = %+v, want nil", got)
				}
			} else if got == nil || got.ServiceName != test.serviceName || got.Server != test.server || got.InstanceName != test.instanceName {
				t.Fatalf("connect data = %+v, want service=%q server=%q instance=%q", got, test.serviceName, test.server, test.instanceName)
			}
			if got := description.LoadBalance != nil && *description.LoadBalance; got != test.wantLoadBalancing {
				t.Errorf("load balancing = %t, want %t", got, test.wantLoadBalancing)
			}
		})
	}
}

// TestHandleDsnParsesTNSDescriptionFields verifies that TNS settings from each
// descriptor scope are retained in the normalized plan, including settings
// that were previously covered only by naming's legacy context extractor.
func TestHandleDsnParsesTNSDescriptionFields(t *testing.T) {
	t.Parallel()
	dsn := `(DESCRIPTION_LIST=(FAILOVER=on)(LOAD_BALANCE=off)(SOURCE_ROUTE=yes)(CUSTOM_LIST=root-value)` +
		`(DESCRIPTION=(FAILOVER=off)(LOAD_BALANCE=on)(SOURCE_ROUTE=no)(USE_SNI=yes)` +
		`(CONNECT_TIMEOUT=15)(RETRY_COUNT=0)(RETRY_DELAY=1sec)(TRANSPORT_CONNECT_TIMEOUT=500ms)` +
		`(EXPIRE_TIME=10)(RECV_TIMEOUT=30sec)(SEND_TIMEOUT=250ms)(SDU=8192)` +
		`(RECV_BUF_SIZE=1024)(SEND_BUF_SIZE=2048)(ENABLE=broken)` +
		`(CONNECTION_ID_PREFIX=description-prefix)(COMPRESSION=on)` +
		`(COMPRESSION_LEVELS=(LEVEL=low)(LEVEL=high))(TYPE_OF_SERVICE=oracle)` +
		`(CUSTOM_DESCRIPTION=description-value)` +
		`(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.51)(PORT=1521)(CUSTOM_ADDRESS=address-value))` +
		`(ADDRESS_LIST=(LOAD_BALANCE=yes)(FAILOVER=off)(SOURCE_ROUTE=yes)(CUSTOM_GROUP=group-value)` +
		`(ADDRESS=(PROTOCOL=TCPS)(HOST=192.0.2.52)(PORT=2484)` +
		`(HTTPS_PROXY=proxy.example.com)(HTTPS_PROXY_PORT=8080)(RECV_BUF_SIZE=4096)(SEND_BUF_SIZE=8192)))` +
		`(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.53)(PORT=1522))` +
		`(SECURITY=(SSL_SERVER_CERT_DN="CN=orders, OU=DB")(SSL_SERVER_DN_MATCH=yes)` +
		`(SSL_ALLOW_WEAK_DN_MATCH=no)(WALLET_LOCATION=/wallet)(MY_WALLET_DIRECTORY=/legacy-wallet)` +
		`(CUSTOM_SECURITY=security-value))` +
		`(CONNECT_DATA=(SERVICE_NAME=orders)(SID=ORD)(SERVER=DEDICATED)(INSTANCE_NAME=orders1)` +
		`(FAILOVER_MODE=(TYPE=select)(METHOD=basic)(RETRIES=3)(DELAY=5))` +
		`(HS=ok)(RDB_DATABASE=remote-db)(GLOBAL_NAME=orders.global)` +
		`(CONNECTION_ID_PREFIX=data-prefix)(CUSTOM_DATA=data-value)))` +
		`(CUSTOM_LIST_END=list-end-value))`

	plans, err := HandleDsn(dsn, NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if plans.Plan.Failover == nil || !*plans.Plan.Failover || plans.Plan.LoadBalance == nil || *plans.Plan.LoadBalance || plans.Plan.SourceRoute == nil || !*plans.Plan.SourceRoute {
		t.Fatalf("unexpected DESCRIPTION_LIST policy: failover=%v loadBalance=%v sourceRoute=%v", plans.Plan.Failover, plans.Plan.LoadBalance, plans.Plan.SourceRoute)
	}
	if plans.Plan.AdditionalSettings["CUSTOM_LIST"] != "root-value" || plans.Plan.AdditionalSettings["CUSTOM_LIST_END"] != "list-end-value" {
		t.Fatalf("DESCRIPTION_LIST additional settings = %v", plans.Plan.AdditionalSettings)
	}
	if len(plans.Plan.Descriptions) != 1 {
		t.Fatalf("description count = %d, want 1", len(plans.Plan.Descriptions))
	}
	description := plans.Plan.Descriptions[0]
	assertBoolPointer := func(name string, value *bool, want bool) {
		t.Helper()
		if value == nil || *value != want {
			t.Errorf("%s = %v, want %t", name, value, want)
		}
	}
	assertIntPointer := func(name string, value *int, want int) {
		t.Helper()
		if value == nil || *value != want {
			t.Errorf("%s = %v, want %d", name, value, want)
		}
	}
	assertDurationPointer := func(name string, value *time.Duration, want time.Duration) {
		t.Helper()
		if value == nil || *value != want {
			t.Errorf("%s = %v, want %v", name, value, want)
		}
	}
	assertBoolPointer("description failover", description.Failover, false)
	assertBoolPointer("description load balance", description.LoadBalance, true)
	assertBoolPointer("description source route", description.SourceRoute, false)
	assertBoolPointer("description use SNI", description.UseSNI, true)
	assertBoolPointer("description compression", description.Compression, true)
	assertDurationPointer("connect timeout", description.ConnectTimeout, 15*time.Second)
	assertIntPointer("retry count", description.RetryCount, 0)
	assertDurationPointer("retry delay", description.RetryDelay, time.Second)
	assertDurationPointer("transport connect timeout", description.TransportConnectTimeout, 500*time.Millisecond)
	assertIntPointer("expire time", description.ExpireTime, 10)
	assertDurationPointer("receive timeout", description.RecvTimeout, 30*time.Second)
	assertDurationPointer("send timeout", description.SendTimeout, 250*time.Millisecond)
	assertIntPointer("SDU", description.SDU, 8192)
	assertIntPointer("receive buffer size", description.RecvBufSize, 1024)
	assertIntPointer("send buffer size", description.SendBufSize, 2048)
	if description.Enable == nil || *description.Enable != "broken" {
		t.Errorf("ENABLE = %v, want broken", description.Enable)
	}
	if description.ConnectionIDPrefix == nil || *description.ConnectionIDPrefix != "description-prefix" {
		t.Errorf("description connection ID prefix = %v, want description-prefix", description.ConnectionIDPrefix)
	}
	if !reflect.DeepEqual(description.CompressionLevels, []string{"low", "high"}) {
		t.Errorf("compression levels = %v, want [low high]", description.CompressionLevels)
	}
	if description.TypeOfService == nil || *description.TypeOfService != "oracle" {
		t.Errorf("type of service = %v, want oracle", description.TypeOfService)
	}
	if description.AdditionalSettings["CUSTOM_DESCRIPTION"] != "description-value" {
		t.Errorf("description additional settings = %v", description.AdditionalSettings)
	}
	if len(description.AddressGroups) != 3 {
		t.Fatalf("address group count = %d, want 3", len(description.AddressGroups))
	}
	if got := description.AddressGroups[0].AccessPoints[0]; got.Host != "192.0.2.51" || got.Port != 1521 || got.Protocol != driverCommon.ProtocolTCP || got.AdditionalSettings["CUSTOM_ADDRESS"] != "address-value" {
		t.Errorf("first direct ADDRESS = %+v", got)
	}
	group := description.AddressGroups[1]
	assertBoolPointer("address-list load balance", group.LoadBalance, true)
	assertBoolPointer("address-list failover", group.Failover, false)
	assertBoolPointer("address-list source route", group.SourceRoute, true)
	if group.AdditionalSettings["CUSTOM_GROUP"] != "group-value" {
		t.Errorf("address-list additional settings = %v", group.AdditionalSettings)
	}
	if len(group.AccessPoints) != 1 {
		t.Fatalf("address-list access point count = %d, want 1", len(group.AccessPoints))
	}
	accessPoint := group.AccessPoints[0]
	if accessPoint.Protocol != driverCommon.ProtocolTCPS || accessPoint.Host != "192.0.2.52" || accessPoint.Port != 2484 {
		t.Errorf("address-list endpoint = %+v", accessPoint)
	}
	if accessPoint.HTTPSProxy == nil || *accessPoint.HTTPSProxy != "proxy.example.com" || accessPoint.HTTPSProxyPort == nil || *accessPoint.HTTPSProxyPort != 8080 {
		t.Errorf("address-list proxy = %v:%v", accessPoint.HTTPSProxy, accessPoint.HTTPSProxyPort)
	}
	assertIntPointer("address receive buffer size", accessPoint.RecvBufSize, 4096)
	assertIntPointer("address send buffer size", accessPoint.SendBufSize, 8192)
	if description.Security == nil {
		t.Fatal("security is nil")
	}
	if description.Security.SSLServerCertDN != "CN=orders, OU=DB" || !description.Security.SSLServerDNMatch || description.Security.SSLAllowWeakDNMatch || description.Security.WalletLocation != "/wallet" || description.Security.MyWalletDirectory != "/legacy-wallet" {
		t.Errorf("security settings = %+v", description.Security)
	}
	if description.Security.AdditionalSettings["CUSTOM_SECURITY"] != "security-value" {
		t.Errorf("security additional settings = %v", description.Security.AdditionalSettings)
	}
	if description.ConnectData == nil {
		t.Fatal("connect data is nil")
	}
	connectData := description.ConnectData
	if connectData.ServiceName != "orders" || connectData.SID != "ORD" || connectData.Server != "DEDICATED" || connectData.InstanceName != "orders1" || connectData.HS != "ok" || connectData.RDBDatabase != "remote-db" || connectData.GlobalName != "orders.global" || connectData.ConnectionIDPrefix != "data-prefix" {
		t.Errorf("connect data = %+v", connectData)
	}
	if !reflect.DeepEqual(connectData.FailoverModeSettings, map[string]string{"TYPE": "select", "METHOD": "basic", "RETRIES": "3", "DELAY": "5"}) {
		t.Errorf("failover mode settings = %v", connectData.FailoverModeSettings)
	}
	if connectData.AdditionalSettings["CUSTOM_DATA"] != "data-value" {
		t.Errorf("connect-data additional settings = %v", connectData.AdditionalSettings)
	}
}

// TestHandleDsnUsesDefaultSecurityValues verifies the security defaults on a
// resolved attempt when the descriptor does not contain a SECURITY node.
func TestHandleDsnUsesDefaultSecurityValues(t *testing.T) {
	t.Parallel()

	config := NewOracleDriverConfig()
	if !config.ConnectionProperties.SSLServerDNMatch {
		t.Fatal("default SSL server DN matching = false, want true")
	}
	if config.ConnectionProperties.SSLAllowWeakDNMatch {
		t.Fatal("default weak DN matching = true, want false")
	}

	plans, err := HandleDsn(
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=192.0.2.70)(PORT=2484))(CONNECT_DATA=(SERVICE_NAME=svc)))`,
		config,
	)
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "default-security-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	security := attempt.GetSecurity()
	if !security.SSLServerDNMatch || security.SSLAllowWeakDNMatch || security.SSLServerCertDN != "" || security.WalletLocation != "" {
		t.Fatalf("unexpected default security settings: %+v", security)
	}
}

// TestHandleDsnRejectsInvalidTNSValues covers invalid values that must fail
// during typed descriptor parsing rather than being deferred to connection setup.
func TestHandleDsnRejectsInvalidTNSValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dsn  string
	}{
		{name: "invalid description boolean", dsn: `(DESCRIPTION=(USE_SNI=maybe)(ADDRESS=(HOST=192.0.2.61))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "negative duration", dsn: `(DESCRIPTION=(CONNECT_TIMEOUT=-1)(ADDRESS=(HOST=192.0.2.61))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "invalid address protocol", dsn: `(DESCRIPTION=(ADDRESS=(PROTOCOL=HTTP)(HOST=192.0.2.61))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "invalid listener port", dsn: `(DESCRIPTION=(ADDRESS=(HOST=192.0.2.61)(PORT=65536))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "invalid proxy port", dsn: `(DESCRIPTION=(ADDRESS=(HOST=192.0.2.61)(HTTPS_PROXY=proxy)(HTTPS_PROXY_PORT=65536))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "invalid security boolean", dsn: `(DESCRIPTION=(ADDRESS=(HOST=192.0.2.61))(SECURITY=(SSL_SERVER_DN_MATCH=maybe))(CONNECT_DATA=(SERVICE_NAME=svc)))`},
		{name: "invalid nested failover mode", dsn: `(DESCRIPTION=(ADDRESS=(HOST=192.0.2.61))(CONNECT_DATA=(SERVICE_NAME=svc)(FAILOVER_MODE=(TYPE=(NESTED=value)))))`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := HandleDsn(test.dsn, NewOracleDriverConfig()); err == nil {
				t.Fatalf("HandleDsn accepted invalid descriptor: %s", test.dsn)
			}
		})
	}
}

func TestHandleDsnNormalizesNegativeRetryCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dsn  string
	}{
		{
			name: "TNS descriptor",
			dsn:  `(DESCRIPTION=(RETRY_COUNT=-1)(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.61))(CONNECT_DATA=(SERVICE_NAME=svc)))`,
		},
		{
			name: "EZConnect query",
			dsn:  `tcp://192.0.2.61:1521/svc?retry_count=-1`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plans, err := HandleDsn(test.dsn, NewOracleDriverConfig())
			if err != nil {
				t.Fatalf("HandleDsn failed: %v", err)
			}
			if got := plans.BaseConfig.ConnectionProperties.RetryCount; got != 0 {
				t.Errorf("base retry count = %d, want 0", got)
			}
			if plans.Plan.Descriptions[0].RetryCount != nil && *plans.Plan.Descriptions[0].RetryCount != 0 {
				t.Errorf("description retry count = %d, want 0", *plans.Plan.Descriptions[0].RetryCount)
			}
		})
	}
}

// TestHandleDsnAppliesEZConnectQuerySettings verifies query properties are
// applied to the effective attempt configuration rather than serialized into a
// generated TNS descriptor.
func TestHandleDsnAppliesEZConnectQuerySettings(t *testing.T) {
	t.Parallel()
	plans, err := HandleDsn(
		"tcps://db.example.com:2484/orders?retry_count=3&retry_delay=1&https_proxy=proxy.example.com&https_proxy_port=8080&ssl_server_dn_match=true",
		NewOracleDriverConfig(),
	)
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "query-settings-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if attempt.GetRetryCount() != 3 || attempt.GetRetryDelay() != time.Second {
		t.Errorf("retry policy = (%d, %v), want (3, 1s)", attempt.GetRetryCount(), attempt.GetRetryDelay())
	}
	properties := attempt.GetEffectiveConfig().ConnectionProperties
	if properties.HttpsProxy != "proxy.example.com" || properties.HttpsProxyPort != 8080 {
		t.Errorf("proxy settings = (%q, %d), want (%q, %d)", properties.HttpsProxy, properties.HttpsProxyPort, "proxy.example.com", 8080)
	}
	if !attempt.GetSecurity().SSLServerDNMatch {
		t.Error("ssl_server_dn_match = false, want true")
	}
}

// TestHandleDsnPreservesQuotedSecurityValues covers values that were
// previously checked from the naming iterator tests. These are DSN parsing and
// configuration behaviors; iteration is only used to inspect the resolved
// attempt values.
func TestHandleDsnPreservesQuotedSecurityValues(t *testing.T) {
	t.Parallel()

	t.Run("EZConnect wallet location", func(t *testing.T) {
		t.Parallel()
		plans, err := HandleDsn(`tcps://192.0.2.41:2484/mydb?wallet_location="C:\my_wallet with space"`, NewOracleDriverConfig())
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "quoted-wallet-test")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if got, want := attempt.GetSecurity().WalletLocation, `C:\my_wallet with space`; got != want {
			t.Fatalf("wallet location = %q, want %q", got, want)
		}
	})

	t.Run("EZConnect wallet location alias", func(t *testing.T) {
		t.Parallel()
		plans, err := HandleDsn(`tcps://192.0.2.41:2484/mydb?my_wallet_directory="C:\my_wallet"`, NewOracleDriverConfig())
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "wallet-alias-test")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if got, want := attempt.GetSecurity().WalletLocation, `C:\my_wallet`; got != want {
			t.Fatalf("wallet location alias = %q, want %q", got, want)
		}
	})

	t.Run("TNS server certificate DN", func(t *testing.T) {
		t.Parallel()
		dsn := `(DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=192.0.2.42)(PORT=2484))` +
			`(CONNECT_DATA=(SERVICE_NAME=mydb))` +
			`(SECURITY=(SSL_SERVER_DN_MATCH=ON)(SSL_SERVER_CERT_DN="CN=My Server, OU=Org")))`
		plans, err := HandleDsn(dsn, NewOracleDriverConfig())
		if err != nil {
			t.Fatalf("HandleDsn failed: %v", err)
		}
		iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "quoted-dn-test")
		if err != nil {
			t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
		}
		attempt, err := iterator.Next()
		if err != nil {
			t.Fatalf("iterator.Next failed: %v", err)
		}
		if got, want := attempt.GetSecurity().SSLServerCertDN, `CN=My Server, OU=Org`; got != want {
			t.Fatalf("server certificate DN = %q, want %q", got, want)
		}
	})
}

func TestHandleDsnAppliesQueryPropertiesAndParsesTNS(t *testing.T) {
	t.Parallel()
	input := `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.44)(PORT=1521))` +
		`(CONNECT_DATA=(SERVICE_NAME=example)))` +
		`?oracle.go.Locale.Language=fr&oracle.go.DriverProperties.DefaultLobPrefetchSize=12345` +
		`&oracle.go.DriverProperties.StrictNullValueHandling=false`
	plans, err := HandleDsn(input, NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if len(plans.Plan.Descriptions) != 1 {
		t.Fatalf("description count = %d, want 1", len(plans.Plan.Descriptions))
	}
	description := plans.Plan.Descriptions[0]
	if got := description.ConnectData.ServiceName; got != "example" {
		t.Fatalf("service name = %q, want example", got)
	}
	if got := plans.BaseConfig.Locale.Language; got != "fr" {
		t.Fatalf("locale language = %q, want fr", got)
	}
	if got := plans.BaseConfig.DriverProperties.DefaultLobPrefetchSize; got != 12345 {
		t.Fatalf("default LOB prefetch size = %d, want 12345", got)
	}
	if plans.BaseConfig.DriverProperties.StrictNullValueHandling {
		t.Fatal("strict null handling = true, want false")
	}
	if strings.Contains(plans.BaseConfig.ConnectDescriptor, "?") {
		t.Fatalf("normalized connect descriptor still contains query settings: %s", plans.BaseConfig.ConnectDescriptor)
	}
}

func TestHandleDsnAcceptsDescriptorWithoutCredentials(t *testing.T) {
	t.Parallel()
	plans, err := HandleDsn(
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=192.0.2.45)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=example)))`,
		NewOracleDriverConfig(),
	)
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if plans.BaseConfig.Credentials.User != "" || plans.BaseConfig.Credentials.Password != "" {
		t.Fatalf("empty credentials changed: %+v", plans.BaseConfig.Credentials)
	}
}

func TestHandleDsnParsesCredentialsFromDSN(t *testing.T) {
	t.Parallel()
	plans, err := HandleDsn("dsnuser/dsnpassword@db.example.com:1521/orders", NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if plans.BaseConfig.Credentials.User != "dsnuser" || plans.BaseConfig.Credentials.Password != "dsnpassword" {
		t.Fatalf("parsed credentials = (%q, %q), want (dsnuser, dsnpassword)", plans.BaseConfig.Credentials.User, plans.BaseConfig.Credentials.Password)
	}
	if plans.BaseConfig.ConnectDescriptor != "db.example.com:1521/orders" {
		t.Fatalf("connect descriptor = %q, want db.example.com:1521/orders", plans.BaseConfig.ConnectDescriptor)
	}
	if len(plans.Plan.Descriptions) != 1 || plans.Plan.Descriptions[0].ConnectData == nil || plans.Plan.Descriptions[0].ConnectData.ServiceName != "orders" {
		t.Fatalf("unexpected plan after credential extraction: %+v", plans.Plan)
	}
}

func TestHandleDsnRejectsInvalidOrConflictingCredentials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dsn     string
		config  func() *OracleDriverConfig
		wantErr oracleErrors.ErrorCode
	}{
		{
			name:    "missing password separator",
			dsn:     "dsnuser@db.example.com/orders",
			config:  NewOracleDriverConfig,
			wantErr: oracleErrors.NamingDSNInvalid,
		},
		{
			name:    "empty username",
			dsn:     "/dsnpassword@db.example.com/orders",
			config:  NewOracleDriverConfig,
			wantErr: oracleErrors.NamingDSNInvalid,
		},
		{
			name: "credentials already configured",
			dsn:  "dsnuser/dsnpassword@db.example.com/orders",
			config: func() *OracleDriverConfig {
				config := NewOracleDriverConfig()
				config.Credentials.User = "configuser"
				return config
			},
			wantErr: oracleErrors.ConflictingConnectionParameterSource,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := HandleDsn(test.dsn, test.config())
			assertConfigErrorCode(t, err, test.wantErr)
		})
	}
}

func TestHandleDsnRejectsInvalidLegacyDSNInputs(t *testing.T) {
	t.Parallel()
	inputs := []struct {
		name string
		dsn  string
	}{
		{name: "empty DSN", dsn: ""},
		{name: "invalid EZConnect port", dsn: "host:abc/mydb"},
		{name: "malformed query property", dsn: "host:1521/mydb?invalidparam"},
		{name: "unsupported descriptor root", dsn: "(FOO=BAR)"},
		{name: "comma between descriptor nodes", dsn: "(DESCRIPTION=(RECV_TIMEOUT=5s),(CONNECT_TIMEOUT=2s)(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"},
	}
	for _, input := range inputs {
		input := input
		t.Run(input.name, func(t *testing.T) {
			t.Parallel()
			if _, err := HandleDsn(input.dsn, NewOracleDriverConfig()); err == nil {
				t.Fatal("HandleDsn accepted invalid DSN")
			}
		})
	}
}

func TestHandleRedirectDsnAllowsConnectDataInRedirectPacket(t *testing.T) {
	t.Parallel()
	baseConfig := NewOracleDriverConfig()
	baseConfig.ConnectionProperties.SendBufSize = 4096
	plans, err := HandleRedirectDsn(
		`(DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=192.0.2.43)(PORT=2484)))`,
		baseConfig,
	)
	if err != nil {
		t.Fatalf("HandleRedirectDsn failed: %v", err)
	}
	if plans.BaseConfig == baseConfig {
		t.Fatal("redirect plans should own a clone of the active configuration")
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "redirect-plan-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if attempt.GetHost() != "192.0.2.43" || attempt.GetPort() != 2484 || attempt.GetProtocol() != driverCommon.ProtocolTCPS {
		t.Fatalf("unexpected redirect endpoint: %s://%s:%d", attempt.GetProtocol(), attempt.GetHost(), attempt.GetPort())
	}
	if got := attempt.GetEffectiveConfig().ConnectionProperties.SendBufSize; got != 4096 {
		t.Fatalf("inherited send buffer size = %d, want 4096", got)
	}
}

// TestHandleDsnRejectsMalformedDescriptors verifies that malformed TNS
// descriptors fail while building the normalized connection plan, before an
// iterator or network session is created.
func TestHandleDsnRejectsMalformedDescriptors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dsn     string
		wantErr oracleErrors.ErrorCode
	}{
		{
			name:    "unclosed descriptor",
			dsn:     "(DESCRIPTION",
			wantErr: oracleErrors.NamingParseBounds,
		},
		{
			name:    "missing CONNECT_DATA",
			dsn:     "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521)))",
			wantErr: oracleErrors.NamingContextError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := HandleDsn(test.dsn, NewOracleDriverConfig())
			assertConfigErrorCode(t, err, test.wantErr)
		})
	}
}

// TestConnectionAttemptBuildsConnectDescriptorWithClientInfo verifies that
// session-specific CONNECT_DATA is serialized from the resolved attempt and
// replaces any caller-supplied CID or CONNECTION_ID fields.
func TestConnectionAttemptBuildsConnectDescriptorWithClientInfo(t *testing.T) {
	t.Parallel()

	dsn := `(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=db.example.com)(PORT=1521))` +
		`(CONNECT_DATA=(SERVICE_NAME=orders)(CID=(PROGRAM=caller)(HOST=caller)(USER=caller))` +
		`(CONNECTION_ID=caller-id)))`
	baseConfig := NewOracleDriverConfig()
	baseConfig.ConnectionProperties.ConnectionIDPrefix = "prefix-"
	plans, err := HandleDsn(dsn, baseConfig)
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "generated-id")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}

	const want = `(CID=(PROGRAM=driver)(HOST=client)(USER=alice))(CONNECTION_ID=prefix-generated-id)`
	if attempt.GetConnectionID() != "prefix-generated-id" {
		t.Fatalf("connection ID = %q, want prefix-generated-id", attempt.GetConnectionID())
	}
	descriptor := attempt.GetConnectDescriptorWithClientInfo("driver", "client", "alice")
	if !strings.Contains(descriptor, "(HOST=db.example.com)") {
		t.Fatalf("descriptor does not contain resolved target host: %s", descriptor)
	}
	if !strings.Contains(descriptor, want) {
		t.Fatalf("descriptor does not contain generated session values %s: %s", want, descriptor)
	}
	if strings.Contains(descriptor, "caller") || strings.Contains(descriptor, "caller-id") {
		t.Fatalf("descriptor retained caller-supplied session values: %s", descriptor)
	}
}

func TestConnectionAttemptSerializesAddressProxy(t *testing.T) {
	t.Parallel()
	dsn := `(DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=db.example.com)(PORT=2484)` +
		`(HTTPS_PROXY=proxy.example.com)(HTTPS_PROXY_PORT=8080))` +
		`(CONNECT_DATA=(SERVICE_NAME=orders)))`
	plans, err := HandleDsn(dsn, NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "proxy-attempt-test")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if attempt.GetHTTPSProxy() != "proxy.example.com" || attempt.GetHTTPSProxyPort() != 8080 {
		t.Fatalf("proxy = %q:%d, want proxy.example.com:8080", attempt.GetHTTPSProxy(), attempt.GetHTTPSProxyPort())
	}
	for _, descriptor := range []string{attempt.GetAddressDescriptor(), attempt.GetConnectDescriptor()} {
		if !strings.Contains(descriptor, "(HTTPS_PROXY=proxy.example.com)") || !strings.Contains(descriptor, "(HTTPS_PROXY_PORT=8080)") {
			t.Errorf("proxy settings missing from descriptor: %s", descriptor)
		}
	}
}

// TestHandleDsnPreservesExplicitEmptyConnectDataQuery verifies that config
// assignment and the query-field presence set together retain an explicit empty
// CONNECT_DATA value through attempt resolution.
func TestHandleDsnPreservesExplicitEmptyConnectDataQuery(t *testing.T) {
	t.Parallel()

	plans, err := HandleDsn("(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=db.example.com)(PORT=1521))(CONNECT_DATA=))?service_name=", NewOracleDriverConfig())
	if err != nil {
		t.Fatalf("HandleDsn failed: %v", err)
	}
	if _, ok := plans.queryConnectDataSet["SERVICE_NAME"]; !ok {
		t.Fatal("query field set does not record explicitly supplied SERVICE_NAME")
	}

	iterator, err := plans.NewConnectionAttemptIterator(context.Background(), "empty-service-id")
	if err != nil {
		t.Fatalf("NewConnectionAttemptIterator failed: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("iterator.Next failed: %v", err)
	}
	if _, ok := attempt.GetConnectDataSet()["SERVICE_NAME"]; !ok {
		t.Fatal("attempt lost the explicit SERVICE_NAME presence marker")
	}
	if descriptor := attempt.GetConnectDataDescriptor(); !strings.Contains(descriptor, "(SERVICE_NAME=)") {
		t.Fatalf("descriptor did not preserve the explicit empty SERVICE_NAME: %s", descriptor)
	}
}

func assertConfigErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %s, got nil", want)
	}
	sqle, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T (%v)", err, err)
	}
	if got := sqle.ErrorCode(); got != string(want) {
		t.Fatalf("error code = %s, want %s (%v)", got, want, err)
	}
}

func TestParseSimple(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if root.Name != "DESCRIPTION" {
		t.Errorf("Expected root name 'DESCRIPTION', got '%s'", root.Name)
	}
	if len(root.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(root.Children))
	}
	connectData := root.Children[0]
	if connectData.Name != "CONNECT_DATA" {
		t.Errorf("Expected child name 'CONNECT_DATA', got '%s'", connectData.Name)
	}
	if len(connectData.Children) != 1 {
		t.Errorf("Expected 1 grandchild, got %d", len(connectData.Children))
	}
	serviceName := connectData.Children[0]
	if serviceName.Name != "SERVICE_NAME" {
		t.Errorf("Expected grandchild name 'SERVICE_NAME', got '%s'", serviceName.Name)
	}
	if serviceName.Value != "example" {
		t.Errorf("Expected value 'example', got '%s'", serviceName.Value)
	}
}

func TestParseComplex(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=example)(SERVER=DEDICATED)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if root.Name != "DESCRIPTION" {
		t.Errorf("Expected root name 'DESCRIPTION', got '%s'", root.Name)
	}
	if len(root.Children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(root.Children))
	}
}

func TestParseEmpty(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor("")
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseWhitespaceOnly(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor(" \t\n\r ")
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example"
	_, err := ParseConnectorDescriptor(input)
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
}

func TestGetValue(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	value, err := root.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if value != "example" {
		t.Errorf("Expected 'example', got '%s'", value)
	}
}

func TestGetValueNotFound(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetValue("DESCRIPTION/NOT_FOUND")
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePathSegment)
}

func TestGetNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	node, err := root.GetNode("DESCRIPTION/CONNECT_DATA")
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if node.Name != "CONNECT_DATA" {
		t.Errorf("Expected 'CONNECT_DATA', got '%s'", node.Name)
	}
}

func TestToString(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	output := root.ToString()
	if output != input {
		t.Errorf("ToString mismatch: expected '%s', got '%s'", input, output)
	}
}

func TestParse_PreservesQuotedValues(t *testing.T) {
	t.Parallel()
	input := `(DESCRIPTION=(SECURITY=(SSL_SERVER_CERT_DN="CN=My Server, OU=Org"))(CONNECT_DATA=(SERVICE_NAME="Sales Service")))`
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	v, err := root.GetValue("DESCRIPTION/SECURITY/SSL_SERVER_CERT_DN")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != `CN=My Server, OU=Org` {
		t.Fatalf("expected wrapping quotes to be removed, got %q", v)
	}
	out := root.ToString()
	if out != input {
		t.Fatalf("descriptor round trip changed quoted values\ninput:  %s\noutput: %s", input, out)
	}
}

func TestChildCount(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)(SERVER=DEDICATED)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	connectData := root.Children[0]
	if connectData.ChildCount() != 2 {
		t.Errorf("Expected 2 children, got %d", connectData.ChildCount())
	}
}

func TestGetChild(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	connectData := root.Children[0]
	child, err := connectData.GetChild(0)
	if err != nil {
		t.Fatalf("GetChild failed: %v", err)
	}
	if child.Name != "SERVICE_NAME" {
		t.Errorf("Expected 'SERVICE_NAME', got '%s'", child.Name)
	}
}

func TestParseUnexpectedOpeningParenWithoutName(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor("(")
	if err == nil {
		t.Error("Expected error for '(' without name")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePosition)
}

func TestParseUnexpectedClosingParen(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor(")")
	if err == nil {
		t.Error("Expected error for unexpected ')'")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingUnexpectedClosingParenthesis)
}

func TestParseUnexpectedToken(t *testing.T) {
	t.Parallel()
	_, err := ParseConnectorDescriptor("foo")
	if err == nil {
		t.Error("Expected error for unexpected token")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseValue)
}

func TestGetValueNonLeafNode(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetValue("DESCRIPTION/CONNECT_DATA")
	if err == nil {
		t.Error("Expected error for non-leaf node")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePath)
}

func TestGetNodeEmptyPath(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("")
	if err == nil {
		t.Error("Expected error for empty path")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingInputMissing)
}

func TestGetNodeWrongRootName(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("WRONG")
	if err == nil {
		t.Error("Expected error for wrong root name")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseValues)
}

func TestGetNodeEmptySegment(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetNode("DESCRIPTION//CONNECT_DATA")
	if err == nil {
		t.Error("Expected error for empty path segment")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParsePathSegment)
}

func TestGetChildInvalidIndex(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_, err = root.GetChild(-1)
	if err == nil {
		t.Error("Expected error for negative index")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
	_, err = root.GetChild(100)
	if err == nil {
		t.Error("Expected error for out of bounds index")
	}
	assertConfigErrorCode(t, err, oracleErrors.NamingParseBounds)
}

func TestGetNodeRootPath(t *testing.T) {
	t.Parallel()
	input := "(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=example)))"
	root, err := ParseConnectorDescriptor(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	node, err := root.GetNode("DESCRIPTION")
	if err != nil {
		t.Errorf("GetNode failed: %v", err)
	}
	if node.Name != "DESCRIPTION" {
		t.Errorf("Expected 'DESCRIPTION', got '%s'", node.Name)
	}
}

func TestParseIterative_NoTokens(t *testing.T) {
	t.Parallel()
	_, err := parseIterative([]string{})
	assertConfigErrorCode(t, err, oracleErrors.NamingTokensMissing)
}

func TestParseMainURL_AllComponents(t *testing.T) {
	t.Parallel()

	parsed, serverMode, err := parseEazyConnectDescriptor("tcps://host1:2484,host2:2484/mydb:dedicated/inst1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.protocol != driverCommon.ProtocolTCPS {
		t.Errorf("expected TCPS protocol, got %v", parsed.protocol)
	}
	if parsed.serviceName != "mydb" {
		t.Errorf("expected mydb service, got %q", parsed.serviceName)
	}
	if serverMode != "DEDICATED" {
		t.Errorf("expected DEDICATED mode, got %q", serverMode)
	}
	if parsed.instanceName != "inst1" {
		t.Errorf("expected inst1 instance, got %q", parsed.instanceName)
	}
	if len(parsed.addressGroups) != 1 || len(parsed.addressGroups[0].hosts) != 2 {
		t.Fatalf("expected one address group with two hosts, got %+v", parsed.addressGroups)
	}
}

func TestParseMainURL_MinimalURL(t *testing.T) {
	t.Parallel()

	parsed, serverMode, err := parseEazyConnectDescriptor("host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.protocol != driverCommon.ProtocolTCP {
		t.Errorf("expected default TCP protocol, got %v", parsed.protocol)
	}
	if parsed.serviceName != "" {
		t.Errorf("expected empty service name, got %q", parsed.serviceName)
	}
	if serverMode != "" {
		t.Errorf("expected empty server mode, got %q", serverMode)
	}
}

func TestParseEzConnect_Protocols(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		protocol driverCommon.Protocol
	}{
		{"tcp://myhost:1521/mydb", driverCommon.ProtocolTCP},
		{"tcps://myhost:2484/securedb", driverCommon.ProtocolTCPS},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.protocol != tc.protocol {
				t.Errorf("protocol = %v, want %v", parsed.protocol, tc.protocol)
			}
		})
	}
}

func TestParseEzConnect_InvalidProtocol(t *testing.T) {
	t.Parallel()

	parsed, _, err := parseEazyConnectDescriptor("http://myhost:1521/mydb")
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if !strings.Contains(err.Error(), "http") || !strings.Contains(err.Error(), "protocol") {
		t.Errorf("expected error mentioning protocol http, got: %v", err)
	}
	if parsed != nil {
		t.Error("expected nil parsed result on error")
	}
}

func TestParseEzConnect_InvalidPorts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		value string
	}{
		{"host:abc/mydb", "abc"},
		{"host:0/mydb", "0"},
		{"host:-1/mydb", "-1"},
		{"host:65536/mydb", "65536"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor(tc.input)
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if !strings.Contains(err.Error(), tc.value) || !strings.Contains(err.Error(), "port") {
				t.Errorf("expected error mentioning value %q and field port, got: %v", tc.value, err)
			}
			if parsed != nil {
				t.Errorf("expected nil result on error for %q", tc.input)
			}
		})
	}
}

func TestParseEzConnect_ValidPort_Boundary(t *testing.T) {
	t.Parallel()

	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			parsed, _, err := parseEazyConnectDescriptor("host:" + port + "/mydb")
			if err != nil {
				t.Fatalf("unexpected error for port %s: %v", port, err)
			}
			if got := parsed.addressGroups[0].hosts[0].port; got != port {
				t.Errorf("port = %q, want %q", got, port)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		port    string
		want    int
		wantErr bool
	}{
		{port: "1", want: 1},
		{port: "1521", want: 1521},
		{port: "65535", want: 65535},
		{port: "0", wantErr: true},
		{port: "-1", wantErr: true},
		{port: "65536", wantErr: true},
		{port: "abc", wantErr: true},
		{port: "", wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.port, func(t *testing.T) {
			t.Parallel()
			value, err := validatePort(reflect.ValueOf(test.port), "PORT")
			if test.wantErr {
				assertConfigErrorCode(t, err, oracleErrors.InvalidConnectionParameter)
				return
			}
			if err != nil {
				t.Fatalf("validatePort(%q) returned unexpected error: %v", test.port, err)
			}
			if got, ok := value.(int); !ok || got != test.want {
				t.Fatalf("validatePort(%q) = %v, want int %d", test.port, value, test.want)
			}
		})
	}
}

func TestParseEzConnect_EmptyAddressGroups(t *testing.T) {
	t.Parallel()

	parsed, _, err := parseEazyConnectDescriptor(";;/mydb")
	if err == nil {
		t.Fatal("expected error for no valid address groups")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("expected error mentioning host, got: %v", err)
	}
	if parsed != nil {
		t.Error("expected nil parsed result on error")
	}
}

func TestParseHostList_TrailingComma(t *testing.T) {
	t.Parallel()

	_, err := parseHostList("host1:1521,")
	if err == nil {
		t.Fatal("expected error for empty host in list")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("expected error mentioning host, got: %v", err)
	}
}

func TestParseHostList_MultipleHosts(t *testing.T) {
	t.Parallel()

	hosts, err := parseHostList("host1:1521,host2:1522,host3:1523")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
}

func TestParseHostList_NormalizesBracketedIPv6Hosts(t *testing.T) {
	t.Parallel()
	hosts, err := parseHostList("[::1]:1521,[2001:db8::1]:1522")
	if err != nil {
		t.Fatalf("parseHostList failed: %v", err)
	}
	want := []hostPort{{host: "::1", port: "1521"}, {host: "2001:db8::1", port: "1522"}}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("hosts = %+v, want %+v", hosts, want)
	}
}

func TestParseHostList_DefaultPort_InMiddle(t *testing.T) {
	t.Parallel()

	hosts, err := parseHostList("host1,host2:1522")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	if hosts[0].host != "host1" || hosts[0].port != defaultOraclePort {
		t.Errorf("expected host1 default port %s, got %+v", defaultOraclePort, hosts[0])
	}
	if hosts[1].host != "host2" || hosts[1].port != "1522" {
		t.Errorf("expected host2:1522, got %+v", hosts[1])
	}
}

func TestParseAddressGroups_MultipleGroups(t *testing.T) {
	t.Parallel()

	groups, err := parseAddressGroups("host1:1521;host2:1522;host3:1523")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}
}

func TestParseAddressGroups_EmptyString(t *testing.T) {
	t.Parallel()

	if _, err := parseAddressGroups(""); err == nil {
		t.Fatal("expected error for empty address groups")
	}
}
