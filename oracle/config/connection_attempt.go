/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
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
	"context"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TransportOptions contains the effective network and Oracle Net settings for
// one selected remote access point.
type TransportOptions struct {
	// Protocol is the transport protocol used for this attempt.
	Protocol driverCommon.Protocol
	// Host is the resolved listener IP used for dialing, or the original hostname
	// when DNS resolution fails or returns no addresses.
	Host string
	// Port is the listener port.
	Port uint16
	// HTTPSProxy is the optional HTTP CONNECT proxy hostname.
	HTTPSProxy string
	// HTTPSProxyPort is the HTTP CONNECT proxy port.
	HTTPSProxyPort int
	// ConnectTimeout limits the complete connection establishment operation.
	ConnectTimeout time.Duration
	// TransportConnectTimeout limits opening the socket or proxy tunnel.
	TransportConnectTimeout time.Duration
	// ReceiveTimeout limits an Oracle Net receive operation.
	ReceiveTimeout time.Duration
	// SendTimeout limits an Oracle Net send operation.
	SendTimeout time.Duration
	// ExpireTime is the interval between liveness probes.
	ExpireTime time.Duration
	// ReceiveBufferSize is the socket receive buffer size in bytes.
	ReceiveBufferSize int
	// SendBufferSize is the socket send buffer size in bytes.
	SendBufferSize int
	// SDU is the Oracle Net Session Data Unit size in bytes.
	SDU int
	// UseSNI enables TLS Server Name Indication.
	UseSNI bool
	// Security contains the effective TLS and wallet options.
	Security Security
	// Compression enables Oracle Net network compression.
	Compression bool
	// CompressionLevels contains requested compression levels or algorithms.
	CompressionLevels []string
	// Enable contains the Oracle Net ENABLE value, such as BROKEN.
	Enable string
	// ConnectionIDPrefix prefixes generated Oracle Net connection identifiers.
	ConnectionIDPrefix string
	// SourceRoute indicates that the connection uses source routing.
	SourceRoute bool
	// TypeOfService contains the requested Oracle Net service type.
	TypeOfService string
	// AdditionalSettings contains preserved scalar settings from the selected
	// plan, description, address group, and access point.
	AdditionalSettings map[string]string
}

// ConnectionAttempt is an immutable, resolved attempt to connect to one
// database listener. It contains the selected address, effective transport
// options, connect data, and effective driver configuration.
type ConnectionAttempt struct {
	// transportOptions contains the resolved transport and Oracle Net settings
	// for this specific target. Its Host is the resolved IP when available, or
	// the original hostname as a fallback.
	transportOptions TransportOptions
	// effectiveConfig is the attempt-specific configuration after base and query
	// settings are combined with the selected DESCRIPTION and ADDRESS settings.
	effectiveConfig *OracleDriverConfig
	// connectionID is the identifier included in this attempt's Oracle Net
	// handshake, including any configured connection ID prefix.
	connectionID string
	// resolvedIP is the listener IP selected for this attempt, whether from DNS
	// or an IP literal. It is separate from accessPoint.Host, which retains the
	// hostname from the DSN, and is empty when DNS fails or returns no addresses.
	resolvedIP string
	// connectData contains the effective CONNECT_DATA values for this attempt,
	// with selected DESCRIPTION values taking precedence over query values.
	connectData ConnectData
	// connectDataSet records which CONNECT_DATA fields were explicitly set by
	// the query or descriptor, including fields explicitly set to empty values.
	connectDataSet map[string]bool
	// description contains the selected DESCRIPTION's scoped settings, with its
	// CONNECT_DATA replaced by the effective merged values for this attempt.
	description ConnectionDescription
	// addressGroup is the parent group (explicit ADDRESS_LIST or implicit group)
	// containing the selected endpoint. It retains group policies and its full
	// access-point list; accessPoint below identifies the one selected member.
	addressGroup ConnectionAddressGroup
	// accessPoint is the selected ADDRESS within addressGroup. It retains the
	// original DSN hostname and address-specific settings such as proxy and
	// socket buffer overrides.
	accessPoint RemoteAccessPoint
	// policy contains the resolved description-level failover, load-balance,
	// retry-count, and retry-delay behavior used to produce this attempt.
	policy attemptPolicy
	// additional contains unrecognized scalar settings preserved from the plan,
	// selected description, address group, and selected access point. Query
	// settings mapped to known config fields are held in effectiveConfig instead.
	additional map[string]string
}

// GetTransportOptions returns an independent copy of the options used to open
// the selected network transport.
//
// Input:
//   - attempt: the resolved connection attempt that owns the effective options.
//
// Output:
//   - TransportOptions: a copy, including copied slices and maps, that callers
//     can inspect without changing the attempt.
func (attempt *ConnectionAttempt) GetTransportOptions() TransportOptions {
	options := attempt.transportOptions
	options.CompressionLevels = append([]string(nil), options.CompressionLevels...)
	options.AdditionalSettings = cloneStringMap(options.AdditionalSettings)
	options.Security.AdditionalSettings = cloneStringMap(options.Security.AdditionalSettings)
	options.Security.StructuredSettings = cloneStringMap(options.Security.StructuredSettings)
	return options
}

// GetProtocol returns the transport protocol selected for this attempt.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - driverCommon.Protocol: the protocol used to contact the selected listener.
func (attempt *ConnectionAttempt) GetProtocol() driverCommon.Protocol {
	return attempt.transportOptions.Protocol
}

// GetHost returns the destination used to open the transport: the resolved IP
// when available, or the original hostname as a DNS fallback.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the destination host used to open the transport.
func (attempt *ConnectionAttempt) GetHost() string {
	return attempt.transportOptions.Host
}

// GetResolvedIP returns the IP selected for this attempt, or an empty string
// when DNS resolution failed or returned no addresses.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the selected listener IP, when one was resolved.
func (attempt *ConnectionAttempt) GetResolvedIP() string {
	return attempt.resolvedIP
}

// GetPort returns the listener port selected for this attempt.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - uint16: the destination port used to open the transport.
func (attempt *ConnectionAttempt) GetPort() uint16 {
	return attempt.transportOptions.Port
}

// GetConnectionID returns the identifier assigned to this connection attempt.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the connection ID included in the Oracle Net handshake.
func (attempt *ConnectionAttempt) GetConnectionID() string {
	return attempt.connectionID
}

// GetHTTPSProxy returns the HTTP CONNECT proxy selected for this attempt, if any.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the proxy host, or an empty string when no proxy is configured.
func (attempt *ConnectionAttempt) GetHTTPSProxy() string {
	return attempt.transportOptions.HTTPSProxy
}

// GetHTTPSProxyPort returns the selected HTTP CONNECT proxy port.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - int: the proxy port used for the tunnel, or its effective default.
func (attempt *ConnectionAttempt) GetHTTPSProxyPort() int {
	return attempt.transportOptions.HTTPSProxyPort
}

// GetConnectTimeout returns the effective connection establishment timeout.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the maximum duration for establishing the connection.
func (attempt *ConnectionAttempt) GetConnectTimeout() time.Duration {
	return attempt.transportOptions.ConnectTimeout
}

// GetTransportConnectTimeout returns the effective socket or proxy timeout.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the timeout used while opening the socket or proxy tunnel.
func (attempt *ConnectionAttempt) GetTransportConnectTimeout() time.Duration {
	return attempt.transportOptions.TransportConnectTimeout
}

// GetReceiveTimeout returns the effective Oracle Net receive timeout.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the timeout applied to Oracle Net receive operations.
func (attempt *ConnectionAttempt) GetReceiveTimeout() time.Duration {
	return attempt.transportOptions.ReceiveTimeout
}

// GetSendTimeout returns the effective Oracle Net send timeout.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the timeout applied to Oracle Net send operations.
func (attempt *ConnectionAttempt) GetSendTimeout() time.Duration {
	return attempt.transportOptions.SendTimeout
}

// GetExpireTime returns the effective network liveness probe interval.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the interval between connection liveness probes.
func (attempt *ConnectionAttempt) GetExpireTime() time.Duration {
	return attempt.transportOptions.ExpireTime
}

// GetReceiveBufferSize returns the effective socket receive buffer size.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - int: the socket receive buffer size in bytes.
func (attempt *ConnectionAttempt) GetReceiveBufferSize() int {
	return attempt.transportOptions.ReceiveBufferSize
}

// GetSendBufferSize returns the effective socket send buffer size.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - int: the socket send buffer size in bytes.
func (attempt *ConnectionAttempt) GetSendBufferSize() int {
	return attempt.transportOptions.SendBufferSize
}

// GetSDU returns the effective Oracle Net Session Data Unit size.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - int: the Session Data Unit size in bytes.
func (attempt *ConnectionAttempt) GetSDU() int {
	return attempt.transportOptions.SDU
}

// GetUseSNI reports whether TLS Server Name Indication is enabled.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - bool: true when the TLS client sends a server name indication.
func (attempt *ConnectionAttempt) GetUseSNI() bool {
	return attempt.transportOptions.UseSNI
}

// GetSecurity returns the effective TLS and wallet settings.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - Security: a copy of the TLS and wallet options, including copied maps.
func (attempt *ConnectionAttempt) GetSecurity() Security {
	security := attempt.transportOptions.Security
	security.AdditionalSettings = cloneStringMap(security.AdditionalSettings)
	security.StructuredSettings = cloneStringMap(security.StructuredSettings)
	return security
}

// GetCompression reports whether Oracle Net compression is enabled.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - bool: true when Oracle Net compression is enabled.
func (attempt *ConnectionAttempt) GetCompression() bool {
	return attempt.transportOptions.Compression
}

// GetCompressionLevels returns the requested compression levels.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - []string: a copy of the configured compression levels or algorithms.
func (attempt *ConnectionAttempt) GetCompressionLevels() []string {
	return append([]string(nil), attempt.transportOptions.CompressionLevels...)
}

