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

package config

import (
	"container/list"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
)

// configuration root node name
const configurationRootName = "oracle.go"

// OracleDriverProperties defines driver-level configuration options that affect
// how Oracle driver behavior upon request exchanges with the remote database.
type OracleDriverProperties struct {
	// When enabled, SQL NULL values are returned as nil. When disabled, legacy
	// or type-specific zero-value handling may be used depending on the decoder.
	StrictNullValueHandling bool `default:"true" help:"if true, NULLs column values are returned as nil"`
	// DefaultLobPrefetchSize sets the default amount of LOB data, in bytes, to
	// prefetch when reading LOB columns.
	//
	// The default is 32 MiB.
	DefaultLobPrefetchSize int `default:"33554432" validator:"validateZeroOrPositive"  help:"default prefetch size"`
}

func (config OracleDriverProperties) String() string {
	return toString(&config)
}

func (config OracleDriverProperties) IsStrictNullValueHandling() bool {
	return config.StrictNullValueHandling
}

func (config OracleDriverProperties) GetDefaultLobPrefetchSize() int {
	return config.DefaultLobPrefetchSize
}

// OracleConnectionProperties contains Oracle Net connection properties used to
// configure transport, failover, load balancing, TLS, wallet, proxy, and timeout
// behavior when establishing a database connection.
type OracleConnectionProperties struct {
	// SSLServerCertDN specifies the distinguished name (DN) expected in the
	// database server certificate.
	SSLServerCertDN string `ns_name:"ssl_server_cert_dn" default:"" help:"specifies the distinguished name (DN) of the database server"`

	// SSLServerDNMatch enables matching the database server certificate DN
	// against SSLServerCertDN during TLS negotiation.
	SSLServerDNMatch bool `ns_name:"ssl_server_dn_match" default:"false" validator:"validateBoolean" help:"specifies the distinguished name (DN) of the database server"`

	// SSLAllowWeakDNMatch allows less strict distinguished-name matching during
	// TLS certificate validation.
	SSLAllowWeakDNMatch bool `ns_name:"ssl_allow_weak_dn_match" validator:"validateBoolean" default:"false"`

	// WalletLocation sets the directory containing Oracle wallets.
	WalletLocation string `ns_name:"wallet_location" default:"" help:"sets the directory containing the Oracle wallets"`

	// HttpsProxy sets the HTTP proxy hostname or IP address used to tunnel TLS
	// client connections.
	HttpsProxy string `ns_name:"https_proxy" default:"" help:"sets an HTTP proxy hostname or IP address for tunneling TLS client connections"`

	// HttpsProxyPort sets the HTTP proxy port used to tunnel TLS client
	// connections.
	HttpsProxyPort int `ns_name:"https_proxy_port" default:"8080" help:"sets an HTTP proxy host port for tunneling TLS client connections"`

	// ConnectTimeout sets the timeout, in seconds, for establishing an Oracle
	// Net connection.
	ConnectTimeout int `ns_name:"connect_timeout" default:"0" validator:"validateZeroOrPositive" help:"sets the timeout duration in seconds for an application to establish an Oracle Net connection"`

	// ExpireTime sets the interval, in minutes, for sending probes that verify
	// whether connections are still active.
	ExpireTime int `ns_name:"expire_time" default:"0" validator:"validateZeroOrPositive" help:"sets a time interval in minutes to send probes to verify that connections are active"`

	// Failover enables or disables connect-time failover when multiple hosts are
	// configured.
	Failover bool `ns_name:"failover" default:"true" validator:"validateBoolean" help:"enables or disables connect-time failover for multiple hosts"`

	// LoadBalance enables or disables Oracle client load balancing when multiple
	// hosts are configured.
	LoadBalance bool `ns_name:"load_balance" default:"false" validator:"validateBoolean" help:"enables or disables Oracle client load balancing for multiple hosts"`

	// ReceiveBufSize sets the TCP or TCPS socket receive buffer size, in bytes.
	ReceiveBufSize int `ns_name:"recv_buf_size" validator:"validateZeroOrPositive" help:"sets the TCP/TCPS socket receive buffer size in bytes"`

	// SendBufSize sets the TCP or TCPS socket send buffer size, in bytes.
	SendBufSize int `ns_name:"send_buf_size" validator:"validateZeroOrPositive" help:"sets the TCP/TCPS socket send buffer size in bytes"`

	// Sdu sets the Oracle Net Session Data Unit packet size, in bytes.
	Sdu int `ns_name:"sdu" validator:"validateZeroOrPositive" help:"sets the Oracle Net Session Data Unit (SDU) packet size in bytes"`

	// SourceRoute enables routing through multiple hosts.
	SourceRoute bool `ns_name:"source_route" default:"false" validator:"validateBoolean" help:"enables network routing through multiple hosts"`

	// RetryCount sets how many times the configured host list is traversed when
	// attempting to connect.
	RetryCount int `ns_name:"retry_count" default:"0" help:"sets the number of times the list of hosts is traversed when attempting to connect to Oracle Database"`

	// RetryDelay sets the delay, in seconds, between retries of host-list
	// traversal.
	RetryDelay int `ns_name:"retry_delay" default:"0" help:"sets the delay in seconds between retries of host list traversal"`

	// TransportConnectTimeout sets the transport-level timeout, in seconds, for
	// establishing a connection to an Oracle database.
	TransportConnectTimeout int `ns_name:"transport_connect_timeout" default:"0" validator:"validateZeroOrPositive" help:"sets the transport connect timeout duration in seconds for a client to establish an Oracle Net connection to an Oracle database"`

	// UseSNI enables Server Name Indication (SNI) for TLS connections.
	UseSNI bool `ns_name:"USE_SNI" default:"false" validator:"validateBoolean" help:"enables Server Name Indication (SNI) for TLS connections"`
}

