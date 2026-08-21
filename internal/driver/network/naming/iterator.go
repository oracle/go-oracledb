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
	"context"
	"math/rand"
	"strconv"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

// ConnectionOption represents a single connection attempt with all necessary information.
type ConnectionOption struct {
	Address         Address
	Description     *Description
	ConnectData     ConnectData
	ConnectDataNode *Node
	ConnectDataStr  string
	ConnectString   string
}

func NewConnectionOption(
	address Address,
	description *Description,
	connectData ConnectData,
	connectDataNode *Node,
	connectDataStr string,
	connectString string,
) *ConnectionOption {
	return &ConnectionOption{
		Address:         address,
		Description:     description,
		ConnectData:     connectData,
		ConnectDataNode: connectDataNode,
		ConnectDataStr:  connectDataStr,
		ConnectString:   connectString,
	}
}

// DescriptionAttempts holds all connection attempts for a single description
// Addresses are stored once, and iterated multiple times based on retry count
type DescriptionAttempts struct {
	Description      *Description  // Parent description
	DescriptionNode  *Node         // Original DESCRIPTION node
	Addresses        []Address     // All addresses collected from this description (with resolved IPs)
	ConnectDataNode  *Node         // Shared CONNECT_DATA node
	ConnectDataStr   string        // Serialized CONNECT_DATA string
	RetryCount       int           // Number of retry cycles (0 = try once)
	RetryDelay       time.Duration // Delay between retry cycles
	CurrentCycle     int           // Current retry cycle (0-based)
	CurrentAddrIndex int           // Current address index within cycle
}

// ConnectionIterator manages iteration through connection attempts.
// Uses lazy iteration with addresses stored once per description to optimize memory.
// DNS resolution is performed once during iterator creation.
type ConnectionIterator struct {
	rootNode         *Node
	context          *ConnectionContext
	callerCtx        context.Context
	descAttempts     []DescriptionAttempts
	currentDescIndex int
	exhausted        bool
	rng              *rand.Rand
	// dnsErrors     []error  // TODO
}

