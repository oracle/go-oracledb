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

// Package sharding defines typed keys for GSM connection routing.
package sharding

import (
	"bytes"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// KeyType identifies the Oracle type of a subkey.
type KeyType uint8

const (
	VARCHAR KeyType = iota + 1
	CHAR
	NVARCHAR
	NCHAR
	NUMBER
	RAW
	DATE
	TIMESTAMP
)

// Subkey is one ordered subkey of a key. Character types accept string;
// NUMBER accepts signed/unsigned integers and finite float32/float64; RAW accepts
// []byte; DATE and TIMESTAMP accept time.Time (wall-clock fields, no time zone).
// At encoding time NUMBER also accepts decimal strings (up to 38 significant
// digits, optional exponent), RAW accepts even-length hexadecimal strings without
// a prefix, and DATE/TIMESTAMP accept YYYY-MM-DD, ISO/SQL timestamps, or RFC3339.
// Zone-free dates use UTC; zoned dates retain wall-clock fields during encoding.
// Null and empty character/RAW values are not supported.
type Subkey struct {
	Type  KeyType
	Value any
}

// ShardingKey retains typed subkeys in the order supplied by the application.
// NewKey copies caller-owned data; encoding is deferred until a protocol needs it.
type ShardingKey struct{ subkeys []Subkey }

// ShardingRequest supplies a required SK and an optional SSK.
type ShardingRequest struct {
	ShardingKey      ShardingKey
	SuperShardingKey *ShardingKey
}

// NewKey stores ordered subkeys without encoding them. Empty keys are rejected;
// protocol-specific type/value and length checks occur during encoding.
func NewKey(subkeys ...Subkey) (ShardingKey, error) {
	if len(subkeys) == 0 {
		common.Odl.Debug("NewKey: no sharding subkeys")
		return ShardingKey{}, common.NewOracleError(oracleErrors.ShardingKeyEmpty, nil)
	}
	return ShardingKey{subkeys: copySubkeys(subkeys)}, nil
}

// Subkeys returns a copy in application-supplied order, including copies of RAW
// bytes. Network and TTC encoders can inspect values without changing the key.
func (k ShardingKey) Subkeys() []Subkey { return copySubkeys(k.subkeys) }

func copySubkeys(source []Subkey) []Subkey {
	result := append([]Subkey(nil), source...)
	for i := range result {
		if raw, ok := result[i].Value.([]byte); ok {
			result[i].Value = bytes.Clone(raw)
		}
	}
	return result
}

// String prevents ordinary formatting from disclosing key data.
func (k ShardingKey) String() string { return "<sharding key>" }

// GoString prevents Go-syntax formatting from disclosing key data.
func (k ShardingKey) GoString() string { return k.String() }
