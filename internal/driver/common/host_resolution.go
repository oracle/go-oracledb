/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** This license is subject to the condition set forth below, permission is hereby
** granted to any person obtaining a copy of this software, associated
** documentation and/or data (collectively, the "Software"), free of charge and
** under all applicable copyright and patent rights, to deal in the Software
** without restriction, including without limitation the rights to copy, modify,
** merge, publish, distribute, sublicense, and/or sell copies of the Software,
** and to permit persons to whom the Software is furnished to do so, subject to
** the following conditions:
**
** The above copyright notice and this permission notice shall be included in
** all copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 */

package common

import (
	"context"
	"net"
)

// LookupIPAddrFunc looks up IP addresses for a hostname using the supplied
// context. It is exposed so callers can test resolution without making DNS
// requests.
type LookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

// ResolveHost resolves a hostname to IP strings. IP literals are returned
// directly without a DNS lookup.
//
// Input:
//   - ctx: context that can cancel or bound the DNS lookup.
//   - host: hostname or IP literal to resolve.
//
// Output:
//   - []string: resolved IP addresses in resolver order; it is empty when the
//     resolver returns no addresses.
//   - error: context cancellation or the resolver error, so the caller can
//     apply its own fallback policy.
func ResolveHost(ctx context.Context, host string) ([]string, error) {
	return ResolveHostWithLookup(ctx, host, net.DefaultResolver.LookupIPAddr)
}

// ResolveHostWithLookup resolves a hostname with an explicit lookup function.
// IP literals bypass the lookup function.
//
// Input:
//   - ctx: context that can cancel or bound the lookup.
//   - host: hostname or IP literal to resolve.
//   - lookup: function that returns the host's IP addresses.
//
// Output:
//   - []string: resolved IP addresses in lookup order; it is empty when the
//     lookup returns no addresses.
//   - error: context cancellation or lookup failure.
func ResolveHostWithLookup(ctx context.Context, host string, lookup LookupIPAddrFunc) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if net.ParseIP(host) != nil {
		return []string{host}, nil
	}
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}

	addresses, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil {
			ips = append(ips, address.IP.String())
		}
	}
	return ips, nil
}
