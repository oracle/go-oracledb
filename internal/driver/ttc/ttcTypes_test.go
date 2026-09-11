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
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// Marshals and unmarshals a keywordValueArray and checks that the sizes and values match
func TestMarshalKeywordValuePairs(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)

	kva := make(keywordValueArray, 2)
	kva[0].textValue.value = []byte{'T', 'e', 's', 't', ' ', 'm', 'a', 'r', 's', 'h', 'a', 'l', ' ', 'D', 'A', 'L', 'C', '1'}
	kva[0].binaryValue.value = []byte{1, 2, 3}
	kva[0].keyword = 0x57F2

	kva[1].textValue.value = []byte{'T', 'e', 's', 't', ' ', 'm', 'a', 'r', 's', 'h', 'a', 'l', ' ', 'D', 'A', 'L', 'C', '2'}
	kva[1].binaryValue.value = []byte{4, 5, 6}
	kva[1].keyword = 0x57F3

	kva.MarshalTo(context.Background(), engine)

	unMarshalledKva := make(keywordValueArray, 2)
	unMarshalledKva.UnMarshalFrom(context.Background(), engine)

	if len(kva) != len(unMarshalledKva) {
		t.Fatalf("Invalid length expected %d, but was %d", len(kva), len(unMarshalledKva))
	}
	for index, item := range kva {

		if len(item.textValue.value) != len(unMarshalledKva[index].textValue.value) {
			t.Fatalf("Invalid length expected %d, but was %d", len(item.textValue.value), len(unMarshalledKva[index].textValue.value))
		}

		for i, value := range item.textValue.value {
			if value != unMarshalledKva[index].textValue.value[i] {
				t.Fatalf("Invalid value at position %d, expected %d, but was %d", i, value, unMarshalledKva[index].textValue.value[i])
			}
		}

		if len(item.binaryValue.value) != len(unMarshalledKva[index].binaryValue.value) {
			t.Fatalf("Invalid length expected %d, but was %d", len(item.binaryValue.value), len(unMarshalledKva[index].binaryValue.value))
		}

		for i, value := range item.binaryValue.value {
			if value != unMarshalledKva[index].binaryValue.value[i] {
				t.Fatalf("Invalid value at position %d, expected %d, but was %d", i, value, unMarshalledKva[index].binaryValue.value[i])
			}
		}

		if item.keyword != unMarshalledKva[index].keyword {
			t.Fatalf("Invalid keyword expected %d, but was %d", item.keyword, unMarshalledKva[index].keyword)
		}
	}
}

// Marshals and unmarshals a keywordValueArray and checks that the sizes and values match
func TestMarshalKeywordValuePairsWithEmptyValues(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)

	kva := make(keywordValueArray, 2)
	kva[0].textValue.value = []byte{'T', 'e', 's', 't', ' ', 'm', 'a', 'r', 's', 'h', 'a', 'l', ' ', 'D', 'A', 'L', 'C', '1'}
	kva[0].binaryValue.value = []byte{}
	kva[0].keyword = 0x57F2

	kva[1].textValue.value = []byte{}
	kva[1].binaryValue.value = []byte{4, 5, 6}
	kva[1].keyword = 0x57F3

	kva.MarshalTo(context.Background(), engine)

	unMarshalledKva := make(keywordValueArray, 2)
	unMarshalledKva.UnMarshalFrom(context.Background(), engine)

	if len(kva) != len(unMarshalledKva) {
		t.Fatalf("Invalid length expected %d, but was %d", len(kva), len(unMarshalledKva))
	}
	for index, item := range kva {

		if len(item.textValue.value) != len(unMarshalledKva[index].textValue.value) {
			t.Fatalf("Invalid length expected %d, but was %d", len(item.textValue.value), len(unMarshalledKva[index].textValue.value))
		}

		for i, value := range item.textValue.value {
			if value != unMarshalledKva[index].textValue.value[i] {
				t.Fatalf("Invalid value at position %d, expected %d, but was %d", i, value, unMarshalledKva[index].textValue.value[i])
			}
		}

		if len(item.binaryValue.value) != len(unMarshalledKva[index].binaryValue.value) {
			t.Fatalf("Invalid length expected %d, but was %d", len(item.binaryValue.value), len(unMarshalledKva[index].binaryValue.value))
		}

		for i, value := range item.binaryValue.value {
			if value != unMarshalledKva[index].binaryValue.value[i] {
				t.Fatalf("Invalid value at position %d, expected %d, but was %d", i, value, unMarshalledKva[index].binaryValue.value[i])
			}
		}

		if item.keyword != unMarshalledKva[index].keyword {
			t.Fatalf("Invalid keyword expected %d, but was %d", item.keyword, unMarshalledKva[index].keyword)
		}
	}
}