// GetEnable returns the Oracle Net ENABLE value for this attempt.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the effective ENABLE descriptor value, or empty when omitted.
func (attempt *ConnectionAttempt) GetEnable() string {
	return attempt.transportOptions.Enable
}

// GetConnectionIDPrefix returns the effective Oracle Net connection ID prefix.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the prefix used when generating the Oracle Net connection ID.
func (attempt *ConnectionAttempt) GetConnectionIDPrefix() string {
	return attempt.transportOptions.ConnectionIDPrefix
}

// GetSourceRoute reports whether source routing is enabled for this attempt.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - bool: true when this attempt uses source routing.
func (attempt *ConnectionAttempt) GetSourceRoute() bool {
	return attempt.transportOptions.SourceRoute
}

// GetTypeOfService returns the selected Oracle Net service type.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the effective Oracle Net type-of-service value, or empty when omitted.
func (attempt *ConnectionAttempt) GetTypeOfService() string {
	return attempt.transportOptions.TypeOfService
}

// GetFailover reports the selected DESCRIPTION's failover policy.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - bool: true when a failed target allows trying another target or description.
func (attempt *ConnectionAttempt) GetFailover() bool {
	return attempt.policy.failover
}

// GetLoadBalance reports whether the selected DESCRIPTION's targets are load balanced.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - bool: true when candidate targets are shuffled for connection attempts.
func (attempt *ConnectionAttempt) GetLoadBalance() bool {
	return attempt.policy.loadBalance
}

// GetRetryCount returns the additional passes configured for the selected DESCRIPTION.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - int: the number of additional passes over its target addresses.
func (attempt *ConnectionAttempt) GetRetryCount() int {
	return attempt.policy.retryCount
}

// GetRetryDelay returns the delay between retry passes for the selected DESCRIPTION.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - time.Duration: the pause before the iterator starts another retry pass.
func (attempt *ConnectionAttempt) GetRetryDelay() time.Duration {
	return attempt.policy.retryDelay
}

// GetConnectData returns the selected service, instance, and DRCP settings.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - ConnectData: a copy of the effective CONNECT_DATA values and preserved
//     nested settings.
func (attempt *ConnectionAttempt) GetConnectData() ConnectData {
	return cloneConnectData(attempt.connectData)
}

// GetConnectDataSet returns a copy of the CONNECT_DATA fields explicitly set
// by the descriptor or query parameters.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - map[string]bool: a copy mapping upper-case CONNECT_DATA field names to
//     true when each field was explicitly supplied.
func (attempt *ConnectionAttempt) GetConnectDataSet() map[string]bool {
	return cloneBoolMap(attempt.connectDataSet)
}

// GetEffectiveConfig returns an independent copy of the resolved driver config.
// The copy includes values from the base config, query settings, and selected
// DESCRIPTION, ADDRESS_LIST, and ADDRESS scopes.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - *OracleDriverConfig: an independent configuration snapshot suitable for
//     initializing the connection.
func (attempt *ConnectionAttempt) GetEffectiveConfig() *OracleDriverConfig {
	return attempt.effectiveConfig.Clone()
}

// GetDescription returns a deep copy of the selected description settings.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - ConnectionDescription: the effective settings scoped to this DESCRIPTION.
func (attempt *ConnectionAttempt) GetDescription() ConnectionDescription {
	return cloneConnectionDescription(attempt.description)
}

// GetAddressGroup returns a deep copy of the selected address group.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - ConnectionAddressGroup: the effective address group containing this target.
func (attempt *ConnectionAttempt) GetAddressGroup() ConnectionAddressGroup {
	return cloneConnectionAddressGroup(attempt.addressGroup)
}

// GetAccessPoint returns a deep copy of the selected remote access point.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - RemoteAccessPoint: the endpoint selected for the network connection.
func (attempt *ConnectionAttempt) GetAccessPoint() RemoteAccessPoint {
	return cloneRemoteAccessPoint(attempt.accessPoint)
}

// GetAdditionalSettings returns a copy of preserved descriptor settings from
// the plan, description, address group, and access point. DSN query settings
// are resolved into the effective configuration and are not returned here.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - map[string]string: a copy of preserved descriptor settings not mapped to
//     typed fields.
func (attempt *ConnectionAttempt) GetAdditionalSettings() map[string]string {
	return cloneStringMap(attempt.additional)
}

// GetAddressDescriptor returns the selected ADDRESS in TNS parenthetical form.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the ADDRESS descriptor for the selected endpoint, including its
//     address-scoped proxy and preserved scalar settings.
func (attempt *ConnectionAttempt) GetAddressDescriptor() string {
	var result strings.Builder
	result.WriteString("(ADDRESS=(PROTOCOL=")
	result.WriteString(attempt.transportOptions.Protocol.String())
	result.WriteString(")(HOST=")
	result.WriteString(descriptorValue(attempt.transportOptions.Host))
	result.WriteString(")(PORT=")
	result.WriteString(strconv.Itoa(int(attempt.transportOptions.Port)))
	result.WriteString(")")
	if attempt.transportOptions.HTTPSProxy != "" {
		result.WriteString("(HTTPS_PROXY=")
		result.WriteString(descriptorValue(attempt.transportOptions.HTTPSProxy))
		result.WriteString(")")
		if attempt.transportOptions.HTTPSProxyPort > 0 {
			result.WriteString("(HTTPS_PROXY_PORT=")
			result.WriteString(strconv.Itoa(attempt.transportOptions.HTTPSProxyPort))
			result.WriteString(")")
		}
	}
	appendDescriptorMap(&result, attempt.accessPoint.AdditionalSettings)
	result.WriteString(")")
	return result.String()
}

// GetConnectDataDescriptor returns the selected CONNECT_DATA section in TNS
// parenthetical form.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: the CONNECT_DATA descriptor used to identify the service and
//     transmit DRCP and other connection settings.
func (attempt *ConnectionAttempt) GetConnectDataDescriptor() string {
	return attempt.getConnectDataDescriptor(nil)
}

// getConnectDataDescriptor serializes resolved connect data and optionally
// replaces caller-supplied session identifiers with values for this session.
//
// Input:
//   - attempt: the resolved attempt whose CONNECT_DATA should be serialized.
//   - sessionInfo: optional generated CID and CONNECTION_ID values; nil preserves
//     values from the DSN.
//
// Output:
//   - string: a parenthetical CONNECT_DATA descriptor.
func (attempt *ConnectionAttempt) getConnectDataDescriptor(sessionInfo *connectDescriptorSessionInfo) string {
	fields := []struct {
		name  string
		value string
	}{
		{"SERVICE_NAME", attempt.connectData.ServiceName},
		{"SID", attempt.connectData.SID},
		{"SERVER", attempt.connectData.Server},
		{"POOL_NAME", attempt.connectData.PoolName},
		{"POOL_CONNECTION_CLASS", attempt.connectData.PoolConnectionClass},
		{"POOL_PURITY", attempt.connectData.PoolPurity},
		{"POOL_BOUNDARY", attempt.connectData.PoolBoundary},
		{"SERVICE_TAG", attempt.connectData.ServiceTag},
		{"INSTANCE_NAME", attempt.connectData.InstanceName},
		{"FAILOVER_MODE", attempt.connectData.FailoverMode},
		{"HS", attempt.connectData.HS},
		{"RDB_DATABASE", attempt.connectData.RDBDatabase},
		{"GLOBAL_NAME", attempt.connectData.GlobalName},
		{"CONNECTION_ID_PREFIX", attempt.connectData.ConnectionIDPrefix},
	}
	var result strings.Builder
	result.WriteString("(CONNECT_DATA=")
	for _, field := range fields {
		if field.name == "FAILOVER_MODE" && len(attempt.connectData.FailoverModeSettings) > 0 {
			continue
		}
		if field.value == "" && !attempt.connectDataSet[field.name] {
			continue
		}
		result.WriteByte('(')
		result.WriteString(field.name)
		result.WriteByte('=')
		result.WriteString(descriptorValue(field.value))
		result.WriteByte(')')
	}
	if len(attempt.connectData.FailoverModeSettings) > 0 {
		result.WriteString("(FAILOVER_MODE=")
		appendDescriptorMap(&result, attempt.connectData.FailoverModeSettings)
		result.WriteByte(')')
	}
	for _, sectionName := range sortedMapKeys(attempt.connectData.StructuredSettings) {
		if sectionName == "FAILOVER_MODE" || (sessionInfo != nil && (sectionName == "CID" || sectionName == "CONNECTION_ID")) {
			continue
		}
		result.WriteByte('(')
		result.WriteString(sectionName)
		result.WriteByte('=')
		appendDescriptorMap(&result, attempt.connectData.StructuredSettings[sectionName])
		result.WriteByte(')')
	}
	if sessionInfo == nil {
		appendDescriptorMap(&result, attempt.connectData.AdditionalSettings)
	} else {
		appendDescriptorMapExcept(&result, attempt.connectData.AdditionalSettings, "CID", "CONNECTION_ID")
		result.WriteString("(CID=")
		appendDescriptorField(&result, "PROGRAM", sessionInfo.programName)
		appendDescriptorField(&result, "HOST", sessionInfo.hostName)
		appendDescriptorField(&result, "USER", sessionInfo.userName)
		result.WriteByte(')')
		appendDescriptorField(&result, "CONNECTION_ID", sessionInfo.connectionID)
	}
	result.WriteByte(')')
	return result.String()
}

// GetConnectDescriptor returns the selected ADDRESS and CONNECT_DATA wrapped
// in a DESCRIPTION suitable for the Oracle Net CONNECT packet.
//
// Input:
//   - attempt: the resolved connection attempt.
//
// Output:
//   - string: a TNS DESCRIPTION containing the selected endpoint and effective
//     connection settings for the network handshake.
func (attempt *ConnectionAttempt) GetConnectDescriptor() string {
	return attempt.getConnectDescriptor(nil)
}

