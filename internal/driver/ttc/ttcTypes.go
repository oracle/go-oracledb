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

package ttc

import (
	"container/list"
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// keyValueList Key-Value pair list. a list.List of *common.KeyValue
type keyValueList struct {
	*list.List
}

const (
	maxAuthKeyValueKeyLength   = 4096
	maxAuthKeyValueValueLength = 1024 * 10
)

func (kvl keyValueList) String() string {
	res := strings.Builder{}
	res.WriteString("(")
	for kv := kvl.Front(); kv != nil; kv = kv.Next() {
		res.WriteString(kv.Value.(*driverCommon.KeyValue).String())
		res.WriteString(";")
	}
	res.WriteString(")")
	return res.String()
}

func (kvl keyValueList) Equals(okvl *keyValueList) bool {
	if okvl == nil {
		return false
	}
	if kvl.Len() != okvl.Len() {
		return false
	}
	okv := okvl.Front()
	for e := kvl.Front(); e != nil; e = e.Next() {
		if !e.Value.(*driverCommon.KeyValue).Equals(okv.Value.(*driverCommon.KeyValue)) {
			return false
		}
		okv = okv.Next()
	}
	return true
}

func newKeyValueList() *keyValueList {
	return &keyValueList{List: list.New()}
}
func newPreallocatedKeyValueList(count int) *keyValueList {
	result := &keyValueList{List: list.New()}
	if count > 0 {
		for range count {
			result.PushFront(nil)
		}
	}
	return result
}

func validateAuthKeyValueLength(field string, length driverCommon.SB4, limit int) error {
	if length < 0 {
		common.Odl.Debug("invalid key/value length", "field", field, "length", length, "limit", limit)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "Key/Value")
	}
	if length > driverCommon.SB4(limit) {
		common.Odl.Debug("key/value length exceeds maximum", "field", field, "length", length, "limit", limit)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "Key/Value")
	}
	return nil
}

// MarshalTo marshals a list of key-value pairs.
//
// Parameters:
//   - engine: the marshaller
//
// Returns:
//   - An error if the marshaling operation fails.
func (keyValueList *keyValueList) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	var size int
	for kv := keyValueList.Front(); kv != nil; kv = kv.Next() {
		size = len(kv.Value.(*driverCommon.KeyValue).Key)
		//marshal value size
		err := engine.MarshalUB4(ctx, driverCommon.UB4(size))
		if err != nil {
			return _wrapError(err, "marshal Key/Value")
		}
		if size > 0 {
			//marshal value
			err = engine.MarshalCLR(ctx, kv.Value.(*driverCommon.KeyValue).Key, 0, size)
			if err != nil {
				return _wrapError(err, "marshal Key/Value")
			}
		}

		size = len(kv.Value.(*driverCommon.KeyValue).Value)
		//marshal key size
		err = engine.MarshalUB4(ctx, driverCommon.UB4(size))
		if err != nil {
			return _wrapError(err, "marshal Key/Value")
		}
		if size > 0 {
			//marshal key
			err = engine.MarshalCLR(ctx, kv.Value.(*driverCommon.KeyValue).Value, 0, size)
			if err != nil {
				return _wrapError(err, "marshal Key/Value")
			}
		}

		err = engine.MarshalSB4(ctx, kv.Value.(*driverCommon.KeyValue).Flag)
		if err != nil {
			return _wrapError(err, "marshal Key/Value")
		}
	}
	return nil
}

// UnMarshalFrom unmarshals a keyValueList. Key-value pairs are decoded as
// follows for each key-value pair:
//   - the length of the kay is unmarshalled (SB4)
//   - if the length of the key is greater than zero, the key is unmarshalled
//   - the length of the value is unmarshalled (SB4)
//   - if the length of the value is greater than zero, the value is unmarshalled
//   - the flag is unmarshalled (SB4)
//
// Parameters:
//   - engine: the marshaller
//
// Returns:
//   - An error if the unmarshalling operation fails.
func (keyValueList *keyValueList) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {

	for e := keyValueList.Front(); e != nil; e = e.Next() {
		// Unmarshal key length
		lenKey, err := engine.UnmarshalSB4(ctx)
		if err != nil {
			return _wrapError(err, "unmarshal Key/Value")
		}
		if err = validateAuthKeyValueLength("key", lenKey, maxAuthKeyValueKeyLength); err != nil {
			return _wrapError(err, "unmarshal Key/Value")
		}
		newKv := driverCommon.KeyValue{}
		// Unmarshal key
		if lenKey > 0 {
			key := make([]byte, int(lenKey))
			length, err := engine.UnmarshalCLR(ctx, key, int(lenKey))
			if err != nil {
				return _wrapError(err, "unmarshal Key/Value")
			}
			newKv.Key = key[:length]
		}

		// Unmarshal value length
		lenVal, err := engine.UnmarshalSB4(ctx)
		if err != nil {
			return _wrapError(err, "unmarshal Key/Value")
		}
		if err = validateAuthKeyValueLength("value", lenVal, maxAuthKeyValueValueLength); err != nil {
			return _wrapError(err, "unmarshal Key/Value")
		}
		// Unmarshal value
		if lenVal > 0 {
			value := make([]byte, int(lenVal))
			length, err := engine.UnmarshalCLR(ctx, value, int(lenVal))
			if err != nil {
				return _wrapError(err, "unmarshal Key/Value")
			}
			newKv.Value = value[:length]
		}

		// Unmarshal flags
		flags, err := engine.UnmarshalSB4(ctx)
		if err != nil {
			return _wrapError(err, "unmarshal Key/Value")
		}
		newKv.Flag = flags
		e.Value = &newKv

	}

	return nil
}