// String implements the Stringer interface
func (config OracleConnectionProperties) String() string {
	return toString(&config)
}

// ToNSConnectionParameters look for parameters that have the ns_name tag assigned
// to build a query parameter string compatible with the NS layer.
func (config *OracleDriverConfig) ToNSConnectionParameters() string {
	var sb strings.Builder
	var subsb strings.Builder
	var parameterValue string = ""
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		subsb.Reset()
		NsNAme := field.element.Tag.Get("ns_name")
		if len(NsNAme) != 0 {
			// we discard convertion error as we trust default value
			switch field.elementValue.Kind() {
			case reflect.String:
				v := field.elementValue.String()
				if len(v) == 0 {
					continue
				}
				// skip empty strings
				if strings.Count(v, " ") != 0 || strings.Count(v, "=") != 0 {
					// quote it then
					parameterValue = fmt.Sprintf("\"%s\"", v)
				} else {
					parameterValue = v
				}

			case reflect.Bool:
				parameterValue = strconv.FormatBool(field.elementValue.Bool())
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				parameterValue = strconv.FormatInt(field.elementValue.Int(), 10)
			}
			if len(parameterValue) > 0 {
				if sb.Len() > 0 {
					sb.WriteString("&")
				}
				sb.WriteString(fmt.Sprintf("%s=%s", NsNAme, parameterValue))
			}

		}
	}
	return sb.String()
}

// OracleLoggingConfig defines logging options for the Oracle driver.
//
// It controls the driver log level, log destination, whether sensitive values
// may be written to logs, and whether an existing log file is truncated at
// startup. Internal fields are used by the configuration parser and are not
// intended to be set by users.
type OracleLoggingConfig struct {
	_allConfigurationField *list.List `internal:"true"`
	_configKeysPrefix      string     `internal:"true"`
	// Level sets the driver logging level.
	Level string `default:"ERROR" validator:"validateLoggingLevel" help:"driver logging level"`

	// Destination sets where driver logs are written.
	//
	// Valid values include a file path, STDOUT, STDERR, or NULL.
	Destination string `default:"NULL" help:"driver logging destination, can be a file path, STDOUT, STDERR or NULL"`

	// IncludeSensitive enables logging of sensitive information.
	//
	// This should be used carefully because logs may contain credentials,
	// connection details, SQL bind values, or application data.
	IncludeSensitive bool `default:"false" help:"activate sensitive logging information"`

	// Truncate controls whether the log file is truncated at startup.
	Truncate bool `default:"false" help:"truncate log file at startup"`
}

