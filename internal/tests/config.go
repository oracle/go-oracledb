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
package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Version stores a database version as three numeric parts.
type Version struct {
	Major int
	Minor int
	Micro int
}

func (v *Version) UnmarshalJSON(data []byte) error {
	var dotted string
	if err := json.Unmarshal(data, &dotted); err != nil {
		return err
	}

	parts := strings.Split(dotted, ".")
	if len(parts) != 3 {
		return fmt.Errorf("version must contain three dot-separated fields, got %q", dotted)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid minor version: %w", err)
	}
	micro, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("invalid patch version: %w", err)
	}
	v.Major = major
	v.Minor = minor
	v.Micro = micro
	return nil
}

// TestConfig represents one database test configuration entry.
type TestConfig struct {
	ConfigName      string  `json:"config_name"`
	DatabaseVersion Version `json:"database_version"`
	Enabled         bool

	Driver struct {
		Name string
	}

	Database struct {
		ServiceName  string
		SIDName      string `json:",omitempty"`
		InstanceName string `json:",omitempty"`
		Port         int16
		Host         string
		Protocol     string
		ServerType   string `json:",omitempty"`
	}

	Credentials struct {
		Username  string
		Password  string
		LogonMode string
	}

	Security struct {
		WalletLocation      string `json:"wallet_location,omitempty"`
		SslServerDnMatch    string `json:"ssl_server_dn_match,omitempty"`
		SslServerCertDn     string `json:"ssl_server_cert_dn,omitempty"`
		SslAllowWeakDnMatch string `json:"ssl_allow_weak_dn_match,omitempty"`
	}

	ConnectionProperties struct {
		StrictNullValueHandling string `json:"oracle.go.StrictNullValueHandling"`
	}
}

func assignStringIfNeeded(dst *string, src string) {
	if len(strings.TrimSpace(src)) > 0 {
		*dst = src
	}
}

func assignIntIfNeeded(dst *int16, src int16) {
	if src >= 0 {
		*dst = src
	}
}

func (t *TestConfig) Clone() *TestConfig {
	newOne := &TestConfig{}
	newOne.Driver.Name = t.Driver.Name
	newOne.Database.ServiceName = t.Database.ServiceName
	newOne.Database.SIDName = t.Database.SIDName
	newOne.Database.InstanceName = t.Database.InstanceName
	newOne.Database.Host = t.Database.Host
	newOne.Database.Port = t.Database.Port
	newOne.Database.Protocol = t.Database.Protocol
	newOne.Database.ServerType = t.Database.ServerType

	newOne.Credentials.Username = t.Credentials.Username
	newOne.Credentials.Password = t.Credentials.Password
	newOne.Credentials.LogonMode = t.Credentials.LogonMode

	newOne.Security = t.Security

	return newOne
}

func (t *TestConfig) MergeWith(from *TestConfig) {
	assignStringIfNeeded(&(t.Driver.Name), from.Driver.Name)

	assignStringIfNeeded(&(t.Database.ServiceName), from.Database.ServiceName)
	assignStringIfNeeded(&(t.Database.SIDName), from.Database.SIDName)
	assignStringIfNeeded(&(t.Database.InstanceName), from.Database.InstanceName)
	assignStringIfNeeded(&(t.Database.ServerType), from.Database.ServerType)
	assignIntIfNeeded(&(t.Database.Port), from.Database.Port)
	assignStringIfNeeded(&(t.Database.Host), from.Database.Protocol)
	assignStringIfNeeded(&(t.Database.Protocol), from.Database.Protocol)

	assignStringIfNeeded(&(t.Credentials.Username), from.Credentials.Username)
	assignStringIfNeeded(&(t.Credentials.Password), from.Credentials.Password)
	assignStringIfNeeded(&(t.Credentials.LogonMode), from.Credentials.LogonMode)

	assignStringIfNeeded(&(t.Security.WalletLocation), from.Security.WalletLocation)
	assignStringIfNeeded(&(t.Security.SslServerDnMatch), from.Security.SslServerDnMatch)
	assignStringIfNeeded(&(t.Security.SslServerCertDn), from.Security.SslServerCertDn)
	assignStringIfNeeded(&(t.Security.SslAllowWeakDnMatch), from.Security.SslAllowWeakDnMatch)
}

func (t *TestConfig) GetConnectionDSN() string {
	dsn := t.GetConnectionStringWithProperties(nil)
	s := strings.SplitN(dsn, "@", 2)
	return s[1]
}

