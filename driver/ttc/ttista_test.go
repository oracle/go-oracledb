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
)

// Test_newTTISTA test that _supportsEndOfCallStatus is not set when newTTISTA
// is called and checks the message code
func Test_newTTISTA(t *testing.T) {
	t.Parallel()
	msg := newTTISTA()
	sta, ok := msg.(*ttiSTA)
	if !ok {
		t.Fatal("newTTISTA did not return *ttiSTA")
	}
	if sta._supportsEndOfCallStatus {
		t.Error("expected _supportsEndOfCallStatus to be false")
	}
	if code := sta.GetMsgCode(); code != TTISTA {
		t.Errorf("expected TTISTA, got %v", code)
	}
}

// Test_newTTISTAWithEndOfCallStatusSupport test that _supportsEndOfCallStatus
// is set when newTTISTAWithEndOfCallStatusSupport is called and checks the
// message code
func Test_newTTISTAWithEndOfCallStatusSupport(t *testing.T) {
	t.Parallel()
	msg := newTTISTAWithEndOfCallStatusSupport()
	sta, ok := msg.(*ttiSTA)
	if !ok {
		t.Fatal("newTTISTAWithEndOfCallStatusSupport did not return *ttiSTA")
	}
	if !sta._supportsEndOfCallStatus {
		t.Error("expected _supportsEndOfCallStatus to be true")
	}
	if code := sta.GetMsgCode(); code != TTISTA {
		t.Errorf("expected TTISTA, got %v", code)
	}
}

// Creates a ttiSTA without eoc support and checks that the ECID sequence number
// is unmarshalled correctly
func Test_ttiSTA_UnMarshalFrom_WithoutSupport(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: false}
	buffer := []byte{
		0x01, 0x2A, // endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 0, 0)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if sta.eocStatus != nil {
		t.Error("expected eocStatus to be nil")
	}
	if sta.endToEndECIDSequenceNumber != 42 {
		t.Errorf("expected endToEndECIDSequenceNumber 42, got %v", sta.endToEndECIDSequenceNumber)
	}
}

// Creates a ttiSTA with eoc support and checks that the ECID sequence number
// containing the elapsed time and checks that values for elapsed time and ECID
// sequence number are unmarshalled correctly
func Test_ttiSTA_UnMarshalFrom_WithSupport(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: true}
	buffer := []byte{
		0x01, 0x08, // UB4 TtiEocEct
		0x01, 0x64, // UB8 elapsed time
		0x01, 0x2A, // UB2 endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 0, 0)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if sta.eocStatus == nil {
		t.Fatal("expected eocStatus not nil")
	}
	if sta.eocStatus.elapsedTime != 100 {
		t.Errorf("expected elapsedTime 100, got %v", sta.eocStatus.elapsedTime)
	}
	if sta.eocStatus.connectionShouldBeDropped {
		t.Error("expected drop false")
	}
	if sta.endToEndECIDSequenceNumber != 42 {
		t.Errorf("expected endToEndECIDSequenceNumber 42, got %v", sta.endToEndECIDSequenceNumber)
	}
}

// Creates a ttiSTA with eoc support and checks that the ECID sequence number
// containing the elapsed time and checks that values for elapsed time and ECID
// sequence number and drop are unmarshalled correctly
func Test_ttiSTA_UnMarshalFrom_WithSupport_DropFlag(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: true}
	buffer := []byte{
		0x02, 0x08, 0x08, // UB4 TtiEocEct | TtiEocfDropWhenReturned
		0x01, 0x00, // UB8 elapsed time
		0x01, 0x2A, // UB2 endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 0, 0)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if sta.eocStatus == nil {
		t.Fatal("expected eocStatus not nil")
	}
	if sta.eocStatus.elapsedTime != 0 {
		t.Errorf("expected elapsedTime 0, got %v", sta.eocStatus.elapsedTime)
	}
	if !sta.eocStatus.connectionShouldBeDropped {
		t.Error("expected drop true")
	}
	if sta.endToEndECIDSequenceNumber != 42 {
		t.Errorf("expected endToEndECIDSequenceNumber 42, got %v", sta.endToEndECIDSequenceNumber)
	}
}

// Creates a ttiSTA without eoc support and checks that an error is returned when
// unmarshalling fails
func Test_ttiSTA_UnMarshalFrom_ErrorInUB2(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: false}
	buffer := []byte{
		0x01, 0x2A, // UB2 endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 1, 1)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// Creates a ttiSTA with eoc support and checks that an error is returned when
// unmarshalling the flag fails
func Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInEOCS(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: true}
	buffer := []byte{
		0x02, 0x08, 0x08, // UB4 TtiEocEct | TtiEocfDropWhenReturned
		0x01, 0x00, // UB8 elapsed time
		0x01, 0x2A, // UB2 endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 1, 1)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// Creates a ttiSTA without eoc support and checks that an error is returned when
// unmarshalling elapsed time fails
func Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInElapsedTime(t *testing.T) {
	t.Parallel()
	sta := &ttiSTA{_supportsEndOfCallStatus: true}
	buffer := []byte{
		0x02, 0x08, 0x08, // UB4 TtiEocEct | TtiEocfDropWhenReturned
		0x01, 0x64, // UB8 elapsed time
		0x01, 0x2A, // UB2 endToEndECIDSequenceNumber = 42
	}
	mockEngine := createMarshaller(buffer, 1, 2)
	err := sta.UnMarshalFrom(context.Background(), mockEngine)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func Test_ttiSTA_getConnectionShouldBeDropped(t *testing.T) {
	t.Parallel()
	msg := &ttiSTA{
		_supportsEndOfCallStatus: false,
		eocStatus:                nil,
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &ttiSTA{
		_supportsEndOfCallStatus: true,
		eocStatus:                nil,
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &ttiSTA{
		_supportsEndOfCallStatus: true,
		eocStatus: &endOfCallStatus{
			elapsedTime:               0,
			connectionShouldBeDropped: false,
		},
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &ttiSTA{
		_supportsEndOfCallStatus: true,
		eocStatus: &endOfCallStatus{
			elapsedTime:               0,
			connectionShouldBeDropped: true,
		},
	}
	if !msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
}
