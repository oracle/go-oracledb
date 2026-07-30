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

package naming

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-driver/driver/common"
)

// ConnectionContext represents the extracted business-level connection information
type ConnectionContext struct {
	// fill one of the below based on root node name
	DescriptionList *DescriptionList
	Description     *Description
	Addresses       []Address
}

// DescriptionList contains multiple connection descriptions with list-level settings
type DescriptionList struct {
	LoadBalance  bool
	Failover     bool
	SourceRoute  bool
	Descriptions []Description
}

func NewDescriptionList() *DescriptionList {
	// fill default values
	return &DescriptionList{
		LoadBalance: false,
		Failover:    true,
		SourceRoute: false,
	}
}

// Description represents a single connection descriptor with timeouts and retry settings
type Description struct {
	// Behavioral parameters
	SourceRoute bool
	LoadBalance bool
	Failover    bool
	UseSNI      bool

	// Connection parameters
	ConnectTimeout          int
	RetryCount              int
	RetryDelay              int
	TransportConnectTimeout int
	ExpireTime              int
	RecvTimeout             int
	SendTimeout             int
	SDU                     int
	RecvBufSize             int
	SendBufSize             int
	Enable                  string
	ConnectionIDPrefix      string

	// Compression parameters
	Compression       bool
	CompressionLevels []string

	// Security parameters
	Security Security

	// Address information - supports multiple ADDRESS_LIST nodes
	AddressLists []AddressList // Multiple ADDRESS_LIST nodes
	Addresses    []Address     // Direct ADDRESS children (if no ADDRESS_LIST)

	// Database connection data
	ConnectData ConnectData
}

func NewDescription() *Description {
	return &Description{
		SourceRoute:             false,
		LoadBalance:             false,
		Failover:                true,
		UseSNI:                  false,
		ConnectTimeout:          0,
		RetryCount:              0,
		RetryDelay:              0,
		TransportConnectTimeout: 0,
		ExpireTime:              0,
		RecvTimeout:             0,
		SendTimeout:             0,
		SDU:                     0,
		RecvBufSize:             0,
		SendBufSize:             0,
		Enable:                  "",
		ConnectionIDPrefix:      "",
		Compression:             false,
		Security:                *NewSecurity(),
	}
}

// AddressList contains multiple addresses with list-level load balancing settings
type AddressList struct {
	LoadBalance bool
	Failover    bool
	SourceRoute bool
	Addresses   []Address
}

func NewAddressList() *AddressList {
	return &AddressList{
		LoadBalance: false,
		Failover:    true,
		SourceRoute: false,
	}
}

// Address represents a single protocol endpoint
type Address struct {
	Protocol common.Protocol
	Host     string
	Port     uint16
	// OriginHost preserves the original hostname in redirection scenarios,
	// used as a fallback for SSL/TLS certificate hostname verification.
	OriginHost string
	// ResolvedIP contains the actual IP address to connect to.
	// For hostnames, this is populated by DNS resolution.
	// For direct IP addresses, this equals Host.
	// The Host field preserves the original hostname for SNI/certificate validation.
	ResolvedIP string
}

func (a Address) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}
func NewAddress() *Address {
	return &Address{
		Protocol:   common.ProtocolTCP,
		Host:       "",
		Port:       0,
		OriginHost: "",
		ResolvedIP: "",
	}
}

// ConnectData contains database identification and connection parameters
type ConnectData struct {
	ServiceName        string
	SID                string
	Server             string
	InstanceName       string
	FailoverMode       string
	HS                 string
	RDBDatabase        string
	GlobalName         string
	ConnectionIDPrefix string
}

// Security with SSL/TLS security parameters
type Security struct {
	SSLServerCertDN     string
	SSLServerDNMatch    bool
	SSLAllowWeakDNMatch bool
	WalletLocation      string
	MyWalletDirectory   string
}

func NewSecurity() *Security {
	return &Security{
		SSLServerCertDN:     "",
		SSLServerDNMatch:    true,
		SSLAllowWeakDNMatch: false,
		WalletLocation:      "",
		MyWalletDirectory:   "",
	}
}

// ExtractConnectionContext converts the parsed node tree into business-level context
func ExtractConnectionContext(root *Node) (*ConnectionContext, error) {
	if root == nil {
		return nil, common.NewOracleError(common.NamingContextError, nil, "", "root")
	}

	ctx := &ConnectionContext{}

	// Normalize root name to uppercase for case-insensitive comparison
	rootName := strings.ToUpper(root.Name)

	switch rootName {
	case "DESCRIPTION_LIST":
		descList, err := extractDescriptionList(root)
		if err != nil {
			return nil, err
		}
		ctx.DescriptionList = descList

	case "DESCRIPTION":
		desc, err := extractDescription(root)
		if err != nil {
			return nil, err
		}
		ctx.Description = desc

	case "ADDRESS_LIST":
		addrList, err := extractAddressList(root)
		if err != nil {
			return nil, err
		}
		ctx.Addresses = addrList.Addresses

	case "ADDRESS":
		addr, err := extractAddress(root)
		if err != nil {
			return nil, err
		}
		ctx.Addresses = []Address{addr}

	default:
		return nil, common.NewOracleError(common.NamingContextError, nil, root.Name, "root")
	}

	return ctx, nil
}

