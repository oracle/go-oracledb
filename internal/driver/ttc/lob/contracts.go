/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights and under any and all patent rights owned or freely licensable by each
** licensor hereunder covering either (i) the unmodified Software as contributed
** to or provided by such licensor, or (ii) the Larger Works (as defined below),
** to deal in both the Software and any piece of software and/or hardware listed
** in the lrgrwrks.txt file if one is included with the Software, without
** restriction, including without limitation the rights to copy, create derivative
** works of, display, perform, and distribute the Software and make, use, sell,
** offer for sale, import, export, have made, and have sold the Software and the
** Larger Work(s), and to sublicense the foregoing on these or other terms.
**
** This license is subject to the condition that the above copyright notice and
** either this complete permission notice or a reference to the UPL be included
** in all copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package lob

// Locator is the opaque locator capability used by the reference registry.
// TTC adapts its concrete locator to this interface. Snapshot returns an
// independent copy of the locator bytes for bind encoding or cleanup payload
// construction.
type Locator interface {
	Snapshot() []byte
	MarkReleased()
	ReferenceIdentity() (ReferenceID, error)
}

// ReferenceID is an opaque, immutable identity used to group aliases of one
// server-side LOB. TTC derives it from the locator format.
type ReferenceID string

// Reference is an opaque token for one local owner of a temporary or abstract
// LOB. Its unexported method keeps construction within this package.
type Reference interface {
	lobReferenceToken()
}

// FreeBatch is an opaque set of pending frees reserved for one outgoing TTC
// exchange. Payload returns the concatenated locator bytes used by the TTC
// array-free message; it must return a copy that callers may safely retain.
type FreeBatch interface {
	Payload() []byte
}

// ReferenceRegistry tracks local owners and supports the reserve/restore/commit
// lifecycle needed to couple cleanup bookkeeping to a successful transport
// flush. The TTC MessageStreamer remains responsible for that flush boundary.
type ReferenceRegistry interface {
	Retain(locator Locator) (Reference, error)
	Release(reference Reference) error
	ReservePending() (FreeBatch, error)
	CompletePending(batch FreeBatch)
	RestorePending(batch FreeBatch)
	Discard()
}
