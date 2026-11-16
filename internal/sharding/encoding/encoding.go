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

// Package encoding provides shared key handling for network and TTC consumers.
package encoding

import (
	"encoding/base64"
	"encoding/binary"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	_sharding "github.com/oracle/go-oracledb/v26/oracle/sharding"
)

// EncodeKey returns the listener's version-1/type-0 representation on demand.
// It is shared by protocol consumers and never modifies or caches data on key.
// TTC callers must still use the representation required by their TTC message;
// this function specifically produces the listener's Base64/header format.
func EncodeKey(key _sharding.ShardingKey) ([]byte, error) {
	subkeys := key.Subkeys()
	if len(subkeys) == 0 {
		common.Odl.Debug("EncodeKey: no sharding subkeys")
		return nil, common.NewOracleError(oracleErrors.ShardingKeyEmpty, nil)
	}
	header := "1 0"
	values := make([]string, 0, len(subkeys))
	for i, subkey := range subkeys {
		code, data, err := encodeSubkey(subkey)
		if err != nil {
			common.Odl.Debug("EncodeKey: subkey encoding failed", "subkey", i, "keyType", subkey.Type)
			return nil, common.NewOracleError(oracleErrors.ShardingSubkeyEncoding, err, i, subkey.Type)
		}
		header += " " + strconv.Itoa(code)
		values = append(values, base64.StdEncoding.EncodeToString(data))
	}
	encoded := header + "," + strings.Join(values, ",")
	if len(encoded) > 65535 {
		common.Odl.Debug("EncodeKey: encoded key exceeds limit", "length", len(encoded), "limit", 65535)
		return nil, common.NewOracleError(oracleErrors.ShardingKeyTooLong, nil, len(encoded), 65535)
	}
	return []byte(encoded), nil
}

func encodeSubkey(c _sharding.Subkey) (int, []byte, error) {
	invalid := func() error {
		common.Odl.Debug("encodeSubkey: unsupported key type or value", "keyType", c.Type)
		return common.NewOracleError(oracleErrors.ShardingSubkeyInvalid, nil, c.Type)
	}
	switch c.Type {
	case _sharding.VARCHAR, _sharding.CHAR, _sharding.NVARCHAR, _sharding.NCHAR:
		s, ok := c.Value.(string)
		if !ok || s == "" || !utf8.ValidString(s) {
			return 0, nil, invalid()
		}
		code := 1
		if c.Type == _sharding.CHAR || c.Type == _sharding.NCHAR {
			code = 96
		}
		if c.Type == _sharding.NVARCHAR || c.Type == _sharding.NCHAR {
			units := utf16.Encode([]rune(s))
			b := make([]byte, 2*len(units))
			for i, u := range units {
				binary.BigEndian.PutUint16(b[2*i:], u)
			}
			return code, b, nil
		}
		return code, []byte(s), nil
	case _sharding.RAW:
		b, ok := c.Value.([]byte)
		if !ok || len(b) == 0 {
			return 0, nil, invalid()
		}
		return 23, append([]byte(nil), b...), nil
	case _sharding.NUMBER:
		if c.Value == nil {
			return 0, nil, invalid()
		}
		v := reflect.ValueOf(c.Value)
		var b []byte
		var err error
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			b, err = converters.EncodeInt(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			b, err = converters.EncodeUInt(v.Uint())
		case reflect.Float32, reflect.Float64:
			b, err = converters.EncodeDecimal(v.Float())
		default:
			return 0, nil, invalid()
		}
		return 2, b, err
	case _sharding.DATE, _sharding.TIMESTAMP:
		t, ok := c.Value.(time.Time)
		// Restrict to AD dates until BC serialization has independent fixtures.
		if !ok || t.Year() < 1 || t.Year() > 9999 {
			return 0, nil, invalid()
		}
		if c.Type == _sharding.DATE {
			b, err := converters.EncodeDate(t)
			return 12, b, err
		}
		b, err := converters.EncodeTimestamp(t)
		return 180, b, err
	default:
		return 0, nil, invalid()
	}
}