type keywordValuePair struct {
	textValue   dynamicAllocatedArray
	binaryValue dynamicAllocatedArray
	keyword     driverCommon.UB2
}

const (
	maxKeywordValueArrayPairs       driverCommon.UB4 = 128 * 10
	maxKeywordValueArrayValueLength driverCommon.UB4 = 64 * 1024 * 10
)

type keywordValueArray []keywordValuePair

func newKeywordValueArray(pairCount driverCommon.UB4) (*keywordValueArray, error) {
	if pairCount > maxKeywordValueArrayPairs {
		common.Odl.Debug("keyword/value pair count exceeds maximum", "pairs", pairCount, "limit", maxKeywordValueArrayPairs)
		return nil, common.NewOracleError(oracleErrors.FailUnmarshal, nil, "keyword/value")
	}
	return new(make(keywordValueArray, pairCount)), nil
}

// MarshalTo Marshals a keyword value list
//  1. Marshals a dynamicAllocatedArray containing the text value
//  2. Marshals a dynamicAllocatedArray containing the binary value
//  3. Marshals the keyword (UB2)
func (keywordValueList keywordValueArray) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	for i := 0; i < len(keywordValueList); i++ {
		err := keywordValueList[i].textValue.MarshalTo(ctx, engine)
		if err != nil {
			return _wrapError(err, "marshal keyword/value")
		}
		err = keywordValueList[i].binaryValue.MarshalTo(ctx, engine)
		if err != nil {
			return _wrapError(err, "marshal keyword/value")
		}
		err = engine.MarshalUB2(ctx, keywordValueList[i].keyword)
		if err != nil {
			return _wrapError(err, "marshal keyword/value")
		}
	}
	return nil
}

// UnMarshalFrom Unmarshals a keyword value list
//  1. Unmarshals a dynamicAllocatedArray containing the text value
//  2. Unmarshals a dynamicAllocatedArray containing the binary value
//  3. Unmarshals the keyword (UB2)
func (keywordValueList keywordValueArray) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	if driverCommon.UB4(len(keywordValueList)) > maxKeywordValueArrayPairs {
		common.Odl.Debug("keyword/value pair count exceeds maximum",
			"pairs", len(keywordValueList), "limit", maxKeywordValueArrayPairs)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "keyword/value")
	}
	for i := 0; i < len(keywordValueList); i++ {
		var textValue dynamicAllocatedArray
		err := textValue.UnMarshalFrom(ctx, engine)
		if err != nil {
			return _wrapError(err, "unmarshal keyword/value")
		}
		var binaryValue dynamicAllocatedArray
		err = binaryValue.UnMarshalFrom(ctx, engine)
		if err != nil {
			return _wrapError(err, "unmarshal keyword/value")
		}
		keyword, err := engine.UnmarshalUB2(ctx)
		if err != nil {
			return _wrapError(err, "unmarshal keyword/value")
		}
		keywordValueList[i].textValue = textValue
		keywordValueList[i].binaryValue = binaryValue
		keywordValueList[i].keyword = keyword
	}
	return nil
}

type dynamicAllocatedArray struct {
	value driverCommon.B1Array
}

// MarshalTo Marshals a dynamic allocated array
//  1. Marshals the length UB4
//  2. Marshals a CLR of that length
func (dalc *dynamicAllocatedArray) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	length := len(dalc.value)
	err := engine.MarshalUB4(ctx, driverCommon.UB4(length))
	if err != nil {
		return _wrapError(err, "marshal value")
	}
	if length != 0 {
		err := engine.MarshalCLR(ctx, dalc.value, 0, length)
		if err != nil {
			return _wrapError(err, "marshal value")
		}
	}
	return nil
}