// GetConnectDescriptorWithClientInfo returns the selected TNS DESCRIPTION
// with session-specific CID and CONNECTION_ID values serialized directly from
// the resolved attempt. Any corresponding values supplied in the DSN are
// replaced by the generated session values.
//
// Input:
//   - attempt: the resolved endpoint, transport options, and connect data.
//   - programName: sanitized client program name for the CID.
//   - hostName: sanitized client host name for the CID.
//   - userName: sanitized client user name for the CID.
//
// Output:
//   - string: the CONNECT packet descriptor for this attempt and session.
func (attempt *ConnectionAttempt) GetConnectDescriptorWithClientInfo(programName, hostName, userName string) string {
	return attempt.getConnectDescriptor(&connectDescriptorSessionInfo{
		connectionID: attempt.connectionID,
		programName:  programName,
		hostName:     hostName,
		userName:     userName,
	})
}

type connectDescriptorSessionInfo struct {
	// connectionID is the generated identifier for this network session.
	connectionID string
	// programName is the sanitized client program name for the CID.
	programName string
	// hostName is the sanitized client host name for the CID.
	hostName string
	// userName is the sanitized client user name for the CID.
	userName string
}

// getConnectDescriptor serializes the attempt as a TNS DESCRIPTION, optionally
// including generated session-specific connect data.
//
// Input:
//   - attempt: the resolved endpoint, policies, and transport settings.
//   - sessionInfo: optional generated CID and CONNECTION_ID values.
//
// Output:
//   - string: the DESCRIPTION for the Oracle Net CONNECT packet.
func (attempt *ConnectionAttempt) getConnectDescriptor(sessionInfo *connectDescriptorSessionInfo) string {
	var result strings.Builder
	result.WriteString("(DESCRIPTION=")
	group := attempt.addressGroup
	address := attempt.GetAddressDescriptor()
	if group.Failover != nil || group.LoadBalance != nil || group.SourceRoute != nil || len(group.AdditionalSettings) > 0 {
		result.WriteString("(ADDRESS_LIST=")
		if group.Failover != nil {
			appendDescriptorField(&result, "FAILOVER", strconv.FormatBool(*group.Failover))
		}
		if group.LoadBalance != nil {
			appendDescriptorField(&result, "LOAD_BALANCE", strconv.FormatBool(*group.LoadBalance))
		}
		if group.SourceRoute != nil {
			appendDescriptorField(&result, "SOURCE_ROUTE", strconv.FormatBool(*group.SourceRoute))
		}
		appendDescriptorMap(&result, group.AdditionalSettings)
		result.WriteString(address)
		result.WriteByte(')')
	} else {
		result.WriteString(address)
	}

	description := attempt.description
	options := attempt.transportOptions
	appendDescriptorField(&result, "FAILOVER", strconv.FormatBool(attempt.policy.failover))
	appendDescriptorField(&result, "LOAD_BALANCE", strconv.FormatBool(attempt.policy.loadBalance))
	appendDescriptorField(&result, "SOURCE_ROUTE", strconv.FormatBool(options.SourceRoute))
	if options.UseSNI || description.UseSNI != nil {
		appendDescriptorField(&result, "USE_SNI", strconv.FormatBool(options.UseSNI))
	}
	appendEffectiveDuration := func(name string, value time.Duration, explicitlySet bool) {
		if value > 0 || explicitlySet {
			appendDescriptorField(&result, name, value.String())
		}
	}
	appendEffectiveDuration("CONNECT_TIMEOUT", options.ConnectTimeout, description.ConnectTimeout != nil)
	if attempt.policy.retryCount > 0 || description.RetryCount != nil {
		appendDescriptorField(&result, "RETRY_COUNT", strconv.Itoa(attempt.policy.retryCount))
	}
	appendEffectiveDuration("RETRY_DELAY", attempt.policy.retryDelay, description.RetryDelay != nil)
	appendEffectiveDuration("TRANSPORT_CONNECT_TIMEOUT", options.TransportConnectTimeout, description.TransportConnectTimeout != nil)
	if options.ExpireTime > 0 || description.ExpireTime != nil {
		appendDescriptorField(&result, "EXPIRE_TIME", strconv.Itoa(int(options.ExpireTime/time.Minute)))
	}
	appendEffectiveDuration("RECV_TIMEOUT", options.ReceiveTimeout, description.RecvTimeout != nil)
	appendEffectiveDuration("SEND_TIMEOUT", options.SendTimeout, description.SendTimeout != nil)
	if options.SDU > 0 || description.SDU != nil {
		appendDescriptorField(&result, "SDU", strconv.Itoa(options.SDU))
	}
	if options.ReceiveBufferSize > 0 || description.RecvBufSize != nil {
		appendDescriptorField(&result, "RECV_BUF_SIZE", strconv.Itoa(options.ReceiveBufferSize))
	}
	if options.SendBufferSize > 0 || description.SendBufSize != nil {
		appendDescriptorField(&result, "SEND_BUF_SIZE", strconv.Itoa(options.SendBufferSize))
	}
	if options.Enable != "" {
		appendDescriptorField(&result, "ENABLE", options.Enable)
	}
	if options.ConnectionIDPrefix != "" {
		appendDescriptorField(&result, "CONNECTION_ID_PREFIX", options.ConnectionIDPrefix)
	}
	if options.Compression || description.Compression != nil {
		appendDescriptorField(&result, "COMPRESSION", strconv.FormatBool(options.Compression))
	}
	if len(options.CompressionLevels) > 0 {
		result.WriteString("(COMPRESSION_LEVELS=")
		for _, level := range options.CompressionLevels {
			appendDescriptorField(&result, "LEVEL", level)
		}
		result.WriteByte(')')
	}
	if options.TypeOfService != "" {
		appendDescriptorField(&result, "TYPE_OF_SERVICE", options.TypeOfService)
	}
	security := options.Security
	if security.SSLServerCertDN != "" || security.SSLServerDNMatch || security.SSLAllowWeakDNMatch ||
		security.WalletLocation != "" || security.MyWalletDirectory != "" || len(description.SecuritySet) > 0 ||
		len(security.AdditionalSettings) > 0 || len(security.StructuredSettings) > 0 {
		result.WriteString("(SECURITY=")
		if security.SSLServerCertDN != "" || description.SecuritySet["SSL_SERVER_CERT_DN"] {
			appendDescriptorField(&result, "SSL_SERVER_CERT_DN", security.SSLServerCertDN)
		}
		if security.SSLServerDNMatch || description.SecuritySet["SSL_SERVER_DN_MATCH"] {
			appendDescriptorField(&result, "SSL_SERVER_DN_MATCH", strconv.FormatBool(security.SSLServerDNMatch))
		}
		if security.SSLAllowWeakDNMatch || description.SecuritySet["SSL_ALLOW_WEAK_DN_MATCH"] {
			appendDescriptorField(&result, "SSL_ALLOW_WEAK_DN_MATCH", strconv.FormatBool(security.SSLAllowWeakDNMatch))
		}
		if security.WalletLocation != "" || description.SecuritySet["WALLET_LOCATION"] {
			walletDescriptor := ""
			if description.Security != nil {
				walletDescriptor = description.Security.StructuredSettings["WALLET_LOCATION"]
			}
			if walletDescriptor != "" {
				result.WriteString(walletDescriptor)
			} else {
				appendDescriptorField(&result, "WALLET_LOCATION", security.WalletLocation)
			}
		}
		if description.SecuritySet["MY_WALLET_DIRECTORY"] {
			appendDescriptorField(&result, "MY_WALLET_DIRECTORY", security.MyWalletDirectory)
		}
		if description.Security != nil {
			appendDescriptorMap(&result, description.Security.AdditionalSettings)
			for _, key := range sortedMapKeys(description.Security.StructuredSettings) {
				if key != "WALLET_LOCATION" {
					result.WriteString(description.Security.StructuredSettings[key])
				}
			}
		}
		result.WriteByte(')')
	}
	appendDescriptorMap(&result, description.AdditionalSettings)
	result.WriteString(attempt.getConnectDataDescriptor(sessionInfo))
	result.WriteByte(')')
	return result.String()
}

// appendDescriptorField appends one named TNS value to a descriptor builder.
//
// Input:
//   - result: the builder receiving the descriptor field.
//   - name: the upper-case descriptor parameter name.
//   - value: the scalar value to encode, quoted when TNS syntax requires it.
//
// Output:
//   - None. The field is appended to result in (NAME=value) form.
func appendDescriptorField(result *strings.Builder, name, value string) {
	result.WriteByte('(')
	result.WriteString(name)
	result.WriteByte('=')
	result.WriteString(descriptorValue(value))
	result.WriteByte(')')
}

// appendDescriptorMap appends scalar descriptor fields in stable key order.
//
// Input:
//   - result: the builder receiving the fields.
//   - values: descriptor names and scalar values to serialize.
//
// Output:
//   - None. Each map entry is appended as a TNS field to result.
func appendDescriptorMap(result *strings.Builder, values map[string]string) {
	for _, key := range sortedMapKeys(values) {
		appendDescriptorField(result, key, values[key])
	}
}

// appendDescriptorMapExcept appends descriptor values in stable key order,
// omitting fields whose values are generated for the current session.
//
// Input:
//   - result: builder receiving the fields.
//   - values: field names and values to serialize.
//   - excludedNames: field names omitted from the serialized output.
//
// Output:
//   - None. All non-excluded values are appended in key order.
func appendDescriptorMapExcept(result *strings.Builder, values map[string]string, excludedNames ...string) {
	for _, key := range sortedMapKeys(values) {
		excluded := false
		for _, excludedName := range excludedNames {
			if strings.EqualFold(key, excludedName) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		appendDescriptorField(result, key, values[key])
	}
}

// sortedMapKeys returns map keys in lexical order for deterministic output.
//
// Input:
//   - values: the map whose keys will be ordered.
//
// Output:
//   - []string: a newly allocated slice containing the map keys in ascending order.
func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// cloneNestedStringMap copies a string map and each nested string map.
//
// Input:
//   - source: the nested map to copy.
//
// Output:
//   - map[string]map[string]string: an independent deep copy, or nil when source
//     is empty.
func cloneNestedStringMap(source map[string]map[string]string) map[string]map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]map[string]string, len(source))
	for key, values := range source {
		clone[key] = cloneStringMap(values)
	}
	return clone
}