// NewOracleLoggingConfig creates a new driver configuration
// All fields' default value are applied
// This method is the only supported way to create a new OracleLoggingConfig
func NewOracleLoggingConfig() *OracleLoggingConfig {
	newConfig := OracleLoggingConfig{}
	newConfig._configKeysPrefix = configurationRootName + ".logging"
	newConfig._allConfigurationField = list.New()
	e := reflect.ValueOf(&newConfig).Elem()
	for i := 0; i < e.NumField(); i++ {
		newConfig._allConfigurationField.PushBack(&_fieldListElem{
			name:         fmt.Sprintf("%s.%s", newConfig._configKeysPrefix, e.Type().Field(i).Name),
			element:      e.Type().Field(i),
			elementValue: e.Field(i)})
	}
	var toBedropped *list.Element
	for item := newConfig._allConfigurationField.Front(); item != nil; item = item.Next() {
		if toBedropped != nil {
			// previous encountered struct need to be dropped
			newConfig._allConfigurationField.Remove(toBedropped)
			toBedropped = nil
		}
		field := item.Value.(*_fieldListElem)
		if shouldExpandConfigurationField(field) {
			for i := 0; i < field.element.Type.NumField(); i++ {
				fullQualifiedName := fmt.Sprintf("%s.%s", field.name, field.element.Type.Field(i).Name)
				newConfig._allConfigurationField.PushBack(&_fieldListElem{
					name:         fullQualifiedName,
					element:      field.element.Type.Field(i),
					elementValue: field.elementValue.Field(i)})
			}
			toBedropped = item
		}
	}

	// apply default values
	for item := newConfig._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		defaultValue := field.element.Tag.Get("default")
		if len(defaultValue) != 0 {
			// we discard convertion error as we trust default value
			switch field.elementValue.Kind() {
			case reflect.String:
				field.elementValue.Set(reflect.ValueOf(defaultValue))
			case reflect.Bool:
				v, _ := strconv.ParseBool(defaultValue)
				field.elementValue.SetBool(v)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				v, _ := strconv.Atoi(defaultValue)
				field.elementValue.SetInt(int64(v))
			}
		}

	}
	return &newConfig
}

// populateFlags create all logging configuration flags.
func (config *OracleLoggingConfig) populateFlags() {
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		if visible, ok := field.element.Tag.Lookup("cliVisible"); ok {
			if strings.Compare(visible, "false") == 0 {
				continue
			}
		}
		if visible, ok := field.element.Tag.Lookup("internal"); ok {
			if strings.Compare(visible, "true") == 0 {
				continue
			}
		}
		ciName := field.name
		ciDescription := field.element.Tag.Get("help")
		switch field.element.Type.Kind() {
		case reflect.String:
			flag.String(ciName, "", ciDescription)
		case reflect.Bool:
			flag.Bool(ciName, false, ciDescription)
		case reflect.Int, reflect.Int8, reflect.Int32:
			flag.Int(ciName, -1, ciDescription)
		case reflect.Uint64, reflect.Int64:
			flag.Int64(ciName, -1, ciDescription)
		}
		_allFlags[ciName] = true
	}
}
func (config OracleLoggingConfig) String() string {
	return toString(&config)
}

func (config *OracleLoggingConfig) GetDestination() string {
	return config.Destination
}

func (config *OracleLoggingConfig) GetLevel() string {
	return config.Level
}

func (config *OracleLoggingConfig) GetIncludeSensitive() bool {
	return config.IncludeSensitive
}

func (config *OracleLoggingConfig) GetTruncate() bool {
	return config.Truncate
}