// UnMarshalFrom Unmarshals a dynamic allocated array
//  1. Unmarshals the maxLength UB4
//  2. Unmarshals a CLR usign the maxLength
func (dalc *dynamicAllocatedArray) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	var length, err = engine.UnmarshalUB4(ctx)
	if err != nil {
		return _wrapError(err, "unmarshal value")
	}
	if length > maxKeywordValueArrayValueLength {
		common.Odl.Debug("dynamic allocated array length exceeds maximum", "length", length, "limit", maxKeywordValueArrayValueLength)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "keyword/value")
	}
	if length > 0 {
		value := make(driverCommon.B1Array, length)
		length, err := engine.UnmarshalCLR(ctx, value, int(length))
		if err != nil {
			return _wrapError(err, "unmarshal value")
		}
		dalc.value = value[:length]
	}

	return nil
}

// getKeyValueFromKeyword translates a TTISPF OCSSYNC keyword/value pair into a session
// property key and a decoded value that can be placed into Properties.
//
// Returned tuple:
//   - key:   the session property name (empty string if the keyword is not handled)
//   - value: the decoded value (usually string), ready to be stored as a property value
//
// Any unhandled keyword returns ("", nil).
func getKeyValueFromKeyword(nlsKeys [64]string, keyword keywordValuePair) (string, any) {
	intValue := keyword.keyword
	binaryValue := keyword.binaryValue
	textValue := keyword.textValue

	if int(intValue) < len(nlsKeys) {
		key := nlsKeys[intValue]
		if key != "" && binaryValue.value != nil {
			return key, driverCommon.B1ArrayToString(binaryValue.value)
		} else if textValue.value != nil {
			return key, textValue
		}
	} else if intValue == al8kwTimezone {
		if len(binaryValue.value) >= 6 {
			v := binaryValue.value

			var regionName string
			// Use region name whenever known to avoid lossy transformation
			if v[2] > ldiRegIDFlag {
				// regid: 7 high bits from v[2] (mask 0x7F) shifted by 6,
				// then 6 low bits from v[3] (mask 0xFC) shifted down by 2
				regid := (int(v[2]) & 0x7F) << 6
				regid += (int(v[3]) & 0xFC) >> 2
				if name, ok := getZoneFromID(regid); ok {
					regionName = name
				}

				hour := v[4] - ldiRegIDSet
				minute := v[5] - ldiMaxTimeField

				var tz string
				if regionName != "" {
					tz = regionName
				} else {
					sign := ""
					if hour > 0 {
						sign = "+"
					}
					tz = fmt.Sprintf("GMT%s%d:%02d", sign, hour, minute)
				}
				return SessionTimeZone, tz
			}
			// No region id, compute GMT offset from hour/minute fields
			hour := v[4] - ldiMaxTimeField
			minute := v[5] - ldiMaxTimeField

			sign := ""
			if hour > 0 {
				sign = "+"
			}
			tz := fmt.Sprintf("GMT%s%d:%02d", sign, hour, minute)
			return SessionTimeZone, tz

		}
	} else if intValue == al8kwPdbElasticPoolLdr {
		// Interpret first 4 bytes as a big-endian int; any value > 0 => "TRUE"
		if len(binaryValue.value) >= 4 {
			n := int32(binary.BigEndian.Uint32(binaryValue.value[:4]))
			isElastic := "FALSE"
			if n > 0 {
				isElastic = "TRUE"
			}
			return al8kwPdbElasticPoolLdrStr, isElastic
		}
	} else if intValue == al8kwPdbAppRoot {
		// PDB App Root flag; first 4 bytes as big-endian int; >0 => TRUE
		if len(binaryValue.value) >= 4 {
			n := int32(binary.BigEndian.Uint32(binaryValue.value[:4]))
			isAppRoot := "FALSE"
			if n > 0 {
				isAppRoot = "TRUE"
			}
			return al8kwPdbAppRootStr, isAppRoot
		}
	} else if intValue == al8kwEnabledRoleNames {
		// Server wires enabled role names as text at OCSSYNC.
		// Use role names for failover restore.
		if len(textValue.value) > 0 {
			roleNames := strings.TrimSpace(driverCommon.B1ArrayToString(textValue.value))
			return al8kwEnabledRoleNamesStr, roleNames
		}
	}

	return "", nil
}