// NewConnectionIterator creates an iterator from parsed connection context.
// Addresses are stored once per description and iterated based on retry count.
// DNS resolution happens during iterator creation - hostnames are resolved to IPs,
// and each IP becomes a separate connection attempt.
func NewConnectionIterator(ctx context.Context, rootNode *Node, connCtx *ConnectionContext) *ConnectionIterator {
	iter := &ConnectionIterator{
		rootNode:         rootNode,
		context:          connCtx, // extracted properties context(different from the passed context)
		callerCtx:        ctx,
		currentDescIndex: 0,
		exhausted:        false,
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	iter.descAttempts = iter.buildDescriptionAttempts(ctx)
	return iter
}

// Next returns the next connection option, or nil if exhausted.
// Handles retry cycles and retry delays automatically.
func (ci *ConnectionIterator) Next() *ConnectionOption {
	if ci.exhausted {
		return nil
	}

	// Iterate through descriptions
	for ci.currentDescIndex < len(ci.descAttempts) {
		desc := &ci.descAttempts[ci.currentDescIndex]

		// Try next address in current cycle
		if desc.CurrentAddrIndex < len(desc.Addresses) {
			option := ci.buildOptionFromDesc(desc)
			desc.CurrentAddrIndex++
			return option
		}

		// All addresses tried in current cycle - check if we should retry
		if desc.CurrentCycle < desc.RetryCount {
			// Apply retry delay before starting next cycle
			if desc.RetryDelay > 0 {
				timer := time.NewTimer(desc.RetryDelay)
				select {
				case <-timer.C:
				case <-ci.callerCtx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					ci.exhausted = true
					return nil
				}
			}

			// Start new retry cycle
			desc.CurrentCycle++
			desc.CurrentAddrIndex = 0

			// Re-shuffle addresses if load balance is enabled
			if desc.Description != nil && desc.Description.IsLoadBalanceEnabled() {
				if desc.CurrentCycle < 1 {
					ci.shuffleAddresses(desc.Addresses)
				} else {
					ci.roundRobin(desc.Addresses)
				}
			}

			// Continue to try first address of new cycle
			continue
		}

		// Description exhausted, move to next
		ci.currentDescIndex++
	}

	// All descriptions exhausted
	ci.exhausted = true
	return nil
}

// HasNext returns true if more attempts are available
func (ci *ConnectionIterator) HasNext() bool {
	if ci.exhausted {
		return false
	}

	// Check if current position has more attempts
	for i := ci.currentDescIndex; i < len(ci.descAttempts); i++ {
		desc := &ci.descAttempts[i]

		// If we're on current description, check from current position
		if i == ci.currentDescIndex {
			// Has more addresses in current cycle
			if desc.CurrentAddrIndex < len(desc.Addresses) {
				return true
			}
			// Has more retry cycles
			if desc.CurrentCycle < desc.RetryCount {
				return true
			}
		} else {
			// Future descriptions - check if they have any addresses
			if len(desc.Addresses) > 0 {
				return true
			}
		}
	}

	// No more attempts available
	ci.exhausted = true
	return false
}

// Reset resets the iterator to the beginning with new randomization
func (ci *ConnectionIterator) Reset() {
	ci.currentDescIndex = 0
	ci.exhausted = false
	ci.rng = rand.New(rand.NewSource(time.Now().UnixNano()))

	// Reset all description attempts
	for i := range ci.descAttempts {
		ci.descAttempts[i].CurrentCycle = 0
		ci.descAttempts[i].CurrentAddrIndex = 0

		// Re-shuffle if load balance enabled
		if ci.descAttempts[i].Description != nil &&
			ci.descAttempts[i].Description.IsLoadBalanceEnabled() {
			ci.shuffleAddresses(ci.descAttempts[i].Addresses)
		}
	}
}

// Remaining returns the number of remaining attempts
func (ci *ConnectionIterator) Remaining() int {
	if ci.exhausted {
		return 0
	}

	count := 0
	for i := ci.currentDescIndex; i < len(ci.descAttempts); i++ {
		desc := &ci.descAttempts[i]

		// Remaining in current cycle
		if i == ci.currentDescIndex {
			count += len(desc.Addresses) - desc.CurrentAddrIndex
			// Add future cycles
			count += len(desc.Addresses) * (desc.RetryCount - desc.CurrentCycle)
		} else {
			// Future descriptions - all cycles
			count += len(desc.Addresses) * (desc.RetryCount + 1)
		}
	}

	return count
}

// Total returns the total number of attempts
func (ci *ConnectionIterator) Total() int {
	count := 0
	for i := range ci.descAttempts {
		count += len(ci.descAttempts[i].Addresses) * (ci.descAttempts[i].RetryCount + 1)
	}
	return count
}

// buildDescriptionAttempts generates the description attempts based on context type
func (ci *ConnectionIterator) buildDescriptionAttempts(ctx context.Context) []DescriptionAttempts {
	if ci.context.DescriptionList != nil {
		return ci.buildFromDescriptionList(ctx, ci.context.DescriptionList)
	}

	if ci.context.Description != nil {
		descAttempt := ci.buildFromDescription(ctx, ci.context.Description, ci.rootNode)
		if descAttempt != nil {
			return []DescriptionAttempts{*descAttempt}
		}
		return []DescriptionAttempts{}
	}

	if len(ci.context.Addresses) > 0 {
		return ci.buildFromAddresses(ctx, ci.context.Addresses)
	}

	return []DescriptionAttempts{}
}

// buildFromDescriptionList processes DESCRIPTION_LIST with failover logic
func (ci *ConnectionIterator) buildFromDescriptionList(ctx context.Context, dl *DescriptionList) []DescriptionAttempts {
	if len(dl.Descriptions) == 0 {
		return []DescriptionAttempts{}
	}

	// Check failover at DESCRIPTION_LIST level
	isFailover := dl.Failover

	// Collect description indices
	indices := make([]int, len(dl.Descriptions))
	for i := range indices {
		indices[i] = i
	}

	// Apply load balancing - shuffle description order
	if dl.LoadBalance {
		ci.shuffleIndices(indices)
	}

	// If failover is disabled, only use the first description (after shuffle)
	if !isFailover {
		indices = indices[:1]
	}

	// Build attempts for selected descriptions
	attempts := make([]DescriptionAttempts, 0, len(indices))
	for _, idx := range indices {
		descNode := ci.findDescriptionNode(idx)
		if descNode == nil {
			continue
		}

		descAttempt := ci.buildFromDescription(ctx, &dl.Descriptions[idx], descNode)
		if descAttempt != nil {
			attempts = append(attempts, *descAttempt)
		}
	}

	return attempts
}

// buildFromDescription processes a single DESCRIPTION with all its addresses
// DNS resolution is performed here
func (ci *ConnectionIterator) buildFromDescription(ctx context.Context, desc *Description, descNode *Node) *DescriptionAttempts {
	// Collect all addresses with failover logic
	addresses := ci.collectAllAddresses(desc)
	if len(addresses) == 0 {
		return nil
	}

	// DNS RESOLUTION HAPPENS HERE
	// Resolve all hostnames to IPs - this expands the address list
	resolvedAddresses := make([]Address, 0, len(addresses)*2)
	for _, addr := range addresses {
		expanded, err := resolveAddress(ctx, addr)
		if err != nil {
			// Context cancelled/expired: stop resolution but keep what we already resolved.
			break
		}
		resolvedAddresses = append(resolvedAddresses, expanded...)
	}

	// If all addresses failed to resolve, skip this description
	if len(resolvedAddresses) == 0 {
		common.Odl.Debug("No resolved addresses for DESCRIPTION; skipping this description")
		return nil
	}

	// Apply load balancing at DESCRIPTION level
	if desc.IsLoadBalanceEnabled() {
		ci.shuffleAddresses(resolvedAddresses)
	}
	// Apply failover at DESCRIPTION level
	if !desc.IsFailoverEnabled() {
		// Only keep first address (already shuffled if load_balance=YES)
		resolvedAddresses = resolvedAddresses[:1]
	}

	// Extract CONNECT_DATA node once
	connectDataNode := ci.extractConnectDataNode(descNode)
	connectDataStr := connectDataNode.ToString()
	// Convert RetryDelay from seconds to duration
	retryDelay := time.Duration(desc.RetryDelay) * time.Second

	return &DescriptionAttempts{
		Description:      desc,
		DescriptionNode:  descNode,
		Addresses:        resolvedAddresses, //contains resolved IPs
		ConnectDataNode:  connectDataNode,
		ConnectDataStr:   connectDataStr,
		RetryCount:       desc.RetryCount,
		RetryDelay:       retryDelay,
		CurrentCycle:     0,
		CurrentAddrIndex: 0,
	}
}

// buildFromAddresses handles simple ADDRESS-only strings without DESCRIPTION wrapper
// DNS resolution is performed here
func (ci *ConnectionIterator) buildFromAddresses(ctx context.Context, addresses []Address) []DescriptionAttempts {
	if len(addresses) == 0 {
		return []DescriptionAttempts{}
	}

	// DNS RESOLUTION HAPPENS HERE
	resolvedAddresses := make([]Address, 0, len(addresses)*2)
	for _, addr := range addresses {
		expanded, err := resolveAddress(ctx, addr)
		if err != nil {
			// Context cancelled/expired: stop resolution but keep what we already resolved.
			break
		}
		resolvedAddresses = append(resolvedAddresses, expanded...)
	}

	// If all addresses failed to resolve, return empty
	if len(resolvedAddresses) == 0 {
		return []DescriptionAttempts{}
	}

	return []DescriptionAttempts{
		{
			Description:      nil,
			DescriptionNode:  nil,
			Addresses:        resolvedAddresses, //contains resolved IPs
			ConnectDataNode:  &Node{Name: "CONNECT_DATA"},
			ConnectDataStr:   "",
			RetryCount:       0,
			RetryDelay:       0,
			CurrentCycle:     0,
			CurrentAddrIndex: 0,
		},
	}
}

// collectAllAddresses gathers all addresses from ADDRESS_LISTs and direct ADDRESSes
// Applies failover logic at ADDRESS_LIST level
func (ci *ConnectionIterator) collectAllAddresses(desc *Description) []Address {
	capacity := len(desc.Addresses)
	for i := range desc.AddressLists {
		capacity += len(desc.AddressLists[i].Addresses)
	}

	addresses := make([]Address, 0, capacity)

	// Process each ADDRESS_LIST
	for i := range desc.AddressLists {
		addrList := &desc.AddressLists[i]
		if len(addrList.Addresses) == 0 {
			continue
		}

		// Note: AddressList doesn't have Failover field, so we assume failover=true
		// If you add Failover field to AddressList later, uncomment and use:
		// isFailover := normalizeBoolean(addrList.Failover)
		// if addrList.Failover == "" {
		//     isFailover = true
		// }

		// For now, always include all addresses from ADDRESS_LIST
		addresses = append(addresses, addrList.Addresses...)
	}

	// Add direct ADDRESS children
	addresses = append(addresses, desc.Addresses...)

	return addresses
}

// extractConnectDataNode retrieves the CONNECT_DATA node from a DESCRIPTION node
func (ci *ConnectionIterator) extractConnectDataNode(descNode *Node) *Node {
	if descNode == nil {
		return &Node{Name: "CONNECT_DATA"}
	}

	// Try using GetNode with full path
	connectDataNode, err := descNode.GetNode("DESCRIPTION/CONNECT_DATA")
	if err == nil {
		return connectDataNode
	}

	// Fallback: search direct children
	for i := range descNode.Children {
		if descNode.Children[i].Name == "CONNECT_DATA" {
			return &descNode.Children[i]
		}
	}

	return &Node{Name: "CONNECT_DATA"}
}

// buildOptionFromDesc creates a connection option from description attempts(called from Next())
// Uses ResolvedIP in the connection string instead of hostname
func (ci *ConnectionIterator) buildOptionFromDesc(desc *DescriptionAttempts) *ConnectionOption {
	addr := &desc.Addresses[desc.CurrentAddrIndex]

	if desc.Description == nil {
		// Simple ADDRESS without DESCRIPTION wrapper
		return NewConnectionOption(*addr, nil, ConnectData{}, nil, "", ci.buildDescriptionWithAddress(addr))
	}

	// Full DESCRIPTION with CONNECT_DATA
	return NewConnectionOption(
		*addr,
		desc.Description,
		desc.Description.ConnectData,
		desc.ConnectDataNode,
		desc.ConnectDataStr,
		ci.buildConnectString(addr, desc.ConnectDataNode),
	)
}

// buildConnectString creates the full (DESCRIPTION=(ADDRESS=...)(CONNECT_DATA=...)) string
// Uses ResolvedIP instead of hostname for the actual connection
func (ci *ConnectionIterator) buildConnectString(addr *Address, connectDataNode *Node) string {
	// Use ResolvedIP if available, otherwise fall back to Host
	host := addr.ResolvedIP
	if host == "" {
		host = addr.Host
	}

	addrNode := Node{
		Name: "ADDRESS",
		Children: []Node{
			{Name: "PROTOCOL", Value: addr.Protocol.String()},
			{Name: "HOST", Value: host}, // Use resolved IP here
			{Name: "PORT", Value: strconv.Itoa(int(addr.Port))},
		},
	}

	descNode := Node{
		Name:     "DESCRIPTION",
		Children: []Node{addrNode, *connectDataNode},
	}

	return descNode.ToString()
}

// buildDescriptionWithAddress wraps a simple ADDRESS in DESCRIPTION for consistency
// Uses ResolvedIP instead of hostname for the actual connection
func (ci *ConnectionIterator) buildDescriptionWithAddress(addr *Address) string {
	// Use ResolvedIP if available, otherwise fall back to Host
	host := addr.ResolvedIP
	if host == "" {
		host = addr.Host
	}

	addrNode := Node{
		Name: "ADDRESS",
		Children: []Node{
			{Name: "PROTOCOL", Value: addr.Protocol.String()},
			{Name: "HOST", Value: host}, // Use resolved IP here
			{Name: "PORT", Value: strconv.Itoa(int(addr.Port))},
		},
	}

	descNode := Node{
		Name:     "DESCRIPTION",
		Children: []Node{addrNode},
	}

	return descNode.ToString()
}

// findDescriptionNode locates the Nth DESCRIPTION node under DESCRIPTION_LIST
func (ci *ConnectionIterator) findDescriptionNode(index int) *Node {
	if ci.rootNode == nil || ci.rootNode.Name != "DESCRIPTION_LIST" {
		return nil
	}

	count := 0
	for i := range ci.rootNode.Children {
		if ci.rootNode.Children[i].Name == "DESCRIPTION" {
			if count == index {
				return &ci.rootNode.Children[i]
			}
			count++
		}
	}

	return nil
}

// shuffleIndices randomizes the order of indices for load balancing
func (ci *ConnectionIterator) shuffleIndices(indices []int) {
	ci.rng.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
}

// shuffleAddresses randomizes the order of addresses for load balancing
func (ci *ConnectionIterator) shuffleAddresses(addresses []Address) {
	ci.rng.Shuffle(len(addresses), func(i, j int) {
		addresses[i], addresses[j] = addresses[j], addresses[i]
	})
}

// rotateAddresses rotates the addresses array left by one position for round-robin ordering.
func (ci *ConnectionIterator) roundRobin(addresses []Address) {
	if len(addresses) < 2 {
		return // No rotation needed for 0 or 1 addresses
	}
	first := addresses[0]
	copy(addresses[0:], addresses[1:])
	addresses[len(addresses)-1] = first
}