// AssignFromEnv updates the logging configuration from environment variables.
//
// For each configured field, AssignFromEnv looks up the corresponding
// environment variable name, validates the value when the field declares a
// validator tag, converts the string value to the field type, and assigns it to
// the config.
//
// Fields without a matching environment variable are left unchanged.
//
// AssignFromEnv returns the first validation, conversion, or assignment error it
// encounters.
func (config *OracleLoggingConfig) AssignFromEnv() error {
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		// populate from env
		if ev, ok := os.LookupEnv(_getEnvironmentVarName(field.name)); ok {
			validatorName := field.element.Tag.Get("validator")
			if len(validatorName) > 0 {
				validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(ev), field.name)
				if err == nil {
					if err := setFieldValue(field, validatedValue); err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				switch field.elementValue.Kind() {
				case reflect.String:
					if err := setFieldValue(field, ev); err != nil {
						return err
					}
				case reflect.Bool:
					v, err := strconv.ParseBool(ev)
					if err != nil {
						return err
					} else {
						if err := setFieldValue(field, v); err != nil {
							return err
						}
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					v, err := strconv.Atoi(ev)
					if err != nil {
						return err
					}
					if err := setFieldValue(field, v); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// AssignFromFlags updates the logging configuration from command-line flags.
//
// AssignFromFlags considers only flags that were explicitly set by the user. For
// each matching configuration field, it validates the flag value when the field
// declares a validator tag, converts the value to the field type, and assigns it
// to the config.
//
// Configuration fields without a matching explicit flag are left unchanged.
//
// AssignFromFlags returns the first validation, conversion, or assignment error
// it encounters.
func (config *OracleLoggingConfig) AssignFromFlags() error {

	if !flag.Parsed() {
		flag.Parse()
	}

	// grab used flags
	enabledFlagsMap := make(map[string]flag.Value)

	flag.Visit(func(f *flag.Flag) {
		enabledFlagsMap[(*f).Name] = f.Value
	})

	var maxIter = len(enabledFlagsMap)
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		if maxIter == 0 {
			// no need to loop further
			break
		}
		field := item.Value.(*_fieldListElem)
		fValue, ok := enabledFlagsMap[field.name]
		if ok {
			maxIter--
			validatorName := field.element.Tag.Get("validator")
			if len(validatorName) > 0 {
				validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(fValue.String()), field.name)
				if err == nil {
					if err := setFieldValue(field, validatedValue); err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				switch field.elementValue.Kind() {
				case reflect.String:
					ps := fValue.(flag.Getter).Get().(string)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				case reflect.Bool:
					ps := fValue.(flag.Getter).Get().(bool)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					ps := fValue.(flag.Getter).Get().(int)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// OracleNLSParameters contains locale and NLS settings used by the driver and
// database session.
type OracleNLSParameters struct {
	// ClientLanguage selects the language used for driver error messages.
	ClientLanguage language.Tag `default:"en" cliVisible:"true" validator:"validateLanguage" help:"driver language to be used"`
	// Language sets the default database language for the session.
	Language string `default:"AMERICAN" help:"specifies the default language of the database"`
	// Territory sets the database territory whose formatting conventions are used.
	Territory string `default:"AMERICA" help:"the name of the territory whose conventions are to be followed for day and week numbering"`
}

func (config OracleNLSParameters) String() string {
	return toString(&config)
}

// OracleCredentials contains the database authentication settings used when
// establishing a new Oracle connection.
type OracleCredentials struct {
	// User is the database user name used for authentication.
	User string ` validator:"validateUserName" help:"the database user"`
	// LogonMode sets the administrative logon mode, such as SYSDBA or SYSOPER.
	LogonMode string `propertyName:"logon_mode" default:"" validator:"validateLogonMode" help:"specifies the logon mode for the connection"`
	// Password stores the database password configuration.
	Password string `cliVisible:"false" sensitive:"true"`
}

func (config OracleCredentials) String() string {
	return toString(&config)
}

func toString(item interface{}) string {
	var sb strings.Builder
	sb.WriteString("{")
	e := reflect.ValueOf(item).Elem()
	for i := 0; i < e.NumField(); i++ {
		if strings.Compare(e.Type().Field(i).Tag.Get("internal"), "true") == 0 {
			// internal field, not to be taken into account
			continue
		}
		sb.WriteString(e.Type().Field(i).Name)
		sb.WriteString("=")
		if strings.Compare(e.Type().Field(i).Tag.Get("sensitive"), "true") == 0 {
			sb.WriteString("*****")
		} else {
			switch e.Type().Field(i).Type.Kind() {
			case reflect.Struct:
				// rely on implementing the Stringer interface
				sb.WriteString(fmt.Sprintf("%v", e.Field(i).Interface()))
			case reflect.String:
				sb.WriteString(e.Field(i).String())
			case reflect.Bool:
				sb.WriteString(strconv.FormatBool(e.Field(i).Bool()))
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				sb.WriteString(strconv.FormatInt(e.Field(i).Int(), 10))
			}
		}
		sb.WriteString(",")
	}
	sb.WriteString("}")
	return sb.String()
}

// OracleDriverConfig Driver configuration template type.
//
// Field tags used:
//   - internal: the field is not considered as configuration item
//   - cliVisible: the field is not expose as cli flag
//   - propertyName: field property alias name to use instead of field's name
//   - ns_name: field property EZ connect format equivalent
//   - default: field default value as string
//   - help: field usage set in flags.
//   - validator: field validator func to be call before setting a value
//   - isConfigGroup: When set, this indicates that the field is a nested configuration group
//     that must be processed. Some fields are structs, like language.Tag but should not be
//     treated as group of configuration but as configuration value (see shouldExpandConfigurationField() )
type OracleDriverConfig struct {
	_allConfigurationField *list.List                 `internal:"true"`
	_configKeysPrefix      string                     `internal:"true"`
	ConnectDescriptor      string                     `help:"The connection descriptor for the remote database."`
	Credentials            OracleCredentials          `isConfigGroup:"true"`
	DriverProperties       OracleDriverProperties     `isConfigGroup:"true"`
	ConnectionProperties   OracleConnectionProperties `isConfigGroup:"true"`
	Locale                 OracleNLSParameters        `isConfigGroup:"true"`
}

// populateFlags create all configuration flags.
// Each (sub)field in OracleDriverConfig are define as flags.
// Fields that have the tag cliVisible set to false are not exposed
// This method is for internal use only and is supposed to be called only once.
func (config *OracleDriverConfig) populateFlags() {
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		if visible, ok := field.element.Tag.Lookup("cliVisible"); ok {
			if strings.Compare(visible, "false") == 0 {
				continue
			}
		}
		ciName := field.name
		ciDescription := field.element.Tag.Get("help")
		switch field.element.Type.Kind() {
		case reflect.String:
			flag.String(ciName, "", ciDescription)
		case reflect.Bool:
			flag.Bool(ciName, false, ciDescription)
		case reflect.Int, reflect.Int8, reflect.Int32:
			flag.Int(ciName, -1, ciDescription)
		case reflect.Uint64, reflect.Int64:
			flag.Int64(ciName, -1, ciDescription)
		case reflect.Float32, reflect.Float64:
			flag.Float64(ciName, -1, ciDescription)
		case reflect.Struct:
			if len(field.element.Tag.Get("validator")) > 0 {
				flag.String(ciName, "", ciDescription)
			}
		}
		_allFlags[ciName] = true
	}
}

// Clone clones this configuration.
func (config *OracleDriverConfig) Clone() *OracleDriverConfig {
	clone := NewOracleDriverConfig()
	clonedItem := clone._allConfigurationField.Front()
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		clonedField := clonedItem.Value.(*_fieldListElem)
		clonedField.elementValue.Set(field.elementValue)
		clonedItem = clonedItem.Next()
	}
	return clone
}

// NewOracleDriverConfig creates a new driver configuration
// All fields' default value are applied
// This method is the only supported way to create a new OracleDriverConfig
func NewOracleDriverConfig() *OracleDriverConfig {
	newConfig := OracleDriverConfig{}
	newConfig._configKeysPrefix = configurationRootName
	newConfig._allConfigurationField = list.New()
	e := reflect.ValueOf(&newConfig).Elem()
	for i := 0; i < e.NumField(); i++ {
		if strings.Compare(e.Type().Field(i).Tag.Get("internal"), "true") == 0 {
			// internal field, not to be taken into account
			continue
		}
		publicName := e.Type().Field(i).Tag.Get("propertyName")
		if len(publicName) == 0 {
			publicName = e.Type().Field(i).Name
		}
		newConfig._allConfigurationField.PushBack(&_fieldListElem{
			name:         fmt.Sprintf("%s.%s", newConfig._configKeysPrefix, publicName),
			element:      e.Type().Field(i),
			elementValue: e.Field(i)})
	}
	var toBedropped *list.Element
	for item := newConfig._allConfigurationField.Front(); item != nil; item = item.Next() {
		if toBedropped != nil {
			// previous encountered struct need to be dropped
			newConfig._allConfigurationField.Remove(toBedropped)
			toBedropped = nil
		}
		field := item.Value.(*_fieldListElem)
		if shouldExpandConfigurationField(field) {
			for i := 0; i < field.element.Type.NumField(); i++ {
				publicName := field.element.Type.Field(i).Tag.Get("propertyName")
				if len(publicName) == 0 {
					publicName = field.element.Type.Field(i).Name
				}
				fullQualifiedName := fmt.Sprintf("%s.%s", field.name, publicName)
				newConfig._allConfigurationField.PushBack(&_fieldListElem{
					name:         fullQualifiedName,
					element:      field.element.Type.Field(i),
					elementValue: field.elementValue.Field(i)})
			}
			toBedropped = item
		}
	}

	// apply default values
	for item := newConfig._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		defaultValue := field.element.Tag.Get("default")
		if len(defaultValue) != 0 {
			if err := setFieldValueFromString(field, defaultValue); err != nil {
				common.Odl.Debug("Invalid default value", "field", field.name, "err", err)
			}
		}
	}
	return &newConfig
}

// validator signature
// parameters:
//
//	value: the value to be validated
//	valueName: value (configuration item) name. used in message in case of error
//
// returns:
//
//	the validated value
//	error if any
type validatorFunc func(value reflect.Value, valueName string) (any, error)

// validators functions map
var _fieldsValidators map[string]validatorFunc

// keep track of all defined flags
var _allFlags map[string]bool

// loadFromFlags assigns configuration item value from values found in application flags
// For each configuration field, the corresponding flag is checked for assigned value.
// If a flag value is set, it is applied to the field.
// error: if flag value did not pass  validator (if any) checks.
func (config *OracleDriverConfig) loadFromFlags() error {
	// grab used flags
	enabledFlagsMap := make(map[string]flag.Value)
	flag.Visit(func(f *flag.Flag) {
		enabledFlagsMap[(*f).Name] = f.Value
	})
	var maxIter = len(enabledFlagsMap)
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		if maxIter == 0 {
			// no need to loop further
			break
		}
		field := item.Value.(*_fieldListElem)
		fValue, ok := enabledFlagsMap[field.name]
		if ok {
			maxIter--
			validatorName := field.element.Tag.Get("validator")
			if len(validatorName) > 0 {
				validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(fValue.String()), field.name)
				if err == nil {
					if err := setFieldValue(field, validatedValue); err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				switch field.elementValue.Kind() {
				case reflect.String:
					ps := fValue.(flag.Getter).Get().(string)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				case reflect.Bool:
					ps := fValue.(flag.Getter).Get().(bool)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					ps := fValue.(flag.Getter).Get().(int)
					if err := setFieldValue(field, ps); err != nil {
						return err
					}
				default:
					common.Odl.Debug("Type not supported")
				}
			}
		}
	}
	return nil
}

// doFieldNameMatches checks that given name apply to the structured field
//
//	name can be its name or JSON name as a tag or special name alias
//
// in propertyName
// return true if name matches
func doFieldNameMatches(name string, field *_fieldListElem) bool {
	if strings.EqualFold(name, field.name) {
		return true
	}
	if strings.EqualFold(name, field.element.Tag.Get("propertyName")) {
		return true
	}
	if strings.EqualFold(name, field.element.Tag.Get("ns_name")) {
		return true
	}
	jsonTag := field.element.Tag.Get("json")
	sjsonTag := strings.SplitN(jsonTag, ",", 2)
	if strings.EqualFold(name, sjsonTag[0]) {
		return true
	}
	return false
}

// loadFromMap assigns configuration item value from values found in map
// For each configuration field, the corresponding map key is checked for assigned value.
// If a map key is mapped to a value, it is applied to the field.
// error: if map value did not pass  validator (if any) checks.
func (config *OracleDriverConfig) loadFromMap(configItems map[string]string) error {
	for configItemsKey, configItemsValue := range configItems {
		for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
			field := item.Value.(*_fieldListElem)
			// populate from map keys
			if doFieldNameMatches(configItemsKey, field) {
				validatorName := field.element.Tag.Get("validator")
				if len(validatorName) > 0 {
					validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(configItemsValue), field.name)
					if err == nil {
						if err := setFieldValue(field, validatedValue); err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					switch field.elementValue.Kind() {
					case reflect.String:
						if err := setFieldValue(field, configItemsValue); err != nil {
							return err
						}
					case reflect.Bool:
						v, err := strconv.ParseBool(configItemsValue)
						if err != nil {
							return err
						}
						if err := setFieldValue(field, v); err != nil {
							return err
						}
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						v, err := strconv.Atoi(configItemsValue)
						if err != nil {
							return err
						}
						if err := setFieldValue(field, v); err != nil {
							return err
						}
					default:
						common.Odl.Debug("Type not supported")
					}
				}
				break
			}
		}
	}
	return nil
}

// implements Stringer intterface
func (config OracleDriverConfig) String() string {
	return toString(&config)
}

// loadFromEnviron assigns configuration item value from values found in current process environment.
// For each configuration field, the corresponding environment variable is checked for assigned value.
// If the variable is mapped to a value, it is applied to the field.
// A dotted configuration key is mapped to upper case variable name separated by '_'
// ex: foo.bar is mapped to FOO_BAR
// error: if environment value did not pass validator (if any) checks.
func (config *OracleDriverConfig) loadFromEnviron() error {
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		// populate from env
		if ev, ok := os.LookupEnv(_getEnvironmentVarName(field.name)); ok {
			validatorName := field.element.Tag.Get("validator")
			if len(validatorName) > 0 {
				validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(ev), field.name)
				if err == nil {
					if err := setFieldValue(field, validatedValue); err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				switch field.elementValue.Kind() {
				case reflect.String:
					if err := setFieldValue(field, ev); err != nil {
						return err
					}
				case reflect.Bool:
					v, err := strconv.ParseBool(ev)
					if err != nil {
						return err
					} else {
						if err := setFieldValue(field, v); err != nil {
							return err
						}
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					v, err := strconv.Atoi(ev)
					if err != nil {
						return err
					}
					if err := setFieldValue(field, v); err != nil {
						return err
					}
				default:
					common.Odl.Debug("Type not supported")
				}
			}
		}

	}
	return nil
}

// _getEnvironmentVarName map a configuration item name to a environment variable name
// ex : a.b.c is mapped to A_B_C
func _getEnvironmentVarName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, ".", "_"))

}

type _fieldListElem struct {
	name         string              // field fully qualified name ex oracle.go.Language
	element      reflect.StructField // field reference within OracleDriverConfig struct
	elementValue reflect.Value       // field value
}

func shouldExpandConfigurationField(field *_fieldListElem) bool {
	return field.element.Type.Kind() == reflect.Struct &&
		len(field.element.Tag.Get("isConfigGroup")) != 0
}

func setFieldValueFromString(field *_fieldListElem, value string) error {
	validatorName := field.element.Tag.Get("validator")
	if len(validatorName) > 0 {
		validatedValue, err := _fieldsValidators[validatorName](reflect.ValueOf(value), field.name)
		if err != nil {
			return err
		}
		return setFieldValue(field, validatedValue)
	}

	switch field.elementValue.Kind() {
	case reflect.String:
		return setFieldValue(field, value)
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		return setFieldValue(field, v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		return setFieldValue(field, v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		return setFieldValue(field, v)
	default:
		common.Odl.Debug("Type not supported")
		return nil
	}
}

func setFieldValue(field *_fieldListElem, value any) error {
	if !field.elementValue.CanSet() {
		return common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, value, field.name, "settable field")
	}

	targetType := field.elementValue.Type()
	valueToSet := reflect.ValueOf(value)
	if !valueToSet.IsValid() {
		return common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, value, field.name, targetType.String())
	}
	if valueToSet.Type().AssignableTo(targetType) {
		field.elementValue.Set(valueToSet)
		return nil
	}
	if valueToSet.Type().ConvertibleTo(targetType) {
		field.elementValue.Set(valueToSet.Convert(targetType))
		return nil
	}
	return common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil, value, field.name, targetType.String())
}

