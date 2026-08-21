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

package session

const (
	// TNS packet types
	NSPTCN  = 1  // Connect
	NSPTAC  = 2  // Accept
	NSPTAK  = 3  // Acknowledge
	NSPTRF  = 4  // Refuse
	NSPTRD  = 5  // Redirect
	NSPTDA  = 6  // Data
	NSPTNL  = 7  // Null
	NSPTAB  = 9  // Abort
	NSPTRS  = 11 // Re-send
	NSPTMK  = 12 // Marker
	NSPTAT  = 13 // Attention
	NSPTCNL = 14 // Control information
	NSPTDD  = 15 // Data descriptor
	NSPTHI  = 19 // Highest legal packet type

	// Packet header
	NSPHDLEN  = 0 // Packet length
	NSPHDPSM  = 2 // Packet checksum (deprecated in version 3.15 with large SDU support)
	NSPHDTYP  = 4 // Packet type
	NSPHDFLGS = 5 // Packet flags
	NSPHDHSM  = 6 // Header checksum
	NSPSIZHD  = 8 // Packet header size

	// Packet flags for NSPHDFLGS
	NSPFSID         = 0x01 // SID is given
	NSPFRDS         = 0x02 // Redirect Separation of cnda vs cndo
	NSPFRDR         = 0x04 // Redirected client Connect (NSPTCN)
	NSPFLSD         = 0x20 // Packet with large SDU field
	NO_HEADER_FLAGS = 0
	NSPFSRN         = 0x08

	// Connect Packet
	NSPCNVSN   = 8  // My version number
	NSPCNLOV   = 10 // Lowest version number I can be compatible with
	NSPCNOPT   = 12 // Global service options
	NSPCNSDU   = 14 // My SDU size
	NSPCNTDU   = 16 // Maximum TDU size
	NSPCNNTC   = 18 // NT characteristics
	NSPCNTNA   = 20 // Line turnaround value
	NSPCNONE   = 22 // The value '1' in my hardware byte order
	NSPCNLEN   = 24 // Length of connect data
	NSPCNOFF   = 26 // Offset to connect data
	NSPCNMXC   = 28 // Maximum connect data you can send me
	NSPCNFL0   = 32 // Connect flags
	NSPCNFL1   = 33
	NSPCNTMO   = 50 // Local connection timeout val
	NSPCNTCK   = 52 // Local tick size
	NSPCNADL   = 54
	NSPCNAOF   = 56  // Offset to reconnect TNS addr
	NSPCNLSD   = 58  // Large SDU
	NSPCNLTD   = 62  // Large TDU
	NSPCNCFL   = 66  // Compression data
	NSPCNCFL2  = 70  // Connect flag2
	NSPCNDAT   = 74  // Start connect data
	NSPMXCDATA = 230 // Maximum length of connect data

	// Connect flags (Used mostly by NA)
	NSINAWANTED                = 0x01 // Want to use NA
	NSINAINTCHG                = 0x02 // Interchange involved
	NSINADISABLEDFORCONNECTION = 0x04 // Disable NA
	NSINANOSERVICES            = 0x08 // No NA services linked
	NSINAREQUIRED              = 0x10 // NA is required
	NSINAAUTHWANTED            = 0x20 // Authentication linked
	NSISUPSECRENEG             = 0x80 // Backward comp: Support Security Re-neg

	// Connect options
	NSGDONTCARE                     = 0x0001 // "Don't care"
	NSGHDX                          = 0x0002 // Half-duplex (w/ token management)
	NSGFDX                          = 0x0004 // Full-duplex
	NSGHDRCHKSUM                    = 0x0008 // Checksum on packet header
	NSGPAKCHKSUM                    = 0x0010 // Checksum on entire packet
	NSGBROKEN                       = 0x0020 // Provide broken connection notification
	NSGUSEVIO                       = 0x0040 // Can use Vectored I/O
	NSGOSAUTHOK                     = 0x0080 // Use OS authentication
	NSGSENDATTN                     = 0x0200 // Can send attention
	NSGRECVATTN                     = 0x0400 // Can receive attention
	NSGNOATTNPR                     = 0x0800 // No attention processing
	NSGRAW                          = 0x1000 // I/O is direct to/from transport
	TNS_VERSION_DESIRED             = 319
	TNS_VERSION_MINIMUM             = 300
	TNS_VERSION_MIN_DATA_FLAGS      = 318
	TNS_VERSION_MIN_END_OF_RESPONSE = 319
	TNS_UUID_OFFSET                 = 45

	// Accept Packet
	NSPACVSN     = 8  // Connection version
	NSPACOPT     = 10 // Global service options
	NSPACSDU     = 12 // SDU size
	NSPACTDU     = 14 // Maximum TDU
	NSPACONE     = 16 // The value '1' in my hardware byte order
	NSPACLEN     = 18 // Connect data length
	NSPACOFF     = 20 // Offset to connect data
	NSPACFL0     = 22 // Connect flags
	NSPACFL1     = 23
	NSPACTMO     = 24 // Connection pool timeout value
	NSPACTCK     = 26 // Local tick size
	NSPACADL     = 28 // Reconnect TNS address length
	NSPACAOF     = 30 // Offset to reconnect TNS addr
	NSPACLSD     = 32 // Large SDU
	NSPACLTD     = 36 // Large TDU
	NSPACCFL     = 40 // Compression flag
	NSPACFL2     = 41 // Accept flag2 (4 bytes)
	NSPACV310DAT = 32 // Start of accept data, V3.10 packet
	NSPACV315DAT = 41 // Start of accept data, V3.15 packet

	// Refuse Packet
	NSPRFURS = 8  // User (application) reason for refusal
	NSPRFSRS = 9  // System (NS) reason for
	NSPRFLEN = 10 // Length of refuse data
	NSPRFDAT = 12 // Start of connect data

	// Compression flags
	NSPACCFON = 0x80 // 1st MSB: compression on/off
	NSPACCFAT = 0x40 // 2nd MSB: compression auto
	NSPACCFNT = 0x02 // Second last LSB: compression for non-TCP protocol

	// Accept flag2
	NSPACOOB                           = 0x00000001 // OOB support check at connection time
	NSGPCHKSCMD                        = 0x01000000 // Support for Poll and Check logic
	TNS_ACCEPT_FLAG_HAS_END_OF_REQUEST = 0x02000000
	TNS_ACCEPT_FLAG_FAST_AUTH          = 0x10000000 // Support Fast Auth

	// Redirect packet
	NSPRDLEN = 8  // Length of redirect data
	NSPRDDAT = 10 // Start of connect data

	// Data Packet
	NSPDAFLG  = 8     // Data flags
	NSPDADAT  = 10    // Start of Data
	NSPDAFEOF = 0x40  // "End of file"
	NSPDAFCMP = 0x400 // "Compressed data"

	// Marker Packet
	NSPMKTYP = 8  // Marker type
	NSPMKODT = 9  // Old (pre-V3.05) data byte
	NSPMKDAT = 10 // Data byte
	NSPMKTD0 = 0  // Data marker - 0 data bytes
	NSPMKTD1 = 1  // Data marker - 1 data byte
	NSPMKTAT = 2  // Attention Marker
	NIQBMARK = 1  // Break marker
	NIQRMARK = 2  // Reset marker
	NIQIMARK = 3  // Interrupt marker

	// Control Packet
	NSPCTLCMD      = 8  // Control Command length is 2 bytes
	NSPCTLDAT      = 10 // Control Data length is specific to the Command type
	NSPCTL_SERR    = 8  // Error Control Command Type
	NSPCTL_CLRATTN = 9  // Clear OOB option

	// OPTIONS
	NSPDFSDULN  = 8192    // Default SDU size
	NSPABSSDULN = 2097152 // Maximum SDU size
	NSPMXSDULN  = 65535   // Maximum SDU size
	NSPMNSDULN  = 512     // Minimum SDU size
	NSPDFTDULN  = 2097152 // Default TDU size
	NSPMXTDULN  = 2097152 // Maximum TDU size
	NSPMNTDULN  = 255     // Minimum TDU size
	// Immediate close

	// PARAMETERS
	DISABLE_OOB_STR                   = "DISABLE_OOB" // Disable OOB parameter
	EXPIRE_TIME                       = "EXPIRE_TIME" // Expire Time
	PEM_WALLET_FILE_NAME              = "ewallet.pem"
	DEFAULT_TRANSPORT_CONNECT_TIMEOUT = 20000 // Default transport connect timeout
	DEFAULT_RETRY_DELAY               = 1000  // Default retry delay

	// Get/Set options
	NT_MOREDATA = 1 // More Data in Transport available
	NS_MOREDATA = 2 // More Data available to be read
	SVCNAME     = 3 // Service name
	SERVERTYPE  = 4 // Server type
	REMOTEADDR  = 5 // Remote Address
	HEALTHCHECK = 6 // Health check of connection
	CONNCLASS   = 7 // Connection Class
	PURITY      = 8 // Purity
	SID         = 9 // SID

	// Network Compression Algorithms
	NETWORK_COMPRESSION_ZLIB = 2
	PACKET_HEADER_SIZE       = 8
	// FLAGS
	NSNOBLOCK             = 0x0001 // Do not block
	NSPDDCNTMAX           = 26
	NSPDDSSLMAX           = 65535
	NSPDDSMAXTO           = NSPDDCNTMAX * NSPDDSSLMAX
	NSECMANSHUT           = 12572
	NSESENDMESG           = 12573
	ORA_ERROR_EMFI_NUMBER = 22

	TCP_DEFAULT_PORT = 1521
)