func TestKeywordValueArrayRejectsOversizedPairCount(t *testing.T) {
	t.Parallel()
	_, err := newKeywordValueArray(maxKeywordValueArrayPairs + 1)
	if err == nil {
		t.Fatal("expected oversized keyword/value pair count to be rejected")
	}
}

func TestKeywordValueArrayRejectsOversizedDynamicValue(t *testing.T) {
	t.Parallel()
	_, engine := newMarshalEngine(common.BIG_ENDIAN, B2, Universal, 1000)
	if err := engine.MarshalUB4(context.Background(), maxKeywordValueArrayValueLength+1); err != nil {
		t.Fatalf("MarshalUB4 failed: %v", err)
	}

	kva, err := newKeywordValueArray(1)
	if err != nil {
		t.Fatalf("newKeywordValueArray failed: %v", err)
	}

	err = kva.UnMarshalFrom(context.Background(), engine)
	if err == nil {
		t.Fatal("expected oversized dynamic value to be rejected")
	}
}

func _newKeyValPairs() *keyValueList {
	l := newKeyValueList()
	l.PushBack(&common.KeyValue{
		Key:   []byte("key1"),
		Value: []byte("value1"),
		Flag:  1,
	})
	l.PushBack(&common.KeyValue{
		Key:   []byte("key2"),
		Value: []byte("value2"),
		Flag:  0,
	})
	return l
}
func _newEmptyKeyValPairs() *keyValueList {
	l := newKeyValueList()
	l.PushBack(&common.KeyValue{
		Key:   []byte{},
		Value: []byte{},
		Flag:  1,
	})
	l.PushBack(&common.KeyValue{
		Key:   []byte{},
		Value: []byte{},
		Flag:  0,
	})
	return l
}
func _newNilKeyValPairs() *keyValueList {
	l := newKeyValueList()
	l.PushBack(&common.KeyValue{
		Key:   nil,
		Value: nil,
		Flag:  1,
	})
	l.PushBack(&common.KeyValue{
		Key:   nil,
		Value: nil,
		Flag:  0,
	})
	return l
}

// Tests that key-value pairs are correctly encoded using different values
func TestAuthRPARejectsOversizedKeyValueListAllocations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		newRPA       func() common.UnMarshallable
		pairCount    common.UB2
		writePayload func(context.Context, *MarshalEngine) error
	}{
		{
			name:      "oauth huge key length",
			newRPA:    func() common.UnMarshallable { return NewOAuthRPA().(common.UnMarshallable) },
			pairCount: 1,
			writePayload: func(ctx context.Context, engine *MarshalEngine) error {
				return engine.MarshalSB4(ctx, common.SB4(maxAuthKeyValueKeyLength+1))
			},
		},
		{
			name:      "oauth huge value length",
			newRPA:    func() common.UnMarshallable { return NewOAuthRPA().(common.UnMarshallable) },
			pairCount: 1,
			writePayload: func(ctx context.Context, engine *MarshalEngine) error {
				if err := engine.MarshalSB4(ctx, 0); err != nil {
					return err
				}
				return engine.MarshalSB4(ctx, common.SB4(maxAuthKeyValueValueLength+1))
			},
		},
		{
			name:         "oauth huge pair count",
			newRPA:       func() common.UnMarshallable { return NewOAuthRPA().(common.UnMarshallable) },
			pairCount:    common.UB2(maxAuthKeyValuePairs + 1),
			writePayload: func(context.Context, *MarshalEngine) error { return nil },
		},
		{
			name:      "osesskey huge key length",
			newRPA:    func() common.UnMarshallable { return NewOSesskeyRPA().(common.UnMarshallable) },
			pairCount: 1,
			writePayload: func(ctx context.Context, engine *MarshalEngine) error {
				return engine.MarshalSB4(ctx, common.SB4(maxAuthKeyValueKeyLength+1))
			},
		},
		{
			name:      "osesskey huge value length",
			newRPA:    func() common.UnMarshallable { return NewOSesskeyRPA().(common.UnMarshallable) },
			pairCount: 1,
			writePayload: func(ctx context.Context, engine *MarshalEngine) error {
				if err := engine.MarshalSB4(ctx, 0); err != nil {
					return err
				}
				return engine.MarshalSB4(ctx, common.SB4(maxAuthKeyValueValueLength+1))
			},
		},
		{
			name:         "osesskey huge pair count",
			newRPA:       func() common.UnMarshallable { return NewOSesskeyRPA().(common.UnMarshallable) },
			pairCount:    common.UB2(maxAuthKeyValuePairs + 1),
			writePayload: func(context.Context, *MarshalEngine) error { return nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, 0, 0, 1000)
			ctx := context.Background()
			if err := engine.MarshalUB2(ctx, tt.pairCount); err != nil {
				t.Fatalf("failed to marshal pair count: %v", err)
			}
			if err := tt.writePayload(ctx, engine); err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			dataBuffer.currentReadPosition = 0
			if err := tt.newRPA().UnMarshalFrom(ctx, engine); err == nil {
				t.Fatalf("expected oversized auth RPA key/value payload to be rejected")
			}
		})
	}
}