// cloneConnectData deep-copies CONNECT_DATA values and their preserved sections.
//
// Input:
//   - source: resolved or parsed connection data to copy.
//
// Output:
//   - ConnectData: an independent value with copied maps.
func cloneConnectData(source ConnectData) ConnectData {
	clone := source
	clone.FailoverModeSettings = cloneStringMap(source.FailoverModeSettings)
	clone.AdditionalSettings = cloneStringMap(source.AdditionalSettings)
	clone.StructuredSettings = cloneNestedStringMap(source.StructuredSettings)
	return clone
}

// ConnectionAttemptIterator walks DESCRIPTION alternatives, address groups,
// remote access points, and retry cycles for one ConnectionPlans value. It
// resolves candidate IPs during construction so DESCRIPTION ordering can
// account for cached down hosts, then resolves each effective configuration
// only when that DESCRIPTION is reached.
type ConnectionAttemptIterator struct {
	// ctx is checked before each attempt and while waiting between retry cycles.
	ctx context.Context
	// plans supplies the immutable parsed descriptor and query settings.
	plans *ConnectionPlans
	// baseConfig is a snapshot of the globally resolved BaseConfig.
	baseConfig *OracleDriverConfig
	// connectionID is generated by the driver for this database/sql connection
	// and copied into each target attempt before it reaches the network layer.
	connectionID string
	// queryConnectData contains configuration and query CONNECT_DATA values
	// already merged before the iterator starts selecting targets.
	queryConnectData ConnectData
	// queryConnectDataSet records query-supplied CONNECT_DATA field names.
	queryConnectDataSet map[string]bool
	// descriptionCursors contains each selected description's resolved target
	// order. Descriptions with only cached-down targets are kept at the end.
	descriptionCursors []*descriptionCursor
	// currentDescription is the next cursor to activate.
	currentDescription int
	// current holds the description currently being attempted.
	current *descriptionCursor
	// random shuffles descriptions, groups, and addresses when load balancing is enabled.
	random *rand.Rand
}

// descriptionCursor tracks one description's endpoint order and retry state.
type descriptionCursor struct {
	// description contains settings parsed from one DESCRIPTION.
	description ConnectionDescription
	// effectiveConfig contains base, query, and description settings. It stays
	// nil until this description becomes current in the iterator.
	effectiveConfig *OracleDriverConfig
	// policy contains resolved failover, load-balance, and retry values.
	policy attemptPolicy
	// targets are the selected remote access points in their current attempt order.
	targets []attemptTarget
	// nextTarget is the next target index in the current retry cycle.
	nextTarget int
	// retryCycle counts completed additional passes over targets.
	retryCycle int
}

// attemptPolicy contains the resolved iteration controls for one description.
type attemptPolicy struct {
	// failover indicates whether later addresses are tried after a failure.
	failover bool
	// loadBalance indicates whether the initial address order is shuffled and
	// retry passes rotate through that order.
	loadBalance bool
	// sourceRoute records the description's source-route setting.
	sourceRoute bool
	// retryCount is the number of additional passes over the selected addresses.
	retryCount int
	// retryDelay is the delay between retry passes.
	retryDelay time.Duration
}

// attemptTarget identifies one access point and its address-group settings.
type attemptTarget struct {
	// group is the source address group.
	group ConnectionAddressGroup
	// accessPoint is the selected endpoint.
	accessPoint RemoteAccessPoint
	// resolvedIP is the specific listener IP for this target; accessPoint.Host
	// retains the hostname supplied by the DSN.
	resolvedIP string
	// sourceRoute is the effective value at the selected address-group scope.
	sourceRoute bool
}

// dialHost returns the resolved IP when available, falling back to the original
// hostname when DNS lookup failed or produced no addresses.
func (target attemptTarget) dialHost() string {
	if target.resolvedIP != "" {
		return target.resolvedIP
	}
	return target.accessPoint.Host
}