func init() {

	_allFlags = make(map[string]bool)

	// assign validators funcs
	_fieldsValidators = make(map[string]validatorFunc)
	_fieldsValidators["validateBoolean"] = validateBooleanValue
	_fieldsValidators["validateLogonMode"] = validateLogonModeValue
	_fieldsValidators["validateLanguage"] = validateLanguage
	_fieldsValidators["validateZeroOrPositive"] = validateZeroOrPositive
	_fieldsValidators["validateLoggingLevel"] = validateLoggingLevel
	_fieldsValidators["validateUserName"] = validateUserName

	// populates flags for each driver config items
	NewOracleDriverConfig().populateFlags()
	NewOracleLoggingConfig().populateFlags()

	// populate flag for user configuration

	// populate flag for user configuration
	flag.BoolFunc("oracledb-config-help", "Display all Oracle driver flags", func(string) error {
		io.WriteString(flag.CommandLine.Output(), "Oracle driver properties:\n\n")
		flag.VisitAll(func(vf *flag.Flag) {
			if _, ok := _allFlags[vf.Name]; ok {
				if len(vf.DefValue) > 0 {
					io.WriteString(flag.CommandLine.Output(),
						fmt.Sprintf("%s\n",
							vf.Name,
						))
					if len(vf.Usage) > 0 {
						io.WriteString(flag.CommandLine.Output(),
							fmt.Sprintf("  %s\n", vf.Usage))
					}
				} else {
					io.WriteString(flag.CommandLine.Output(),
						fmt.Sprintf("%s (default=%s)\n",
							vf.Name, vf.DefValue,
						))
					if len(vf.Usage) > 0 {
						io.WriteString(flag.CommandLine.Output(),
							fmt.Sprintf("  %s\n", vf.Usage))
					}
				}
			}
		})

		return nil
	})

}