// Tests that key-value pairs are correctly encoded using different values
func TestMarshalKeyValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		keyValuePairs *keyValueList
	}{
		{
			name:          "valid key-value pairs",
			keyValuePairs: _newKeyValPairs(),
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newEmptyKeyValPairs(),
		},
		{
			name:          "empty keys and values",
			keyValuePairs: _newNilKeyValPairs(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBuffer, engine := newMarshalEngine(common.BIG_ENDIAN, 0, 0, 150000)
			err := ((common.Marshallable)(tt.keyValuePairs)).MarshalTo(context.Background(), engine)
			if err != nil {
				t.Errorf("MarshalKeyValue failed: %v", err)
			}

			// Verify the marshaled data
			dataBuffer.currentReadPosition = 0
			for e := tt.keyValuePairs.Front(); e != nil; e = e.Next() {
				kv := e.Value.(*common.KeyValue)
				keySize, err := engine.UnmarshalUB4(context.Background())
				if err != nil {
					t.Errorf("Errror: %s", err.Error())
				}
				if int(keySize) != len(kv.Key) {
					t.Errorf("key size mismatch: got %d, want %d", keySize, len(kv.Key))
				}
				if keySize > 0 {
					key := make([]byte, keySize)
					readKeySize, err := engine.UnmarshalCLR(context.Background(), (common.B1Array)(key), int(keySize))
					if err != nil {
						t.Errorf("Errror: %s", err.Error())
					}
					if int(keySize) != readKeySize {
						t.Errorf("Wrong value of bytes read %d", readKeySize)
					}
					if key == nil {
						t.Error("Key is null")
					}
				}

				valueSize, err := engine.UnmarshalUB4(context.Background())
				if err != nil {
					t.Errorf("Errror: %s", err.Error())
				}
				if int(valueSize) != len(kv.Value) {
					t.Errorf("value size mismatch: got %d, want %d", valueSize, len(kv.Value))
				}
				if valueSize > 0 {
					value := make([]byte, valueSize)
					readValueSize, err := engine.UnmarshalCLR(context.Background(), (common.B1Array)(value), int(valueSize))
					if err != nil {
						t.Errorf("Errror: %s", err.Error())
					}
					if int(valueSize) != readValueSize {
						t.Errorf("Wrong value of bytes read %d", readValueSize)
					}
					if value == nil {
						t.Error("Key is null")
					}
				}

				keyvalflagValue, err := engine.UnmarshalUB4(context.Background())
				if err != nil {
					t.Errorf("Errror: %s", err.Error())
				}
				if uint32(keyvalflagValue) != uint32(kv.Flag) {
					t.Errorf("keyvalflag mismatch: got %d, want %d", keyvalflagValue, kv.Flag)
				}
			}
		})
	}
}
