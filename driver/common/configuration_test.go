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
package common

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/language"
)

// TestConfiguration_AssignFromEmptyMap checks AssignFromMap on nil or empty map
// expectations:
//
//	no failure
func TestConfiguration_AssignFromEmptyMap(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()
	err := conf.AssignFromMap(nil)
	if err != nil {
		t.Fatalf("nil map should not raise erorr")
	}
	err = conf.AssignFromMap(make(map[string]string))
	if err != nil {
		t.Fatalf("empty map should not raise erorr")
	}
}

// TestConfiguration_AssignFromEmptyMap checks AssignFromMap on nil or empty map
// expectations:
//
//	no failure
func TestConfiguration_AssignFromMapUnknownKey(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()
	m := make(map[string]string)
	m["xxxxxx"] = ""
	err := conf.AssignFromMap(m)
	if err != nil {
		t.Fatalf("unknown keys should not raise erorr")
	}
}

// TestConfiguration_AssignFromMap checks AssignFromMap
// expectations:
//
//	key from the map overwrites initial values
func TestConfiguration_AssignFromMap(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()
	conf.Credentials.User = "foo"
	conf.ConnectDescriptor = "myhost:1234/svc"

	m := make(map[string]string)
	m["oracle.go.credentials.user"] = "bar"
	m["oracle.go.connectDescriptor"] = "dbhost:1521/freedp1"
	err := conf.AssignFromMap(m)
	if err != nil {
		t.Fatalf("assign  keys should not raise error")
	}

	if strings.Compare(conf.ConnectDescriptor, "dbhost:1521/freedp1") != 0 {
		t.Fatalf("descriptor not assigned by map")
	}

	if strings.Compare(conf.Credentials.User, "bar") != 0 {
		t.Fatalf("descriptor not assigned by map")
	}
}

// TestConfiguration_AssignFromMapValidatedIntString checks AssignFromMap for
// integer fields that use validators.
// expectations:
//
//	string values are validated, converted, and assigned to integer fields.
func TestConfiguration_AssignFromMapValidatedIntString(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()

	err := conf.AssignFromMap(map[string]string{
		"oracle.go.ConnectionProperties.ConnectTimeout": "42",
	})
	if err != nil {
		t.Fatalf("validated int string should not raise error: %v", err)
	}
	if conf.ConnectionProperties.ConnectTimeout != 42 {
		t.Fatalf("connect timeout not assigned, got %d", conf.ConnectionProperties.ConnectTimeout)
	}
}

// TestConfiguration_DefaultClientLanguageIsLanguageTag checks the default
// ClientLanguage configuration value.
// expectations:
//
//	the default client language is parsed as language.English and validates.
func TestConfiguration_DefaultClientLanguageIsLanguageTag(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()

	if conf.Locale.ClientLanguage != language.English {
		t.Fatalf("expected default client language en, got %s", conf.Locale.ClientLanguage)
	}
	if err := conf.Validate(); err != nil {
		t.Fatalf("default client language should validate: %v", err)
	}
}

// TestConfiguration_AssignFromMapClientLanguageTag checks AssignFromMap for
// ClientLanguage values.
// expectations:
//
//	string language values are parsed and assigned as language.Tag values.
func TestConfiguration_AssignFromMapClientLanguageTag(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()

	err := conf.AssignFromMap(map[string]string{
		"oracle.go.Locale.ClientLanguage": "fr",
	})
	if err != nil {
		t.Fatalf("client language assignment should not raise error: %v", err)
	}
	if conf.Locale.ClientLanguage != language.French {
		t.Fatalf("expected client language fr, got %s", conf.Locale.ClientLanguage)
	}
}

// TestConfiguration_AssignFromMap checks AssignFromEnv
// expectations:
//
//	key from the map overwrites initial values
func TestConfiguration_AssignFromEnv(t *testing.T) {
	// cannot run in parallel as we are modifying the env
	conf := NewOracleDriverConfig()
	conf.Credentials.User = "foo"
	conf.ConnectDescriptor = "myhost:1234/svc"

	t.Setenv("ORACLE_GO_CREDENTIALS_USER", "bar")
	t.Setenv("ORACLE_GO_CONNECTDESCRIPTOR", "dbhost:1521/freedp1")

	err := conf.AssignFromEnv()
	if err != nil {
		t.Fatalf("assign  keys should not raise error")
	}

	if strings.Compare(conf.ConnectDescriptor, "dbhost:1521/freedp1") != 0 {
		t.Fatalf("descriptor not assigned by map")
	}

	if strings.Compare(conf.Credentials.User, "bar") != 0 {
		t.Fatalf("descriptor not assigned by map")
	}
}