// NewConnectionAttemptIterator creates an iterator over plans whose global
// query settings have already been merged into BaseConfig. It resolves the
// candidate IPs for selected descriptions up front to preserve down-host
// ordering, then applies each DESCRIPTION's effective configuration only when
// that description is reached. Address settings and the connection ID are
// applied to each yielded attempt.
//
// Input:
//   - plans: parsed connection alternatives, base configuration, and query settings.
//   - ctx: context used to cancel iteration or retry-delay waits; nil uses a
//     background context.
//   - connectionID: identifier generated by the driver for this connection.
//
// Output:
//   - *ConnectionAttemptIterator: an iterator that yields one fully configured
//     attempt at a time.
//   - error: missing plans, base configuration, or connection ID; or context
//     cancellation while pre-resolving selected description targets.
func (plans *ConnectionPlans) NewConnectionAttemptIterator(ctx context.Context, connectionID string) (*ConnectionAttemptIterator, error) {
	if plans == nil || plans.BaseConfig == nil {
		return nil, fmt.Errorf("connection plans and base config are required")
	}
	if connectionID == "" {
		return nil, fmt.Errorf("connection ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Step 1: use the already merged base configuration directly. HandleDsn
	// validates it; copies are made later when description and target settings
	// need independent effective configurations.
	baseConfig := plans.BaseConfig
	queryConnectData := connectDataFromConfig(baseConfig.ConnectionProperties)

	// Step 2: choose DESCRIPTION order from the plan-level policy, falling back
	// to the resolved base configuration when the descriptor omits that policy.
	loadBalance := baseConfig.ConnectionProperties.LoadBalance
	if plans.Plan.LoadBalance != nil {
		loadBalance = *plans.Plan.LoadBalance
	}
	failover := baseConfig.ConnectionProperties.Failover
	if plans.Plan.Failover != nil {
		failover = *plans.Plan.Failover
	}

	descriptionOrder := make([]int, len(plans.Plan.Descriptions))
	for i := range descriptionOrder {
		descriptionOrder[i] = i
	}
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	if loadBalance {
		random.Shuffle(len(descriptionOrder), func(i, j int) {
			descriptionOrder[i], descriptionOrder[j] = descriptionOrder[j], descriptionOrder[i]
		})
	}
	if !failover && len(descriptionOrder) > 1 {
		descriptionOrder = descriptionOrder[:1]
	}

	// Step 3: resolve candidate targets for each selected DESCRIPTION. This
	// mirrors the old iterator's host-ordering pass without creating attempts or
	// applying DESCRIPTION configuration before that description is reached.
	iterator := &ConnectionAttemptIterator{
		ctx:                 ctx,
		plans:               plans,
		baseConfig:          baseConfig,
		connectionID:        connectionID,
		queryConnectData:    queryConnectData,
		queryConnectDataSet: cloneBoolMap(plans.queryConnectDataSet),
		random:              random,
	}
	for _, index := range descriptionOrder {
		cursor, err := iterator.newDescriptionCursor(index)
		if err != nil {
			return nil, err
		}
		if len(cursor.targets) > 0 {
			iterator.descriptionCursors = append(iterator.descriptionCursors, cursor)
		}
	}
	reorderDescriptionCursorsByDownHostStatus(iterator.descriptionCursors)
	return iterator, nil
}

// Next returns the next resolved connection attempt. It returns io.EOF after
// all descriptions and retry cycles are exhausted, or the context error if the
// caller cancels before an attempt or while the iterator is waiting between
// retry cycles.
//
// Input:
//   - iterator: the connection-plan iterator tracking description and retry state.
//
// Output:
//   - *ConnectionAttempt: the next endpoint with all settings resolved.
//   - error: io.EOF when there are no more targets, the context error on
//     cancellation, or a configuration error while activating a description or
//     resolving a target.
func (iterator *ConnectionAttemptIterator) Next() (*ConnectionAttempt, error) {
	for {
		// Check cancellation before resolving or yielding another target.
		if err := iterator.ctx.Err(); err != nil {
			return nil, err
		}
		if iterator.current == nil {
			if iterator.currentDescription >= len(iterator.descriptionCursors) {
				return nil, io.EOF
			}
			// Begin the next DESCRIPTION only after the previous one has exhausted
			// its addresses and retry cycles. Its effective config is loaded here,
			// while the already-resolved IP list is reused from construction.
			cursor := iterator.descriptionCursors[iterator.currentDescription]
			iterator.currentDescription++
			if err := iterator.resolveDescriptionConfig(cursor); err != nil {
				return nil, err
			}
			iterator.current = cursor
		}

		cursor := iterator.current
		if cursor.nextTarget < len(cursor.targets) {
			// Apply endpoint-specific settings now so each returned attempt has its
			// own config snapshot with address-specific overrides applied.
			target := cursor.targets[cursor.nextTarget]
			cursor.nextTarget++
			return iterator.resolveSelectedTarget(cursor, target)
		}

		if cursor.retryCycle >= cursor.policy.retryCount {
			iterator.current = nil
			continue
		}
		// Retry the same DESCRIPTION after its configured delay. A fresh order is
		// selected on each pass when its load-balance policy is enabled.
		if cursor.policy.retryDelay > 0 {
			timer := time.NewTimer(cursor.policy.retryDelay)
			select {
			case <-timer.C:
			case <-iterator.ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, iterator.ctx.Err()
			}
		}
		cursor.retryCycle++
		cursor.nextTarget = 0
		if cursor.policy.loadBalance && len(cursor.targets) > 1 {
			roundRobinTargets(cursor.targets)
		}
		reorderTargetsByDownHostStatus(cursor.targets)
	}
}

// newDescriptionCursor resolves a DESCRIPTION's policy and candidate endpoint
// order. It does not build the effective configuration; Next does that when
// this cursor becomes current.
//
// Input:
//   - iterator: supplies the base configuration, query values, plan policy, and
//     load-balancing source.
//   - index: index of the DESCRIPTION in the original plan.
//
// Output:
//   - *descriptionCursor: description settings, retry policy, and ordered
//     candidate endpoints. Its effective configuration is loaded on demand.
//   - error: context cancellation while resolving candidate endpoints.
func (iterator *ConnectionAttemptIterator) newDescriptionCursor(index int) (*descriptionCursor, error) {
	source := iterator.plans.Plan.Descriptions[index]
	description := cloneConnectionDescription(source)
	// Step 1: resolve retry and address-selection policy from validated base
	// defaults, then apply explicit DESCRIPTION values over those defaults.
	policy := attemptPolicy{
		failover:    iterator.baseConfig.ConnectionProperties.Failover,
		loadBalance: iterator.baseConfig.ConnectionProperties.LoadBalance,
		sourceRoute: iterator.baseConfig.ConnectionProperties.SourceRoute,
		retryCount:  iterator.baseConfig.ConnectionProperties.RetryCount,
		retryDelay:  time.Duration(iterator.baseConfig.ConnectionProperties.RetryDelay) * time.Second,
	}
	if iterator.plans.Plan.SourceRoute != nil {
		policy.sourceRoute = *iterator.plans.Plan.SourceRoute
	}
	if source.Failover != nil {
		policy.failover = *source.Failover
	}
	if source.LoadBalance != nil {
		policy.loadBalance = *source.LoadBalance
	}
	if source.SourceRoute != nil {
		policy.sourceRoute = *source.SourceRoute
	}
	if source.RetryCount != nil {
		policy.retryCount = *source.RetryCount
	}
	if source.RetryDelay != nil {
		policy.retryDelay = *source.RetryDelay
	}
	// Step 2: flatten address groups into candidate targets while honoring each
	// group's load-balance and failover settings.
	groups := source.AddressGroups
	if len(groups) == 0 && len(iterator.plans.Plan.AddressGroups) > 0 {
		groups = iterator.plans.Plan.AddressGroups
	}
	targets := make([]attemptTarget, 0)
	for _, group := range groups {
		groupTargets := make([]attemptTarget, 0, len(group.AccessPoints))
		groupSourceRoute := policy.sourceRoute
		if group.SourceRoute != nil {
			groupSourceRoute = *group.SourceRoute
		}
		for _, accessPoint := range group.AccessPoints {
			resolvedTargets, err := iterator.resolveAccessPoint(group, accessPoint, groupSourceRoute)
			if err != nil {
				return nil, err
			}
			groupTargets = append(groupTargets, resolvedTargets...)
		}
		groupLoadBalance := policy.loadBalance
		if group.LoadBalance != nil {
			groupLoadBalance = *group.LoadBalance
		}
		groupFailover := policy.failover
		if group.Failover != nil {
			groupFailover = *group.Failover
		}
		if groupLoadBalance && !policy.loadBalance && len(groupTargets) > 1 {
			iterator.shuffleTargets(groupTargets)
		}
		if !groupFailover && len(groupTargets) > 1 {
			groupTargets = groupTargets[:1]
		}
		targets = append(targets, groupTargets...)
	}
	if policy.loadBalance && len(targets) > 1 {
		iterator.shuffleTargets(targets)
	}
	if !policy.failover && len(targets) > 1 {
		targets = targets[:1]
	}
	reorderTargetsByDownHostStatus(targets)

	return &descriptionCursor{
		description: description,
		policy:      policy,
		targets:     targets,
	}, nil
}

// resolveDescriptionConfig applies and validates a DESCRIPTION's effective
// configuration the first time its cursor becomes current.
//
// Input:
//   - cursor: prepared description cursor whose target IPs and iteration policy
//     are already resolved.
//
// Output:
//   - error: configuration validation failure or invalid retry settings.
func (iterator *ConnectionAttemptIterator) resolveDescriptionConfig(cursor *descriptionCursor) error {
	if cursor.effectiveConfig != nil {
		return nil
	}
	config := iterator.baseConfig.Clone()
	applyDescriptionConfig(config, cursor.description)
	if err := config.Validate(); err != nil {
		return err
	}
	if cursor.policy.retryCount < 0 {
		return fmt.Errorf("retry count cannot be negative")
	}
	if cursor.policy.retryDelay < 0 {
		return fmt.Errorf("retry delay cannot be negative")
	}
	cursor.effectiveConfig = config
	return nil
}

// resolveAccessPoint expands one parsed access point into one attempt target
// per resolved IP address. It retains the input hostname for descriptor and
// endpoint metadata, and uses the hostname only when resolution fails or finds
// no addresses.
//
// Input:
//   - iterator: supplies the lookup context and shared DNS resolver.
//   - group: source address group copied into each generated target.
//   - accessPoint: parsed listener endpoint to resolve.
//   - sourceRoute: effective source-route setting for this group.
//
// Output:
//   - []attemptTarget: one target per resolved IP, or one hostname fallback.
//   - error: context cancellation while preparing the attempt list.
func (iterator *ConnectionAttemptIterator) resolveAccessPoint(group ConnectionAddressGroup, accessPoint RemoteAccessPoint, sourceRoute bool) ([]attemptTarget, error) {
	ips, err := driverCommon.ResolveHost(iterator.ctx, accessPoint.Host)
	if err != nil {
		if iterator.ctx.Err() != nil {
			return nil, iterator.ctx.Err()
		}
		common.Odl.Debug(fmt.Sprintf("DNS resolution failed for host '%s': %v", accessPoint.Host, err))
		// make sure this failure is cached
		driverCommon.MarkDownHost(accessPoint.Host)
		return []attemptTarget{newAttemptTarget(group, accessPoint, "", sourceRoute)}, nil
	}
	if len(ips) == 0 {
		common.Odl.Debug(fmt.Sprintf("DNS resolution returned no IPs for host '%s'", accessPoint.Host))
		return []attemptTarget{newAttemptTarget(group, accessPoint, "", sourceRoute)}, nil
	}

	targets := make([]attemptTarget, 0, len(ips))
	for _, ip := range ips {
		targets = append(targets, newAttemptTarget(group, accessPoint, ip, sourceRoute))
		common.Odl.Debug(fmt.Sprintf("Resolved '%s' to IP '%s'", accessPoint.Host, ip))
	}
	return targets, nil
}

// newAttemptTarget copies the source settings into one independently resolved
// target. The original hostname remains in accessPoint while resolvedIP
// identifies the actual socket destination.
func newAttemptTarget(group ConnectionAddressGroup, accessPoint RemoteAccessPoint, resolvedIP string, sourceRoute bool) attemptTarget {
	return attemptTarget{
		group:       cloneConnectionAddressGroup(group),
		accessPoint: cloneRemoteAccessPoint(accessPoint),
		resolvedIP:  resolvedIP,
		sourceRoute: sourceRoute,
	}
}

// shuffleTargets randomizes the initial order of connection attempts when the
// applicable LOAD_BALANCE setting is enabled.
func (iterator *ConnectionAttemptIterator) shuffleTargets(targets []attemptTarget) {
	iterator.random.Shuffle(len(targets), func(i, j int) {
		targets[i], targets[j] = targets[j], targets[i]
	})
}

// roundRobinTargets rotates the attempt list left once, matching the old
// iterator's ordering for each retry cycle after its initial shuffle.
func roundRobinTargets(targets []attemptTarget) {
	if len(targets) < 2 {
		return
	}
	first := targets[0]
	copy(targets, targets[1:])
	targets[len(targets)-1] = first
}

// reorderTargetsByDownHostStatus keeps cached unreachable IPs as fallbacks
// while preserving the order of healthy targets selected by load balancing.
//
// Input:
//   - targets: resolved endpoints in their current load-balance order.
//
// Output: none. The slice is reordered in place.
func reorderTargetsByDownHostStatus(targets []attemptTarget) {
	stablePartition(targets, func(target attemptTarget) bool {
		return driverCommon.IsDownHost(target.dialHost())
	})
}

// reorderDescriptionCursorsByDownHostStatus keeps descriptions with at least
// one non-cached-down target ahead of descriptions whose targets are all down.
// It preserves the selected load-balance order within both groups.
//
// Input:
//   - cursors: prepared descriptions in their selected load-balance order.
//
// Output: none. The slice is reordered in place.
func reorderDescriptionCursorsByDownHostStatus(cursors []*descriptionCursor) {
	stablePartition(cursors, func(cursor *descriptionCursor) bool {
		return allTargetsDown(cursor.targets)
	})
}

// allTargetsDown reports whether every resolved target is in the down-host
// cache. Empty target lists are not considered down.
//
// Input:
//   - targets: resolved targets belonging to one description.
//
// Output:
//   - bool: true when every target is cached as down; false for an empty list
//     or when any target is not cached as down.
func allTargetsDown(targets []attemptTarget) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !driverCommon.IsDownHost(target.dialHost()) {
			return false
		}
	}
	return true
}

// stablePartition preserves the current order while moving matching values to
// the end of the slice.
//
// Input:
//   - items: values to partition, kept in their original relative order.
//   - moveToEnd: predicate identifying values to move to the end.
//
// Output: none. The input slice is partitioned in place.
func stablePartition[T any](items []T, moveToEnd func(T) bool) {
	if len(items) < 2 {
		return
	}
	ordered := make([]T, 0, len(items))
	moveToEndItems := make([]bool, len(items))
	for index, item := range items {
		moveToEndItems[index] = moveToEnd(item)
		if !moveToEndItems[index] {
			ordered = append(ordered, item)
		}
	}
	for index, item := range items {
		if moveToEndItems[index] {
			ordered = append(ordered, item)
		}
	}
	copy(items, ordered)
}

