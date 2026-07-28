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

import "context"

type StreamDirection int

const (
	IN StreamDirection = iota
	OUT
	INOUT
)

// Streamer interface used to exchange messages
type Streamer[T any] interface {
	// Push pushes a message to the streamer. There is no guaranty the message is sent.
	// When the marshaling and send of the message happen is up to implementation
	// Parameter:
	//   context : method execution context
	//   Messages: the message to be placed for delivery
	// Returns:
	//   error: eror happened while placing the message
	Push(context.Context, Message[T]) error

	// Pull pulls a message form the streamer.
	// This is a blocking call that returns on error or when a message of the
	// requested type is available for delivery.
	//
	// Parameter:
	//   context : method execution context
	//   Message types: the variadic list of message types that a Pull should return
	//
	// Returns:
	//   message : the next message available. Can't be nil
	//   error: error happened while processing the incoming message. The caller must first check this error so the return message
	//          can be considered in a stable state.
	Pull(context.Context, ...T) (Message[T], error)

	// Flush does whatever it takes to send the message to the other end of the stream.
	// Parameter:
	//   context : method execution context
	//   Message types: the variadic list of message types that a Pull should return
	// Returns:
	//   error: error happened while placing the message
	Flush(context.Context) error

	// Drain drains out any outgoing and incoming data form this streamer.
	// Parameter:
	//   Context : method execution context
	//   StreamDirection: streamer direction to be drained
	// Returns:
	//    - the number of message present in the incoming queue
	//    - the number of message present in the outgoing queue
	Drain(context.Context, StreamDirection) (int, int)
}