func (config *OracleDriverConfig) AssignFromEnv() error {
	return config.loadFromEnviron()
}

func (config *OracleDriverConfig) AssignFromFlags() error {
	if !flag.Parsed() {
		flag.Parse()
	}
	return config.loadFromFlags()
}

func (config *OracleDriverConfig) AssignFromMap(items map[string]string) error {
	return config.loadFromMap(items)
}

// QueryStringToMap converts a query string (key1=value1&key2=value2&...)
// to a key/value map
// returns :
//   - the key/valeu map
//   - the error if parsing has failed
func QueryStringToMap(s string) (map[string]string, error) {
	result := make(map[string]string)
	queryParts := strings.Split(s, "&")
	for _, part := range queryParts {
		keyValuePair := strings.SplitN(part, "=", 2)
		if len(keyValuePair) != 2 {
			return nil, common.NewOracleError(oracleErrors.NamingParseFailed, fmt.Errorf("error parsing connection property [%s]", part))
		} else {
			result[strings.TrimSpace(keyValuePair[0])] = strings.TrimSpace(keyValuePair[1])
		}
	}
	return result, nil
}

// Validate runs validators on configuration fields
// error : of validation has failed.
func (config *OracleDriverConfig) Validate() error {
	for item := config._allConfigurationField.Front(); item != nil; item = item.Next() {
		field := item.Value.(*_fieldListElem)
		validatorName := field.element.Tag.Get("validator")
		if len(validatorName) != 0 {
			_, err := _fieldsValidators[validatorName](field.elementValue, field.name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// validateUserName validator for username.
// Return a trimmed version of the string
// errors:
//   - value is not assignable to a string
func validateUserName(value reflect.Value, valueName string) (any, error) {
	if value.Kind() == reflect.String {
		return strings.TrimSpace(value.String()), nil
	}
	return nil, common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName, "not string user name",
	)
}

// validateLoggingLevel validator for logging level.
// Tries to parse the value with slog.Level.UnmarshalText
// errors:
//   - value is not assignable to a string
//   - string value cannot be parsed as a logging level
func validateLoggingLevel(value reflect.Value, valueName string) (any, error) {
	if value.Kind() == reflect.String {
		if len(value.String()) != 0 {
			var l slog.Level
			if err := l.UnmarshalText([]byte(value.String())); err == nil {
				return l.String(), nil
			} else {
				return nil, common.NewOracleError(
					oracleErrors.InvalidConnectionParameter,
					nil,
					value,
					valueName, "valid logger level name",
				)
			}
		}
		return slog.LevelError.String(), nil
	}
	return nil, common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName, "valid logging level name",
	)
}

// validateZeroOrPositive validator for positive non-zero integer value.
// errors:
//   - value is not assignable to a int
//   - int value is less or equal to zero
func validateZeroOrPositive(value reflect.Value, valueName string) (any, error) {
	switch value.Kind() {
	case reflect.String:
		parsedValue, err := strconv.ParseInt(strings.TrimSpace(value.String()), 10, 64)
		if err != nil || parsedValue < 0 {
			return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil,
				value.String(),
				valueName,
				[]string{"int64"})
		}
		return parsedValue, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.Int() < 0 {
			return nil, common.NewOracleError(oracleErrors.InvalidConnectionParameter, nil,
				value.Int(),
				valueName,
				"positive integer")
		}
		return value.Int(), nil
	default:
		common.Odl.Debug("Type not supported")
		return nil, common.NewOracleError(
			oracleErrors.InvalidConnectionParameter,
			errors.ErrUnsupported,
			value.String(),
			valueName,
			[]string{"int64"})

	}
}

var _languageTagType = reflect.TypeOf(language.Tag{})

// validateLanguage parses a string to a valid language name
// the value is parse using language.Parse() method.
// parameters:
//   - key : property key to be parsed
//   - value : property value
//
// error:
//   - value is not assignable to string
//   - parsing has failed
func validateLanguage(value reflect.Value, valueName string) (any, error) {
	var lang language.Tag
	var err error
	if value.Kind() == reflect.String {
		if len(value.String()) != 0 {
			lang, err = language.Parse(value.String())
			if err != nil {
				common.Odl.Debug(fmt.Sprintf("Error parsing connection property [%s], value [%s]", valueName, value))
			} else {
				return lang, nil
			}
		}
	}
	// is it already a Tag ?
	if value.IsValid() && value.Type() == _languageTagType {
		return value, nil
	}

	return language.Tag{}, common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName, "one of language Tag",
	)
}