// resolveSelectedTarget applies all settings for a selected endpoint and
// creates the immutable attempt snapshot returned by Next.
//
// Input:
//   - iterator: provides query-scoped CONNECT_DATA and plan settings.
//   - cursor: the resolved description and retry policy containing the target.
//   - target: the selected access point and its address-group overrides.
//
// Output:
//   - *ConnectionAttempt: an immutable snapshot of the effective config,
//     transport options, connect data, and target descriptor.
//   - error: invalid endpoint data or configuration validation failure.
func (iterator *ConnectionAttemptIterator) resolveSelectedTarget(cursor *descriptionCursor, target attemptTarget) (*ConnectionAttempt, error) {
	// Step 1: validate and normalize the selected endpoint before using it to
	// configure either the socket or the Oracle Net descriptor.
	protocol, err := driverCommon.NormalizeProtocol(target.accessPoint.Protocol.String())
	if err != nil {
		return nil, fmt.Errorf("invalid protocol for connection attempt: %w", err)
	}
	if target.accessPoint.Host == "" {
		return nil, fmt.Errorf("connection attempt host is empty")
	}
	if target.accessPoint.Port < 0 || target.accessPoint.Port > 65535 {
		return nil, fmt.Errorf("connection attempt port %d is outside the valid range 0..65535", target.accessPoint.Port)
	}
	target.accessPoint.Protocol = protocol
	if target.accessPoint.Port == 0 {
		target.accessPoint.Port = 1521
	}

	// Step 2: clone the resolved DESCRIPTION config and apply the selected
	// ADDRESS overrides. This snapshot is isolated from other attempts.
	config := cursor.effectiveConfig.Clone()
	applyAddressConfig(config, target.accessPoint)
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Step 3: merge query CONNECT_DATA with the selected DESCRIPTION's values;
	// the more specific descriptor values take precedence.
	connectData, connectDataSet := mergeConnectData(iterator.queryConnectData, iterator.queryConnectDataSet,
		cursor.description.ConnectData, cursor.description.ConnectDataSet)
	description := cloneConnectionDescription(cursor.description)
	description.ConnectData = &connectData
	description.ConnectDataSet = cloneBoolMap(connectDataSet)
	resolvedCursor := *cursor
	resolvedCursor.description = description

	// Step 4: build transport settings and preserve remaining scoped parameters,
	// then capture the selected endpoint and descriptor in the attempt.
	transportOptions, err := buildTransportOptions(config, &resolvedCursor, target)
	if err != nil {
		return nil, err
	}
	additional := make(map[string]string)
	mergeStringMaps(additional, iterator.plans.Plan.AdditionalSettings)
	mergeStringMaps(additional, cursor.description.AdditionalSettings)
	mergeStringMaps(additional, target.group.AdditionalSettings)
	mergeStringMaps(additional, target.accessPoint.AdditionalSettings)
	transportOptions.AdditionalSettings = cloneStringMap(additional)
	connectionID := iterator.connectionID
	if connectionID != "" && transportOptions.ConnectionIDPrefix != "" {
		connectionID = transportOptions.ConnectionIDPrefix + connectionID
	}

	attempt := &ConnectionAttempt{
		transportOptions: transportOptions,
		connectionID:     connectionID,
		resolvedIP:       target.resolvedIP,
		connectData:      connectData,
		connectDataSet:   connectDataSet,
		description:      description,
		addressGroup:     cloneConnectionAddressGroup(target.group),
		accessPoint:      cloneRemoteAccessPoint(target.accessPoint),
		policy:           cursor.policy,
		additional:       additional,
	}
	config.ConnectDescriptor = attempt.GetConnectDescriptor()
	attempt.effectiveConfig = config
	return attempt, nil
}

// connectDataFieldsFromQuerySettings records which supported query keys map to
// CONNECT_DATA fields. AssignFromMap has already applied their values to config;
// this presence set preserves explicitly supplied empty values for descriptor
// precedence and serialization.
//
// Input:
//   - config: configuration metadata used to resolve query keys.
//   - settings: normalized query parameter names and their values.
//
// Output:
//   - map[string]bool: upper-case CONNECT_DATA field names explicitly supplied
//     by the query string.
func connectDataFieldsFromQuerySettings(config *OracleDriverConfig, settings map[string]string) map[string]bool {
	fields := make(map[string]bool)
	for rawKey := range settings {
		field := config.findConfigurationField(rawKey)
		if field == nil {
			common.Odl.Warn("Ignoring unsupported DSN query setting", "setting", rawKey)
			continue
		}
		if field.element.Tag.Get("tns_section") != "CONNECT_DATA" {
			continue
		}
		if name := configurationDescriptorName(field); name != "" {
			fields[name] = true
		}
	}
	return fields
}

// configurationDescriptorName returns the canonical upper-case DSN name for a
// configuration field using its Oracle Net name or first declared alias.
//
// Input:
//   - field: metadata for the matching configuration field.
//
// Output:
//   - string: canonical DSN field name, or empty when the field has no DSN name.
func configurationDescriptorName(field *_fieldListElem) string {
	if name := strings.TrimSpace(field.element.Tag.Get("ns_name")); name != "" {
		return strings.ToUpper(name)
	}
	for _, alias := range strings.Split(field.element.Tag.Get("aliases"), ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			return strings.ToUpper(alias)
		}
	}
	return ""
}

// connectDataFromConfig copies the non-DRCP CONNECT_DATA fields stored in the
// driver configuration into the descriptor-facing ConnectData value.
//
// Input:
//   - properties: resolved connection properties for this attempt.
//
// Output:
//   - ConnectData: service, server mode, and related values used to build the
//     CONNECT_DATA section of the network descriptor.
func connectDataFromConfig(properties OracleConnectionProperties) ConnectData {
	return ConnectData{
		ServiceName:        properties.ServiceName,
		SID:                properties.SID,
		Server:             properties.ServerType,
		ServiceTag:         properties.ServiceTag,
		InstanceName:       properties.InstanceName,
		FailoverMode:       properties.FailoverMode,
		HS:                 properties.HS,
		RDBDatabase:        properties.RDBDatabase,
		GlobalName:         properties.GlobalName,
		ConnectionIDPrefix: properties.ConnectionIDPrefix,
	}
}

// applyConnectDataToConfig copies non-DRCP CONNECT_DATA values into the
// matching OracleConnectionProperties fields.
//
// Input:
//   - properties: effective configuration to update in place.
//   - connectData: resolved service, server mode, and other CONNECT_DATA values.
//
// Output:
//   - None. Matching OracleConnectionProperties fields are replaced.
func applyConnectDataToConfig(properties *OracleConnectionProperties, connectData ConnectData) {
	properties.ServiceName = connectData.ServiceName
	properties.SID = connectData.SID
	properties.ServerType = connectData.Server
	properties.ServiceTag = connectData.ServiceTag
	properties.InstanceName = connectData.InstanceName
	properties.FailoverMode = connectData.FailoverMode
	properties.HS = connectData.HS
	properties.RDBDatabase = connectData.RDBDatabase
	properties.GlobalName = connectData.GlobalName
	properties.ConnectionIDPrefix = connectData.ConnectionIDPrefix
}

// applyDescriptionConfig applies explicitly set DESCRIPTION values over the
// already-resolved base and query configuration.
//
// Input:
//   - config: effective configuration to update in place.
//   - description: settings explicitly supplied at DESCRIPTION scope.
//
// Output:
//   - None. Matching configuration properties are replaced by the description's
//     more-specific values.
func applyDescriptionConfig(config *OracleDriverConfig, description ConnectionDescription) {
	props := &config.ConnectionProperties
	if description.ConnectTimeout != nil {
		props.ConnectTimeout = durationSeconds(*description.ConnectTimeout)
	}
	if description.TransportConnectTimeout != nil {
		props.TransportConnectTimeout = durationSeconds(*description.TransportConnectTimeout)
	}
	if description.ExpireTime != nil {
		props.ExpireTime = *description.ExpireTime
	}
	if description.RetryCount != nil {
		props.RetryCount = *description.RetryCount
	}
	if description.RetryDelay != nil {
		props.RetryDelay = durationSeconds(*description.RetryDelay)
	}
	if description.SDU != nil {
		props.Sdu = *description.SDU
	}
	if description.RecvBufSize != nil {
		props.ReceiveBufSize = *description.RecvBufSize
	}
	if description.SendBufSize != nil {
		props.SendBufSize = *description.SendBufSize
	}
	if description.Compression != nil {
		props.Compression = *description.Compression
	}
	if description.UseSNI != nil {
		props.UseSNI = *description.UseSNI
	}
	if description.Failover != nil {
		props.Failover = *description.Failover
	}
	if description.LoadBalance != nil {
		props.LoadBalance = *description.LoadBalance
	}
	if description.SourceRoute != nil {
		props.SourceRoute = *description.SourceRoute
	}
	if description.RecvTimeout != nil {
		props.ReceiveTimeout = description.RecvTimeout.String()
	}
	if description.SendTimeout != nil {
		props.SendTimeout = description.SendTimeout.String()
	}
	if description.Enable != nil {
		props.Enable = *description.Enable
	}
	if len(description.CompressionLevels) > 0 {
		props.CompressionLevels = strings.Join(description.CompressionLevels, ",")
	}
	if description.TypeOfService != nil {
		props.TypeOfService = *description.TypeOfService
	}
	if description.ConnectData != nil {
		connectData, _ := mergeConnectData(connectDataFromConfig(*props), nil,
			description.ConnectData, description.ConnectDataSet)
		applyConnectDataToConfig(props, connectData)
	}
	if description.ConnectionIDPrefix != nil {
		props.ConnectionIDPrefix = *description.ConnectionIDPrefix
	}
	if description.Security == nil {
		return
	}
	security := description.Security
	if description.SecuritySet["SSL_SERVER_CERT_DN"] {
		props.SSLServerCertDN = security.SSLServerCertDN
	}
	if description.SecuritySet["SSL_SERVER_DN_MATCH"] {
		props.SSLServerDNMatch = security.SSLServerDNMatch
	}
	if description.SecuritySet["SSL_ALLOW_WEAK_DN_MATCH"] {
		props.SSLAllowWeakDNMatch = security.SSLAllowWeakDNMatch
	}
	if description.SecuritySet["WALLET_LOCATION"] {
		props.WalletLocation = security.WalletLocation
	}
	if description.SecuritySet["MY_WALLET_DIRECTORY"] {
		props.WalletLocation = security.MyWalletDirectory
	}
}