// TestConfiguration_AssignFromEnvValidatedIntString checks AssignFromEnv for
// integer fields that use validators.
// expectations:
//
//	environment string values are validated, converted, and assigned to integer fields.
func TestConfiguration_AssignFromEnvValidatedIntString(t *testing.T) {
	// cannot run in parallel as we are modifying the env
	conf := NewOracleDriverConfig()
	t.Setenv("ORACLE_GO_CONNECTIONPROPERTIES_CONNECTTIMEOUT", "43")

	err := conf.AssignFromEnv()
	if err != nil {
		t.Fatalf("validated int string from env should not raise error: %v", err)
	}
	if conf.ConnectionProperties.ConnectTimeout != 43 {
		t.Fatalf("connect timeout not assigned, got %d", conf.ConnectionProperties.ConnectTimeout)
	}
}

func TestConfiguration_AssignFromEnvClientLanguageTag(t *testing.T) {
	conf := NewOracleDriverConfig()
	t.Setenv("ORACLE_GO_LOCALE_CLIENTLANGUAGE", "fr")

	err := conf.AssignFromEnv()
	if err != nil {
		t.Fatalf("client language from env should not raise error: %v", err)
	}
	if conf.Locale.ClientLanguage != language.French {
		t.Fatalf("expected client language fr, got %s", conf.Locale.ClientLanguage)
	}
}

// TestConfiguration_AssignFromEmptyFlags checks AssignFromFlags
// expectations:
//
//	no failure
func TestConfiguration_AssignFromEmptyFlags(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()

	flag.Set("oracle.go.Locale.Territory", "FOO")

	flag.Set("oracle.go.Credentials.User", "myuser")

	conf.AssignFromFlags()

	fmt.Printf("%s\n", conf.String())

	if strings.Compare(conf.Credentials.User, "myuser") != 0 {
		t.Fatalf("user from flag not propagated")
	}

	if strings.Compare(conf.Locale.Territory, "FOO") != 0 {
		t.Fatalf("territory from flag not propagated")
	}

}

// TestConfiguration_Clone checks Clone of configuration
// expectations:
//
//	 configuration clone has the same value
//		configuration items remain independent.
func TestConfiguration_Clone(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()
	conf.Credentials.User = "foo"
	conf.ConnectDescriptor = "myhost:1234/svc"
	conf.Locale.ClientLanguage = language.English
	cloneConf := conf.Clone()

	if cloneConf == nil {
		t.Fatalf("cloned configuration is nil")
	}

	if strings.Compare(conf.Credentials.User, cloneConf.Credentials.User) != 0 {
		t.Fatalf("cloned user value do not match")
	}

	if strings.Compare(conf.ConnectDescriptor, cloneConf.ConnectDescriptor) != 0 {
		t.Fatalf("cloned descriptor value do not match")
	}

	cloneConf.Credentials.User = "bar"

	conf.Locale.Territory = "none"

	if strings.Compare(conf.Credentials.User, "bar") == 0 {
		t.Fatalf("original user conf has been modified")
	}

	if strings.Compare(cloneConf.Locale.Territory, "none") == 0 {
		t.Fatalf("original Territory conf has been modified")
	}
}

// TestConfiguration_Clone checks Clone of configuration
// expectations:
//
//	 configuration clone has the same value
//		configuration items remain independent.
func TestConfiguration_toNSConnectionParameters(t *testing.T) {
	t.Parallel()
	conf := NewOracleDriverConfig()
	conf.ConnectionProperties.SSLServerCertDN = "CN=test"
	conf.ConnectionProperties.Failover = false
	conf.ConnectionProperties.HttpsProxyPort = 9000
	conf.ConnectionProperties.RetryDelay = 7

	params := conf.ToNSConnectionParameters()
	if len(params) == 0 {
		t.Fatalf("expected non-empty NS connection parameters")
	}
	want := map[string]bool{
		"ssl_server_cert_dn=\"CN=test\"": false,
		"ssl_server_dn_match=false":      false,
		"ssl_allow_weak_dn_match=false":  false,
		"https_proxy_port=9000":          false,
		"connect_timeout=0":              false,
		"expire_time=0":                  false,
		"failover=false":                 false,
		"load_balance=false":             false,
		"recv_buf_size=0":                false,
		"send_buf_size=0":                false,
		"sdu=0":                          false,
		"source_route=false":             false,
		"retry_count=0":                  false,
		"retry_delay=7":                  false,
		"transport_connect_timeout=0":    false,
		"USE_SNI=false":                  false,
	}
	tokens := strings.Split(params, "&")

	for _, s := range tokens {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}

	for param, found := range want {
		if !found {
			t.Fatalf("expected NS connection parameter %q in %q", param, params)
		}
	}
}
