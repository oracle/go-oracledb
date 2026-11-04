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
** to deal in both the Software and any Larger Work, without restriction.
**
** This license is subject to the condition that the above copyright notice and
** this permission notice be included in all copies or substantial portions of
** the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 */

package ttc

import (
	"errors"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func TestUrowidToString(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "physical",
			data: []byte{
				0x01,                   // type
				0x00, 0x00, 0x00, 0x01, // object
				0x00, 0x01, // file
				0x00, 0x00, 0x00, 0x02, // block
				0x00, 0x03, // slot
			},
			want: "AAAAABAABAAAAACAAD",
		},
		{
			name: "logical",
			data: []byte{0x02, 0x00, 0x01, 0x3d},
			want: "AAAE9",
		},
		{
			name: "remote",
			data: []byte{0x03, 0x00, 0x01, 0x3d},
			want: "*AAE9",
		},
		{
			name: "external",
			data: []byte{0x04, 0x00, 0x01, 0x3d},
			want: "-AAE9",
		},
		{
			name: "not used",
			data: []byte{0x05, 0x00, 0x01, 0x3d},
			want: "(AAE9",
		},
		{
			name: "type 6 external",
			data: []byte{0x06, 0x00, 0x01, 0x3d},
			want: ")AAE9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := urowidToString(tt.data)
			if err != nil {
				t.Fatalf("urowidToString() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("urowidToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUrowidToStringNegative(t *testing.T) {
	data := []byte{
		0x01, // type
		// other fields omitted
	}
	_, err := urowidToString(data)
	if err == nil {
		t.Fatalf("urowidToString() should have raised OGD-00023")
	}
	var sqlErr oracleErrors.SQLError
	ok := errors.As(err, &sqlErr)
	if !ok {
		t.Fatalf("expected SQLError, got%v", sqlErr)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ConverterExpectedFormat) {
		t.Fatalf("expected %s, got %v", oracleErrors.ConverterExpectedFormat, sqlErr)
	}
}