// applyAddressConfig applies endpoint-level proxy and socket-buffer overrides.
//
// Input:
//   - config: effective configuration to update in place.
//   - address: selected ADDRESS values, which override broader settings.
//
// Output:
//   - None. Proxy and buffer properties are updated when set on address.
func applyAddressConfig(config *OracleDriverConfig, address RemoteAccessPoint) {
	props := &config.ConnectionProperties
	if address.HTTPSProxy != nil {
		props.HttpsProxy = *address.HTTPSProxy
	}
	if address.HTTPSProxyPort != nil {
		props.HttpsProxyPort = *address.HTTPSProxyPort
	}
	if address.RecvBufSize != nil {
		props.ReceiveBufSize = *address.RecvBufSize
	}
	if address.SendBufSize != nil {
		props.SendBufSize = *address.SendBufSize
	}
}

// mergeConnectData overlays descriptor fields on top of query-provided connect
// data so the selected DESCRIPTION retains its scope-specific values.
//
// Input:
//   - query: CONNECT_DATA values supplied in the DSN query.
//   - querySet: query field names explicitly supplied, including explicit empty values.
//   - description: selected description's CONNECT_DATA values, or nil when absent.
//   - descriptionSet: descriptor field names explicitly supplied.
//
// Output:
//   - ConnectData: merged connect data with description fields taking precedence.
//   - map[string]bool: copied set of fields explicitly supplied by either source.
func mergeConnectData(query ConnectData, querySet map[string]bool, description *ConnectData, descriptionSet map[string]bool) (ConnectData, map[string]bool) {
	result := query
	result.FailoverModeSettings = cloneStringMap(query.FailoverModeSettings)
	result.AdditionalSettings = cloneStringMap(query.AdditionalSettings)
	result.StructuredSettings = cloneNestedStringMap(query.StructuredSettings)
	set := cloneBoolMap(querySet)
	if description == nil {
		return result, set
	}
	values := map[string]*string{
		"SERVICE_NAME":          &result.ServiceName,
		"SID":                   &result.SID,
		"SERVER":                &result.Server,
		"POOL_NAME":             &result.PoolName,
		"POOL_CONNECTION_CLASS": &result.PoolConnectionClass,
		"POOL_PURITY":           &result.PoolPurity,
		"POOL_BOUNDARY":         &result.PoolBoundary,
		"SERVICE_TAG":           &result.ServiceTag,
		"INSTANCE_NAME":         &result.InstanceName,
		"FAILOVER_MODE":         &result.FailoverMode,
		"HS":                    &result.HS,
		"RDB_DATABASE":          &result.RDBDatabase,
		"GLOBAL_NAME":           &result.GlobalName,
		"CONNECTION_ID_PREFIX":  &result.ConnectionIDPrefix,
	}
	descriptorValues := ConnectData{
		ServiceName: description.ServiceName, SID: description.SID, Server: description.Server,
		PoolName: description.PoolName, PoolConnectionClass: description.PoolConnectionClass,
		PoolPurity: description.PoolPurity, PoolBoundary: description.PoolBoundary,
		ServiceTag: description.ServiceTag, InstanceName: description.InstanceName,
		FailoverMode: description.FailoverMode, HS: description.HS,
		RDBDatabase: description.RDBDatabase, GlobalName: description.GlobalName,
		ConnectionIDPrefix: description.ConnectionIDPrefix,
	}
	valueByName := map[string]string{
		"SERVICE_NAME": descriptorValues.ServiceName, "SID": descriptorValues.SID,
		"SERVER": descriptorValues.Server, "POOL_NAME": descriptorValues.PoolName,
		"POOL_CONNECTION_CLASS": descriptorValues.PoolConnectionClass,
		"POOL_PURITY":           descriptorValues.PoolPurity, "POOL_BOUNDARY": descriptorValues.PoolBoundary,
		"SERVICE_TAG": descriptorValues.ServiceTag, "INSTANCE_NAME": descriptorValues.InstanceName,
		"FAILOVER_MODE": descriptorValues.FailoverMode, "HS": descriptorValues.HS,
		"RDB_DATABASE": descriptorValues.RDBDatabase, "GLOBAL_NAME": descriptorValues.GlobalName,
		"CONNECTION_ID_PREFIX": descriptorValues.ConnectionIDPrefix,
	}
	for name, destination := range values {
		if descriptionSet[name] || valueByName[name] != "" {
			*destination = valueByName[name]
			set[name] = true
		}
	}
	if descriptionSet["FAILOVER_MODE"] || descriptorValues.FailoverMode != "" || len(description.FailoverModeSettings) > 0 {
		result.FailoverMode = descriptorValues.FailoverMode
		result.FailoverModeSettings = cloneStringMap(description.FailoverModeSettings)
		set["FAILOVER_MODE"] = true
	}
	if len(description.AdditionalSettings) > 0 {
		if result.AdditionalSettings == nil {
			result.AdditionalSettings = make(map[string]string, len(description.AdditionalSettings))
		}
		mergeStringMaps(result.AdditionalSettings, description.AdditionalSettings)
	}
	if len(description.StructuredSettings) > 0 {
		if result.StructuredSettings == nil {
			result.StructuredSettings = make(map[string]map[string]string, len(description.StructuredSettings))
		}
		for name, values := range description.StructuredSettings {
			result.StructuredSettings[name] = cloneStringMap(values)
		}
	}
	return result, set
}

// buildTransportOptions copies resolved configuration and selected address
// overrides into the transport-facing value returned by an attempt.
//
// Input:
//   - config: effective configuration after connector, query, and description
//     values have been applied.
//   - cursor: selected description and its non-config transport settings.
//   - target: selected address and address-scoped overrides.
//
// Output:
//   - TransportOptions: complete transport settings for the selected endpoint.
//   - error: invalid duration string in the effective configuration.
func buildTransportOptions(config *OracleDriverConfig, cursor *descriptionCursor, target attemptTarget) (TransportOptions, error) {
	props := config.ConnectionProperties
	description := cursor.description
	receiveTimeout, err := parseConfiguredDuration(props.ReceiveTimeout)
	if err != nil {
		return TransportOptions{}, err
	}
	sendTimeout, err := parseConfiguredDuration(props.SendTimeout)
	if err != nil {
		return TransportOptions{}, err
	}
	// Step 1: initialize options from the resolved configuration, which already
	// contains connector defaults, query values, and DESCRIPTION overrides.
	options := TransportOptions{
		Protocol: target.accessPoint.Protocol, Host: target.dialHost(),
		Port: uint16(target.accessPoint.Port), HTTPSProxy: props.HttpsProxy,
		HTTPSProxyPort:          props.HttpsProxyPort,
		ConnectTimeout:          time.Duration(props.ConnectTimeout) * time.Second,
		TransportConnectTimeout: time.Duration(props.TransportConnectTimeout) * time.Second,
		ExpireTime:              time.Duration(props.ExpireTime) * time.Minute,
		ReceiveBufferSize:       props.ReceiveBufSize, SendBufferSize: props.SendBufSize,
		SDU: props.Sdu, UseSNI: props.UseSNI,
		ReceiveTimeout: receiveTimeout,
		SendTimeout:    sendTimeout,
		Security: Security{SSLServerCertDN: props.SSLServerCertDN,
			SSLServerDNMatch: props.SSLServerDNMatch, SSLAllowWeakDNMatch: props.SSLAllowWeakDNMatch,
			WalletLocation: props.WalletLocation, MyWalletDirectory: props.WalletLocation},
		Compression: props.Compression, Enable: props.Enable,
		CompressionLevels:  parseStringList(props.CompressionLevels),
		ConnectionIDPrefix: props.ConnectionIDPrefix,
		SourceRoute:        target.sourceRoute,
		TypeOfService:      props.TypeOfService,
	}
	if props.HttpsProxyPort <= 0 {
		options.HTTPSProxyPort = 80
	}
	// Step 2: apply explicitly supplied DESCRIPTION values over the base config.
	if description.ConnectTimeout != nil {
		options.ConnectTimeout = *description.ConnectTimeout
	}
	if description.TransportConnectTimeout != nil {
		options.TransportConnectTimeout = *description.TransportConnectTimeout
	}
	if description.RecvTimeout != nil {
		options.ReceiveTimeout = *description.RecvTimeout
	}
	if description.SendTimeout != nil {
		options.SendTimeout = *description.SendTimeout
	}
	if description.ExpireTime != nil {
		options.ExpireTime = time.Duration(*description.ExpireTime) * time.Minute
	}
	if description.RecvBufSize != nil {
		options.ReceiveBufferSize = *description.RecvBufSize
	}
	if description.SendBufSize != nil {
		options.SendBufferSize = *description.SendBufSize
	}
	if description.SDU != nil {
		options.SDU = *description.SDU
	}
	if description.UseSNI != nil {
		options.UseSNI = *description.UseSNI
	}
	if description.Compression != nil {
		options.Compression = *description.Compression
	}
	if len(description.CompressionLevels) > 0 {
		options.CompressionLevels = append([]string(nil), description.CompressionLevels...)
	}
	if description.ConnectionIDPrefix != nil {
		options.ConnectionIDPrefix = *description.ConnectionIDPrefix
	}
	if description.Enable != nil {
		options.Enable = *description.Enable
	}
	if description.TypeOfService != nil {
		options.TypeOfService = *description.TypeOfService
	}
	if options.ConnectionIDPrefix == "" && cursor.description.ConnectData != nil {
		options.ConnectionIDPrefix = cursor.description.ConnectData.ConnectionIDPrefix
	}
	if description.Security != nil {
		security := description.Security
		options.Security.AdditionalSettings = cloneStringMap(security.AdditionalSettings)
		options.Security.StructuredSettings = cloneStringMap(security.StructuredSettings)
		if description.SecuritySet["SSL_SERVER_CERT_DN"] {
			options.Security.SSLServerCertDN = security.SSLServerCertDN
		}
		if description.SecuritySet["SSL_SERVER_DN_MATCH"] {
			options.Security.SSLServerDNMatch = security.SSLServerDNMatch
		}
		if description.SecuritySet["SSL_ALLOW_WEAK_DN_MATCH"] {
			options.Security.SSLAllowWeakDNMatch = security.SSLAllowWeakDNMatch
		}
		if description.SecuritySet["WALLET_LOCATION"] {
			options.Security.WalletLocation = security.WalletLocation
			options.Security.MyWalletDirectory = security.WalletLocation
		}
		if description.SecuritySet["MY_WALLET_DIRECTORY"] {
			options.Security.WalletLocation = security.MyWalletDirectory
			options.Security.MyWalletDirectory = security.MyWalletDirectory
		}
	}
	// Step 3: apply ADDRESS-scoped proxy and buffer settings last because they
	// are the most specific values for this transport endpoint.
	if target.accessPoint.HTTPSProxy != nil {
		options.HTTPSProxy = *target.accessPoint.HTTPSProxy
	}
	if target.accessPoint.HTTPSProxyPort != nil {
		options.HTTPSProxyPort = *target.accessPoint.HTTPSProxyPort
	}
	if target.accessPoint.RecvBufSize != nil {
		options.ReceiveBufferSize = *target.accessPoint.RecvBufSize
	}
	if target.accessPoint.SendBufSize != nil {
		options.SendBufferSize = *target.accessPoint.SendBufSize
	}
	return options, nil
}