// validateLogonModeValue validator for logon mode value.
// errors:
//   - value is not assignable to a string
//   - string value cannot be parsed as a LogonMode
func validateLogonModeValue(value reflect.Value, valueName string) (any, error) {
	if value.Kind() == reflect.String {
		if len(value.String()) == 0 {
			return "", nil
		}
		mode, err := common.GetLogonModeFromString(value.String())
		if err == nil {
			return mode.String(), nil
		}
		return nil, err
	}
	return nil, common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName,
		[]string{common.KpzLogonSysdba.String(), common.KpzLogonSysoper.String()},
	)
}

// validateBooleanValue normalizes a string property value into a
// boolean. Empty or whitespace-only values resolve to false, and any value
// other than "true" or "false" results in an error.
func validateBooleanValue(value reflect.Value, valueName string) (any, error) {
	if value.Kind() == reflect.Bool {
		// nothing to do
		return value.Bool(), nil
	}
	if value.Kind() == reflect.String {
		valueToUse := value.String()
		if len(strings.TrimSpace(valueToUse)) == 0 {
			return false, nil
		}
		switch strings.ToLower(strings.TrimSpace(valueToUse)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, common.NewOracleError(
				oracleErrors.InvalidConnectionParameter,
				nil,
				valueToUse,
				valueName,
				[]string{"true", "false"},
			)
		}
	}
	return nil, common.NewOracleError(
		oracleErrors.InvalidConnectionParameter,
		nil,
		value,
		valueName,
		[]string{"true", "false"},
	)
}
