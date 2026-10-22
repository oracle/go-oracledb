/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** SPDX-License-Identifier: UPL-1.0
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
package tests

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestVersionUnmarshalJSON checks valid version decoding and rejects malformed
// version strings and non-string JSON values.
func TestVersionUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var got Version
	if err := json.Unmarshal([]byte(`"23.4.1"`), &got); err != nil {
		t.Fatalf("unmarshal valid version: %v", err)
	}
	if got != (Version{Major: 23, Minor: 4, Micro: 1}) {
		t.Fatalf("version = %+v, want 23.4.1", got)
	}

	for _, input := range []string{`"23.4"`, `"23.x.1"`, `23.4`} {
		t.Run(input, func(t *testing.T) {
			var version Version
			if err := json.Unmarshal([]byte(input), &version); err == nil {
				t.Fatalf("unmarshal %s succeeded, want an error", input)
			}
		})
	}
}

// TestTestConfigMergeWithCopiesHostAndProtocolIndependently verifies that the
// host and protocol overrides are copied into their matching fields.
func TestTestConfigMergeWithCopiesHostAndProtocolIndependently(t *testing.T) {
	t.Parallel()
	target := &TestConfig{}
	target.Database.Host = "old.example.test"
	target.Database.Protocol = "tcp"
	source := &TestConfig{}
	source.Database.Host = "db.example.test"
	source.Database.Protocol = "tcps"
	target.MergeWith(source)

	if target.Database.Host != "db.example.test" {
		t.Errorf("Host = %q, want %q", target.Database.Host, source.Database.Host)
	}
	if target.Database.Protocol != "tcps" {
		t.Errorf("Protocol = %q, want %q", target.Database.Protocol, source.Database.Protocol)
	}
}

// TestTestConfigCloneCopiesAllFields verifies that Clone preserves the full
// test configuration value.
func TestTestConfigCloneCopiesAllFields(t *testing.T) {
	t.Parallel()
	source := &TestConfig{}
	source.ConfigName = "phoenix23c"
	source.DatabaseVersion = Version{Major: 23, Minor: 4, Micro: 1}
	source.Enabled = true
	source.Driver.Name = "oracledb"
	source.Database.ServiceName = "service.example.test"
	source.Database.SIDName = "sid"
	source.Database.InstanceName = "instance"
	source.Database.Host = "db.example.test"
	source.Database.Port = 1521
	source.Database.Protocol = "tcps"
	source.Database.ServerType = "pooled"
	source.Credentials.Username = "test-user"
	source.Credentials.Password = "test-password"
	source.Credentials.LogonMode = "sysdba"
	source.Security.WalletLocation = "/wallet"
	source.Security.SslServerDnMatch = "true"
	source.Security.SslServerCertDn = "CN=db.example.test"
	source.Security.SslAllowWeakDnMatch = "false"
	source.ConnectionProperties.StrictNullValueHandling = "true"

	clone := source.Clone()
	if clone == source {
		t.Fatal("Clone returned the original pointer")
	}
	if !reflect.DeepEqual(clone, source) {
		t.Fatalf("Clone() = %+v, want %+v", clone, source)
	}
}

var testCases = []CategorizedTestCase{
	{Name: "TestVersionUnmarshalJSON", Categories: "unitary", Exclusive: false, Fn: TestVersionUnmarshalJSON},
	{Name: "TestTestConfigMergeWithCopiesHostAndProtocolIndependently", Categories: "unitary", Exclusive: false, Fn: TestTestConfigMergeWithCopiesHostAndProtocolIndependently},
	{Name: "TestTestConfigCloneCopiesAllFields", Categories: "unitary", Exclusive: false, Fn: TestTestConfigCloneCopiesAllFields},
}

func TestCategoryExecutor(t *testing.T) {
	RunCategoryExecutor(t, TestCategories, testCases)
}