// parseConfiguredDuration parses an optional duration stored in configuration.
//
// Input:
//   - value: validated duration text, or an empty string when the setting is unset.
//
// Output:
//   - time.Duration: parsed duration, or zero when value is empty.
//   - error: invalid duration text.
func parseConfiguredDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	return parseDescriptorDuration(value)
}

// durationSeconds converts a duration to the integer seconds accepted by the
// public OracleDriverConfig fields.
//
// Input:
//   - value: duration to convert.
//
// Output:
//   - int: whole seconds; any fractional second is truncated.
func durationSeconds(value time.Duration) int {
	return int(value / time.Second)
}

// durationPointer creates a pointer to an independent duration value.
//
// Input:
//   - value: duration to copy.
//
// Output:
//   - *time.Duration: pointer to a newly copied duration.
func durationPointer(value time.Duration) *time.Duration {
	copy := value
	return &copy
}

// descriptionString returns an empty string for an omitted string option.
//
// Input:
//   - value: optional string from a description.
//
// Output:
//   - string: the pointed-to value, or an empty string when value is nil.
func descriptionString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// descriptorValue quotes TNS values that contain separators or whitespace.
//
// Input:
//   - value: scalar descriptor value to encode.
//
// Output:
//   - string: value in TNS-safe form, with embedded quotes escaped when needed.
func descriptorValue(value string) string {
	if strings.ContainsAny(value, " \t\r\n(),=") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

// cloneConnectionDescription returns an independent copy of pointer, map, and
// slice fields within description.
//
// Input:
//   - description: description values and nested settings to copy.
//
// Output:
//   - ConnectionDescription: a copy whose pointer, map, slice, and nested
//     structure fields do not alias the source.
func cloneConnectionDescription(description ConnectionDescription) ConnectionDescription {
	clone := description
	clone.Failover = cloneBoolPointer(description.Failover)
	clone.LoadBalance = cloneBoolPointer(description.LoadBalance)
	clone.SourceRoute = cloneBoolPointer(description.SourceRoute)
	clone.UseSNI = cloneBoolPointer(description.UseSNI)
	clone.ConnectTimeout = cloneDurationPointer(description.ConnectTimeout)
	clone.RetryCount = cloneIntPointer(description.RetryCount)
	clone.RetryDelay = cloneDurationPointer(description.RetryDelay)
	clone.TransportConnectTimeout = cloneDurationPointer(description.TransportConnectTimeout)
	clone.ExpireTime = cloneIntPointer(description.ExpireTime)
	clone.RecvTimeout = cloneDurationPointer(description.RecvTimeout)
	clone.SendTimeout = cloneDurationPointer(description.SendTimeout)
	clone.SDU = cloneIntPointer(description.SDU)
	clone.RecvBufSize = cloneIntPointer(description.RecvBufSize)
	clone.SendBufSize = cloneIntPointer(description.SendBufSize)
	clone.Enable = cloneStringPointer(description.Enable)
	clone.ConnectionIDPrefix = cloneStringPointer(description.ConnectionIDPrefix)
	clone.Compression = cloneBoolPointer(description.Compression)
	clone.CompressionLevels = append([]string(nil), description.CompressionLevels...)
	clone.SecuritySet = cloneBoolMap(description.SecuritySet)
	if description.Security != nil {
		security := *description.Security
		security.AdditionalSettings = cloneStringMap(description.Security.AdditionalSettings)
		security.StructuredSettings = cloneStringMap(description.Security.StructuredSettings)
		clone.Security = &security
	}
	clone.ConnectDataSet = cloneBoolMap(description.ConnectDataSet)
	if description.ConnectData != nil {
		connectData := *description.ConnectData
		connectData.FailoverModeSettings = cloneStringMap(description.ConnectData.FailoverModeSettings)
		connectData.AdditionalSettings = cloneStringMap(description.ConnectData.AdditionalSettings)
		connectData.StructuredSettings = cloneNestedStringMap(description.ConnectData.StructuredSettings)
		clone.ConnectData = &connectData
	}
	clone.AddressGroups = make([]ConnectionAddressGroup, len(description.AddressGroups))
	for i := range description.AddressGroups {
		clone.AddressGroups[i] = cloneConnectionAddressGroup(description.AddressGroups[i])
	}
	clone.AdditionalSettings = cloneStringMap(description.AdditionalSettings)
	return clone
}

// cloneConnectionAddressGroup returns an independent copy of an address group.
//
// Input:
//   - group: address group, including access points and policy fields, to copy.
//
// Output:
//   - ConnectionAddressGroup: a copy with independent pointers, slices, and maps.
func cloneConnectionAddressGroup(group ConnectionAddressGroup) ConnectionAddressGroup {
	clone := group
	clone.Failover = cloneBoolPointer(group.Failover)
	clone.LoadBalance = cloneBoolPointer(group.LoadBalance)
	clone.SourceRoute = cloneBoolPointer(group.SourceRoute)
	clone.AccessPoints = make([]RemoteAccessPoint, len(group.AccessPoints))
	for i := range group.AccessPoints {
		clone.AccessPoints[i] = cloneRemoteAccessPoint(group.AccessPoints[i])
	}
	clone.AdditionalSettings = cloneStringMap(group.AdditionalSettings)
	return clone
}

// cloneRemoteAccessPoint returns an independent copy of a remote access point.
//
// Input:
//   - accessPoint: endpoint and address-scoped settings to copy.
//
// Output:
//   - RemoteAccessPoint: a copy with independent optional values and settings map.
func cloneRemoteAccessPoint(accessPoint RemoteAccessPoint) RemoteAccessPoint {
	clone := accessPoint
	clone.HTTPSProxy = cloneStringPointer(accessPoint.HTTPSProxy)
	clone.HTTPSProxyPort = cloneIntPointer(accessPoint.HTTPSProxyPort)
	clone.RecvBufSize = cloneIntPointer(accessPoint.RecvBufSize)
	clone.SendBufSize = cloneIntPointer(accessPoint.SendBufSize)
	clone.AdditionalSettings = cloneStringMap(accessPoint.AdditionalSettings)
	return clone
}

// cloneStringMap copies a string map and returns nil for an empty input.
//
// Input:
//   - source: string map to copy.
//
// Output:
//   - map[string]string: an independent copy, or nil when source is empty.
func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// cloneBoolMap copies a bool map and returns an initialized empty map when the
// input is nil or empty so callers can safely add entries to the copy.
//
// Input:
//   - source: bool map to copy.
//
// Output:
//   - map[string]bool: an independent, non-nil copy, including when source is nil
//     or empty.
func cloneBoolMap(source map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// mergeStringMaps copies each non-empty source map into destination in order.
// Later source maps replace values for keys already copied from earlier maps.
//
// Input:
//   - destination: initialized map that receives the merged entries.
//   - sources: maps to copy, ordered from lower to higher precedence.
//
// Output:
//   - None. destination is updated in place.
func mergeStringMaps(destination map[string]string, sources ...map[string]string) {
	for _, source := range sources {
		for key, value := range source {
			destination[key] = value
		}
	}
}

// cloneBoolPointer copies an optional bool value.
//
// Input:
//   - value: optional bool pointer to copy.
//
// Output:
//   - *bool: an independent pointer to the same bool value, or nil when omitted.
func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// cloneIntPointer copies an optional integer value.
//
// Input:
//   - value: optional integer pointer to copy.
//
// Output:
//   - *int: an independent pointer to the same integer value, or nil when omitted.
func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// cloneDurationPointer copies an optional duration value.
//
// Input:
//   - value: optional duration pointer to copy.
//
// Output:
//   - *time.Duration: an independent pointer to the same duration, or nil when omitted.
func cloneDurationPointer(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// cloneStringPointer copies an optional string value.
//
// Input:
//   - value: optional string pointer to copy.
//
// Output:
//   - *string: an independent pointer to the same string, or nil when omitted.
func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
