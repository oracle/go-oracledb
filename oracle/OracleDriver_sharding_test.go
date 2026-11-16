/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package oracle

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle/sharding"
)

// TestShardingGSMIntegration checks standalone GSM routing for NUMBER keys 5, 1,
// and 4 by querying USERENV for the expected PDB and instance. Environment
// overrides can select another deployment and an optional SSK.
// Select shda2-gsm with the standard driver.config flags, or opt in using
// GO_ORACLE_SHARDING_INTEGRATION=1 and the environment overrides below.
// Pooling, catalog discovery, and chunk-info piggybacks are not involved.
func TestShardingGSMIntegration(t *testing.T) {
	selected := TestingConfig != nil && TestingConfig.ConfigName == "shda2-gsm"
	if !selected && os.Getenv("GO_ORACLE_SHARDING_INTEGRATION") != "1" {
		t.Skip("select shda2-gsm with driver.config flags or set GO_ORACLE_SHARDING_INTEGRATION=1")
	}
	cfg := NewOracleDriverConfig()
	if selected {
		cfg.ConnectDescriptor = TestingConfig.GetConnectionDSN()
		cfg.Credentials.User = TestingConfig.Credentials.Username
		cfg.Credentials.Password = TestingConfig.Credentials.Password
	}
	for _, setting := range []struct {
		name   string
		target *string
	}{
		{"GO_ORACLE_SHARDING_GSM", &cfg.ConnectDescriptor},
		{"GO_ORACLE_SHARDING_USER", &cfg.Credentials.User},
		{"GO_ORACLE_SHARDING_PASSWORD", &cfg.Credentials.Password},
	} {
		if value := os.Getenv(setting.name); value != "" {
			*setting.target = value
		}
		if *setting.target == "" {
			t.Fatalf("missing %s or selected test configuration", setting.name)
		}
	}
	cases := []struct {
		key           int64
		pdb, instance string
	}{
		{5, "CDB2_PDB1", "shpool%1"},
		{1, "CDB3_PDB1", "shpool%11"},
		{4, "CDB4_PDB1", "shpool%21"},
	}
	// Allow reuse with another provisioned deployment without editing fixtures.
	if value := os.Getenv("GO_ORACLE_SHARDING_SK"); value != "" {
		key, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatal("SK must be int64")
		}
		pdb, instance := os.Getenv("GO_ORACLE_SHARDING_EXPECTED_PDB"), os.Getenv("GO_ORACLE_SHARDING_EXPECTED_INSTANCE")
		if pdb == "" || instance == "" {
			t.Fatal("custom SK requires EXPECTED_PDB and EXPECTED_INSTANCE overrides")
		}
		cases = cases[:1]
		cases[0].key = key
		cases[0].pdb = pdb
		cases[0].instance = instance
	}
	connector, err := NewOracleConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("NUMBER_%d", tc.key), func(t *testing.T) {
			key, err := sharding.NewKey(sharding.Subkey{Type: sharding.NUMBER, Value: tc.key})
			if err != nil {
				t.Fatal(err)
			}
			request := sharding.ShardingRequest{ShardingKey: key}
			if value := os.Getenv("GO_ORACLE_SHARDING_SSK"); value != "" {
				super, err := sharding.NewKey(sharding.Subkey{Type: sharding.VARCHAR, Value: value})
				if err != nil {
					t.Fatal(err)
				}
				request.SuperShardingKey = &super
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ctx, err = sharding.WithSharding(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := connector.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			queryer, ok := conn.(driver.QueryerContext)
			if !ok {
				t.Fatal("connection does not support QueryContext")
			}
			// Use the destination-verification SQL from the shda2 setup.
			rows, err := queryer.QueryContext(ctx, `select sys_context('USERENV', 'CON_NAME') as pdb,
       sys_context('USERENV', 'INSTANCE_NAME') as instance
from dual`, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if len(rows.Columns()) != 2 {
				t.Fatalf("expected two destination columns, got %d", len(rows.Columns()))
			}
			values := make([]driver.Value, 2)
			if err := rows.Next(values); err != nil {
				t.Fatal(err)
			}
			text := func(v driver.Value) string {
				switch x := v.(type) {
				case string:
					return x
				case []byte:
					return string(x)
				default:
					t.Fatalf("unexpected destination type %T", v)
					return ""
				}
			}
			pdb, instance := text(values[0]), text(values[1])
			if pdb != tc.pdb || instance != tc.instance {
				t.Fatalf("key %d: got PDB=%q instance=%q; want PDB=%q instance=%q", tc.key, pdb, instance, tc.pdb, tc.instance)
			}
			if err := rows.Next(values); err != io.EOF {
				t.Fatalf("expected one row, got %v", err)
			}
			t.Logf("key %d → PDB=%s instance=%s", tc.key, pdb, instance)
		})
	}
}
