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

// Package sharding stores private routing values without depending on the
// public sharding types or their protocol encoders.
package sharding

import "context"

type contextKey uint8

// WithKeys stores snapshotted, immutable keys. It copies the optional pointer's
// value so replacing the caller's key cannot change the stored routing request.
func WithKeys[T any](ctx context.Context, key T, super *T) context.Context {
	keys := [2]*T{&key, nil}
	if super != nil {
		copied := *super
		keys[1] = &copied
	}
	return context.WithValue(ctx, contextKey(0), keys)
}

// Keys retrieves typed keys without encoding them. Returned pointers refer to
// copies so callers cannot replace the context's stored keys.
func Keys[T any](ctx context.Context) (key, super *T) {
	keys, ok := ctx.Value(contextKey(0)).([2]*T)
	if !ok {
		return nil, nil
	}
	copied := *keys[0]
	key = &copied
	if keys[1] != nil {
		copiedSuper := *keys[1]
		super = &copiedSuper
	}
	return key, super
}