// extractDescriptionList processes DESCRIPTION_LIST node
func extractDescriptionList(node *Node) (*DescriptionList, error) {
	descList := NewDescriptionList()
	var parsingError error

	for _, child := range node.Children {
		//case-insensitive comparison
		name := strings.ToUpper(child.Name)

		switch name {
		case "LOAD_BALANCE":
			if descList.LoadBalance, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "FAILOVER":
			if descList.Failover, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "SOURCE_ROUTE":
			if descList.SourceRoute, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "DESCRIPTION":
			desc, err := extractDescription(&child)
			if err != nil {
				return nil, err
			}
			descList.Descriptions = append(descList.Descriptions, *desc)
		default:
			return nil, common.NewOracleError(common.NamingContextError, nil, child.Name, "DESCRIPTION_LIST")
		}
	}

	return descList, nil
}

// extractDescription processes DESCRIPTION node
func extractDescription(node *Node) (*Description, error) {
	desc := NewDescription()
	var parsingError error
	for _, child := range node.Children {
		//case-insensitive comparison
		name := strings.ToUpper(child.Name)

		switch name {
		// Behavioral parameters
		case "SOURCE_ROUTE":
			if desc.SourceRoute, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "LOAD_BALANCE":
			if desc.LoadBalance, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "FAILOVER":
			if desc.Failover, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "USE_SNI":
			if desc.UseSNI, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		// Connection timeout parameters
		case "CONNECT_TIMEOUT":
			if desc.ConnectTimeout, parsingError = parseDurationMS(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "RETRY_COUNT":
			if desc.RetryCount, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "RETRY_DELAY":
			if desc.RetryDelay, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "TRANSPORT_CONNECT_TIMEOUT":
			if desc.TransportConnectTimeout, parsingError = parseDurationMS(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "EXPIRE_TIME":
			if desc.ExpireTime, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "RECV_TIMEOUT":
			if desc.RecvTimeout, parsingError = parseDurationMS(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "SEND_TIMEOUT":
			if desc.SendTimeout, parsingError = parseDurationMS(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "SDU":
			if desc.SDU, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "RECV_BUF_SIZE":
			if desc.RecvBufSize, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "SEND_BUF_SIZE":
			if desc.SendBufSize, parsingError = parseIntField(child.Value); parsingError != nil {
				return nil, parsingError
			}
		case "ENABLE":
			desc.Enable = child.Value
		case "CONNECTION_ID_PREFIX":
			desc.ConnectionIDPrefix = child.Value

		// Compression parameters
		// **NEED TO CONFIRM THIS Below implementation??? (REVIEW)
		case "COMPRESSION":
			if desc.Compression, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "COMPRESSION_LEVELS":
			// Extract LEVEL children from COMPRESSION_LEVELS
			for _, levelChild := range child.Children {
				if strings.ToUpper(levelChild.Name) == "LEVEL" {
					desc.CompressionLevels = append(desc.CompressionLevels, levelChild.Value)
				}
			}

		// Security parameters
		case "SECURITY":
			security, err := extractSecurity(&child)
			if err != nil {
				return nil, err
			}
			desc.Security = *security

		// Address information - supports multiple ADDRESS_LIST nodes
		case "ADDRESS_LIST":
			addrList, err := extractAddressList(&child)
			if err != nil {
				return nil, err
			}
			desc.AddressLists = append(desc.AddressLists, *addrList)

		case "ADDRESS":
			addr, err := extractAddress(&child)
			if err != nil {
				return nil, err
			}
			desc.Addresses = append(desc.Addresses, addr)

		// Connection data
		case "CONNECT_DATA":
			connectData, err := extractConnectData(&child)
			if err != nil {
				return nil, err
			}
			desc.ConnectData = *connectData
		default:
			return nil, common.NewOracleError(common.NamingContextError, nil, child.Name, "DESCRIPTION")
		}
	}

	return desc, nil
}

// extractAddressList processes ADDRESS_LIST node
func extractAddressList(node *Node) (*AddressList, error) {
	addrList := NewAddressList()
	var parsingError error

	for _, child := range node.Children {
		//case-insensitive comparison
		name := strings.ToUpper(child.Name)

		switch name {
		case "LOAD_BALANCE":
			if addrList.LoadBalance, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "FAILOVER":
			if addrList.Failover, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "SOURCE_ROUTE":
			if addrList.SourceRoute, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "ADDRESS":
			addr, err := extractAddress(&child)
			if err != nil {
				return nil, err
			}
			addrList.Addresses = append(addrList.Addresses, addr)
		default:
			return nil, common.NewOracleError(common.NamingContextError, nil, child.Name, "ADDRESS_LIST")
		}
	}

	return addrList, nil
}

// extractAddress processes ADDRESS node
func extractAddress(node *Node) (Address, error) {
	addr := Address{}
	var parsingError error
	for _, child := range node.Children {

		name := strings.ToUpper(child.Name)

		switch name {
		case "PROTOCOL":
			if addr.Protocol, parsingError = common.NormalizeProtocol(child.Value); parsingError != nil {
				return Address{}, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "HOST":
			addr.Host = child.Value
		case "PORT":
			var _uint uint
			var p error
			if _uint, p = parseUIntField(child.Value); p != nil {
				return Address{}, p
			}
			addr.Port = uint16(_uint)
		default:
			return Address{}, common.NewOracleError(common.NamingContextError, nil, child.Name, "ADDRESS")
		}
	}

	return addr, nil
}

// extractConnectData processes CONNECT_DATA node
func extractConnectData(node *Node) (*ConnectData, error) {
	connectData := &ConnectData{}

	for _, child := range node.Children {
		// Normalize name to uppercase for case-insensitive comparison
		name := strings.ToUpper(child.Name)

		switch name {
		case "SERVICE_NAME":
			connectData.ServiceName = child.Value
		case "SID":
			connectData.SID = child.Value
		case "SERVER":
			connectData.Server = child.Value
		case "INSTANCE_NAME":
			connectData.InstanceName = child.Value
		case "FAILOVER_MODE":
			connectData.FailoverMode = child.Value
		case "HS":
			connectData.HS = child.Value
		case "RDB_DATABASE":
			connectData.RDBDatabase = child.Value
		case "GLOBAL_NAME":
			connectData.GlobalName = child.Value
		case "CONNECTION_ID_PREFIX":
			connectData.ConnectionIDPrefix = child.Value
		default:
			return nil, common.NewOracleError(common.NamingContextError, nil, child.Name, "CONNECT_DATA")
		}
	}

	return connectData, nil
}

// extractSecurity processes SECURITY node
func extractSecurity(node *Node) (*Security, error) {
	security := NewSecurity()
	var parsingError error
	for _, child := range node.Children {
		// Normalize name to uppercase for case-insensitive comparison
		name := strings.ToUpper(child.Name)

		switch name {
		case "SSL_SERVER_CERT_DN":
			security.SSLServerCertDN = child.Value
		case "SSL_SERVER_DN_MATCH":
			if security.SSLServerDNMatch, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "SSL_ALLOW_WEAK_DN_MATCH":
			if security.SSLAllowWeakDNMatch, parsingError = normalizeBoolean(child.Value); parsingError != nil {
				return nil, common.NewOracleError(common.NamingContextError, parsingError, child.Value, child.Name)
			}
		case "WALLET_LOCATION":
			security.WalletLocation = child.Value
		case "MY_WALLET_DIRECTORY":
			security.MyWalletDirectory = child.Value
		default:
			return nil, common.NewOracleError(common.NamingContextError, nil, child.Name, "SECURITY")
		}
	}

	return security, nil
}

/*
Helper method to normalize boolean string values and support all the three boolean standard
This function can be utilized all over in general as a standard convention for boolean values
This will help nake the code be simple all over and without handling all these cases each time
Below are few examples using this.
*/

// parses a boolean expressed as string
//  "true" values "on", "yes", "true"
//  "false" values "off", "no", "false"

// inputs:
//   - boolean as string.
//
// outputs:
//   - normalized boolean
//
// errors:
//   - invalid value
func normalizeBoolean(value string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "on" || normalized == "yes" || normalized == "true" {
		return true, nil
	}
	if normalized == "off" || normalized == "no" || normalized == "false" {
		return false, nil
	}
	return false, common.NewOracleError(
		common.InvalidConnectionParameter,
		nil,
		value,
		"boolean",
		"on|off|yes|no|true|false",
	)
}

// parses a duration string as milliseconds.
//  when there is no duration suffix, we assume that the value is in seconds
//  Negative values are not allowed
//  ex:
//    "10"  -> 10000
//    "10s" -> 10000

// inputs:
//   - duration as string. see time.ParseDuration()
//
// outputs:
//   - the numeric value of the duration
//
// errors:
//   - conversion has failed.
func parseDurationMS(value string) (int, error) {
	d, err := time.ParseDuration(value)
	if err == nil {
		return int(d.Milliseconds()), nil
	}
	// an error means two things:
	//   - invalid numeric value
	//   - value provided without duration suffix (TODO: find a way to distinguish that)

	//handles cases where the timeout value is provided as a plain integer
	// (e.g., "10") without a unit suffix.
	num, err := strconv.Atoi(value)
	if err != nil {
		return -1, common.NewOracleError(
			common.InvalidConnectionParameter,
			err,
			value,
			"duration",
			"Go duration (e.g. 10s, 500ms) OR integer seconds (e.g. 10)",
		)
	}
	if num >= 0 {
		return num * 1000, nil // we assume value is expessed in seconds
	}
	return -1, common.NewOracleError(
		common.InvalidConnectionParameter,
		nil,
		value,
		"duration",
		"non-negative",
	)
}

// parses a  integer expressed as string
// negative value are treat as 0
// inputs:
//   - int as string.
//
// outputs:
//   - converted numeric value
//
// errors:
//   - invalid value
func parseIntField(value string) (int, error) {
	num, err := strconv.Atoi(value)
	if err != nil {
		return -1, common.NewOracleError(
			common.InvalidConnectionParameter,
			err,
			value,
			"integer",
			"numeric",
		)
	}
	if num < 0 {
		num = 0
	}
	return num, nil
}

// parses a unsigned integer expressed as string
//
// inputs:
//   - int as string.
//
// outputs:
//   - converted numeric value
//
// errors:
//   - invalid value
func parseUIntField(value string) (uint, error) {
	num, err := strconv.Atoi(value)
	if err != nil || num < 0 {
		return 0, common.NewOracleError(
			common.InvalidConnectionParameter,
			err,
			value,
			"unsigned-integer",
			"0 or greater",
		)
	}

	return uint(num), nil
}

/*DESCRIPTION Helpers*/
// IsLoadBalanceEnabled returns true if load balancing is enabled
func (d *Description) IsLoadBalanceEnabled() bool {
	return d.LoadBalance
}

// IsFailoverEnabled returns true if failover is enabled
func (d *Description) IsFailoverEnabled() bool {
	return d.Failover
}

// IsSourceRouteEnabled returns true if source routing is enabled
func (d *Description) IsSourceRouteEnabled() bool {
	return d.SourceRoute
}

// IsUseSNIEnabled returns true if SNI (Server Name Indication) is enabled
func (d *Description) IsUseSNIEnabled() bool {
	return d.UseSNI
}

// IsCompressionEnabled returns true if network compression is enabled
func (d *Description) IsCompressionEnabled() bool {
	return d.Compression
}

/*   ADDRESSLIST Helpers   */
// IsLoadBalanceEnabled returns true if load balancing is enabled
func (a *AddressList) IsLoadBalanceEnabled() bool {
	return a.LoadBalance
}

// IsFailoverEnabled returns true if failover is enabled
func (a *AddressList) IsFailoverEnabled() bool {
	return a.Failover
}

// IsSourceRouteEnabled returns true if source routing is enabled
func (a *AddressList) IsSourceRouteEnabled() bool {
	return a.SourceRoute
}

/*   DESCRIPTIONLIST Helpers   */
// IsLoadBalanceEnabled returns true if load balancing is enabled
func (dl *DescriptionList) IsLoadBalanceEnabled() bool {
	return dl.LoadBalance
}

// IsFailoverEnabled returns true if failover is enabled
func (dl *DescriptionList) IsFailoverEnabled() bool {
	return dl.Failover
}

// IsSourceRouteEnabled returns true if source routing is enabled
func (dl *DescriptionList) IsSourceRouteEnabled() bool {
	return dl.SourceRoute
}

/*   SECURITY Helpers   */
// IsSSLServerDNMatchEnabled returns true if SSL server DN matching is enabled
func (s *Security) IsSSLServerDNMatchEnabled() bool {
	return s.SSLServerDNMatch
}

// IsSSLAllowWeakDNMatchEnabled returns true if weak DN matching is allowed
func (s *Security) IsSSLAllowWeakDNMatchEnabled() bool {
	return s.SSLAllowWeakDNMatch
}

// GetAllAddresses retrieves all addresses from a Description
// Collects from all AddressLists and direct Address children
func (d *Description) GetAllAddresses() []Address {
	var allAddresses []Address

	// Collect from all AddressLists
	for _, addrList := range d.AddressLists {
		allAddresses = append(allAddresses, addrList.Addresses...)
	}

	// Also include direct Address children if any
	allAddresses = append(allAddresses, d.Addresses...)

	return allAddresses
}

// HasAddressLists checks if there are any AddressLists
func (d *Description) HasAddressLists() bool {
	return len(d.AddressLists) > 0
}

// GetPrimaryAddressList returns the first AddressList if present, or nil otherwise.
func (d *Description) GetPrimaryAddressList() *AddressList {
	if len(d.AddressLists) > 0 {
		return &d.AddressLists[0]
	}
	return nil
}