func (t *TestConfig) GetConnectionString() string {
	return t.GetConnectionStringWithProperties(nil)
}

func (t *TestConfig) GetConnectionStringWithProperties(properties map[string]string) string {
	var b strings.Builder
	if properties != nil {
		for k, v := range properties {
			b.WriteString(fmt.Sprintf("(%s=%s)", k, v))
		}
	}
	res := fmt.Sprintf("%s/%s@(description=%s(address=(protocol=%s)(host=%s)(port=%d))(connect_data=",
		t.Credentials.Username,
		t.Credentials.Password,
		b.String(),
		t.Database.Protocol,
		t.Database.Host,
		t.Database.Port)

	var resC strings.Builder
	resC.WriteString(res)
	if len(t.Database.ServiceName) > 0 {
		resC.WriteString(fmt.Sprintf("(service_name=%s)", t.Database.ServiceName))
		if len(t.Database.InstanceName) > 0 {
			resC.WriteString(fmt.Sprintf("(instance_name=%s)", t.Database.InstanceName))
		}
	} else if len(t.Database.SIDName) > 0 {
		resC.WriteString(fmt.Sprintf("(sid=%s)", t.Database.SIDName))
	}

	if len(t.Database.ServerType) > 0 {
		resC.WriteString(fmt.Sprintf("(server=%s)", t.Database.ServerType))
	}

	resC.WriteString(")")

	if t.Security.WalletLocation != "" || t.Security.SslServerDnMatch != "" ||
		t.Security.SslServerCertDn != "" || t.Security.SslAllowWeakDnMatch != "" {
		resC.WriteString("(security=")
		if t.Security.WalletLocation != "" {
			resC.WriteString(fmt.Sprintf("(wallet_location=%s)", t.Security.WalletLocation))
		}
		if t.Security.SslServerDnMatch != "" {
			resC.WriteString(fmt.Sprintf("(ssl_server_dn_match=%s)", t.Security.SslServerDnMatch))
		}
		if t.Security.SslAllowWeakDnMatch != "" {
			resC.WriteString(fmt.Sprintf("(ssl_allow_weak_dn_match=%s)", t.Security.SslAllowWeakDnMatch))
		}
		if t.Security.SslServerCertDn != "" {
			resC.WriteString(fmt.Sprintf("(ssl_server_cert_dn=\"%s\")", t.Security.SslServerCertDn))
		}
		resC.WriteString(")")
	}

	resC.WriteString(")")

	if len(t.Credentials.LogonMode) > 0 {
		resC.WriteString(fmt.Sprintf("?oracle.go.Credentials.logonMode=%s", t.Credentials.LogonMode))
	}
	return resC.String()
}

func (t *TestConfig) GetConnectionStringWithMergedConfig(config *TestConfig) string {
	merged := t.Clone()
	merged.MergeWith(config)
	return merged.GetConnectionStringWithProperties(nil)
}

func (t *TestConfig) IsAutonomousDatabase() bool {
	if strings.Contains(t.Database.Host, "oraclecloud.com") && strings.Contains(t.Database.ServiceName, "adb.oraclecloud.com") {
		return true
	}
	return false
}

// TestingEnvironment holds all parsed test configurations.
type TestingEnvironment struct {
	driverConfigs []TestConfig
}

// DefaultTestConfig is kept for compatibility with existing tests.
var DefaultTestConfig *TestConfig

func NewTestingEnvironment(fileName string) (TestingEnvironment, error) {
	var driverConfigs []TestConfig

	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return TestingEnvironment{}, fmt.Errorf("specified configuration file %s do not exists", fileName)
	}

	f, err := os.Open(fileName)
	if err != nil {
		return TestingEnvironment{}, fmt.Errorf("unable to open configuration %s: %v", fileName, err)
	}
	defer func() {
		_ = f.Close()
	}()

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&driverConfigs); err != nil {
		return TestingEnvironment{}, fmt.Errorf("unable to read configuration %s: %w", fileName, err)
	}

	return TestingEnvironment{driverConfigs: driverConfigs}, nil
}

func (e *TestingEnvironment) GetConfig(name string) (*TestConfig, error) {
	if e.driverConfigs == nil {
		return nil, fmt.Errorf("attempt to get a configuration but not configuration available")
	}
	for _, config := range e.driverConfigs {
		if config.ConfigName == name {
			return &config, nil
		}
	}
	return nil, fmt.Errorf("no configuration %s found", name)
}

var TestEnvironement TestingEnvironment
var TestingConfig *TestConfig
