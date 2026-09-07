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
	"database/sql/driver"
	"reflect"
	"testing"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
	"golang.org/x/text/language"
)

// 1. Create a new shelf and check that it is not nil
// 2. Ensure maps are nil by default (until registered)
func TestTTIShelf_NewShelf(t *testing.T) {
	t.Parallel()
	shelf := newShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	if shelf.Shelf == nil {
		t.Fatal("embedded common shelf should not be nil")
	}

	if shelf.GetCodecFactory() != nil {
		t.Fatal("Codec Factory should be nil by default")
	}

	if shelf.getEventService() == nil {
		t.Fatal("event service should not be nil")
	}

	if shelf.getEventService() != shelf.getEventService() {
		t.Fatal("event service should be kept on the shelf")
	}
}

// TestNewMessageStreamerRegistersConnectionValidator verifies that a newly
// created message streamer participates in shelf connection validation.
func TestNewMessageStreamerRegistersConnectionValidator(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	streamer := NewMessageStreamer(shelf)
	validators := shelf._validatorRegistry.GetAll()

	if len(validators) != 1 {
		t.Fatalf("validator count = %d, want 1", len(validators))
	}
	if validators[0] != streamer {
		t.Fatalf("registered validator = %p, want streamer %p", validators[0], streamer)
	}
}

type shelfConnectionValidator struct {
	valid bool
}

func (s *shelfConnectionValidator) isValid(context.Context) bool {
	return s.valid
}

type recordingConnectionValidator struct {
	name   string
	valid  bool
	called *[]string
}

func (v *recordingConnectionValidator) isValid(context.Context) bool {
	*v.called = append(*v.called, v.name)
	return v.valid
}

// TestTTIShelf_ValidateConnection verifies that the shelf reports invalid
// connections and accepts shelves with no invalid validators.
func TestTTIShelf_ValidateConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		validator stateValidator
		wantError bool
	}{
		{name: "no validators", wantError: false},
		{name: "valid connection", validator: &shelfConnectionValidator{valid: true}, wantError: false},
		{name: "invalid connection", validator: &shelfConnectionValidator{valid: false}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shelf := newShelf[driverCommon.MessageType]()
			if tt.validator != nil {
				shelf.registerStateValidator(tt.validator)
			}

			err := shelf.checkCurrentState(context.Background())
			if (err != nil) != tt.wantError {
				t.Fatalf("error presence = %t, want %t; error = %v", err != nil, tt.wantError, err)
			}
			if tt.wantError {
				sqlErr, ok := err.(oracleErrors.SQLError)
				if !ok || sqlErr.ErrorCode() != string(oracleErrors.InternalError) {
					t.Fatalf("error = %T %v, want InternalError", err, err)
				}
			}
		})
	}
}

// TestTTIShelf_ValidateConnectionStopsAtFirstInvalidValidator verifies that
// validation preserves registration order and short-circuits on failure.
func TestTTIShelf_ValidateConnectionStopsAtFirstInvalidValidator(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	called := []string{}
	shelf.registerStateValidator(&recordingConnectionValidator{name: "first", valid: true, called: &called})
	shelf.registerStateValidator(&recordingConnectionValidator{name: "second", valid: false, called: &called})
	shelf.registerStateValidator(&recordingConnectionValidator{name: "third", valid: true, called: &called})

	err := shelf.checkCurrentState(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got, want := called, []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("validators called = %v, want %v", got, want)
	}
}

// 1. Register codec factory and verify GetCodecFactory returns it
// 2. Re-register factory and verify it is replaced
func TestTTIShelf_RegisterCodecFactoryAndGetter(t *testing.T) {
	t.Parallel()
	shelf := newShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	factory1 := &testCodecFactory{
		encode:    nil,
		decode:    nil,
		bindOac:   nil,
		defineOac: nil,
	}

	ret := shelf.RegisterCodecFactory(factory1)
	if ret != shelf {
		t.Fatal("RegisterCodecFactory should return shelf for chaining")
	}
	if got, ok := shelf.GetCodecFactory().(*testCodecFactory); !ok || got != factory1 {
		t.Fatal("codec factory should match the registered factory")
	}

	factory2 := &testCodecFactory{
		encode:    nil,
		decode:    nil,
		bindOac:   nil,
		defineOac: nil,
	}

	shelf.RegisterCodecFactory(factory2)

	if got, ok := shelf.GetCodecFactory().(*testCodecFactory); !ok || got != factory2 {
		t.Fatal("codec factory should be replaced with factory2")
	}
}

// TestTTIShelf_LocalizedStatementExecError verifies statement-level boundary
// methods localize errors before returning them to callers.
func TestTTIShelf_LocalizedStatementExecError(t *testing.T) {
	t.Parallel()

	mockStr := &mockStreamer{}

	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterLocalizationService(internalCommon.NewLocalizationService(language.French))
	shelf.RegisterMessageStreamer(mockStr)

	stmt := &Statement{
		shelf: shelf,
		execStatementExecutor: execContextFunc(func(context.Context, *qualifiedSQLStatement, []driver.NamedValue) (driver.Result, error) {
			return nil, internalCommon.NewOracleError(oracleErrors.InternalError, nil)
		}),
	}

	_, err := stmt.ExecContext(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if sqle, ok := err.(oracleErrors.SQLError); !ok {
		t.Fatalf("expected SQLError, got %T", err)
	} else if sqle.ErrorCode() != string(oracleErrors.InternalError) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.InternalError, sqle.ErrorCode())
	}
	if got, want := err.Error(), "OGD-00062 - erreur interne factice."; got != want {
		t.Fatalf("unexpected localized error %q, want %q", got, want)
	}
}

// TestTTIShelf_RegisterProviderRegistry stores a provider registry on the shelf
// and verifies the same instance is returned by the getter.
func TestTTIShelf_RegisterProviderRegistry(t *testing.T) {
	t.Parallel()

	shelf := newShelf[int]()
	registry := internalCommon.NewSafeRegistry[oracleProviders.Provider]()

	shelf.registerProviderRegistry(registry)

	if got := shelf.getProviderRegistry(); got != registry {
		t.Fatalf("expected provider registry %p, got %p", registry, got)
	}
}

// TestTTIShelf_RegisterProviderRegistry_ReplacesExistingRegistry verifies that
// registering a second provider registry replaces the previous one on the shelf.
func TestTTIShelf_RegisterProviderRegistry_ReplacesExistingRegistry(t *testing.T) {
	t.Parallel()

	shelf := newShelf[int]()
	firstRegistry := internalCommon.NewSafeRegistry[oracleProviders.Provider]()
	secondRegistry := internalCommon.NewSafeRegistry[oracleProviders.Provider]()

	shelf.registerProviderRegistry(firstRegistry)
	shelf.registerProviderRegistry(secondRegistry)

	if got := shelf.getProviderRegistry(); got != secondRegistry {
		t.Fatalf("expected replacement provider registry %p, got %p", secondRegistry, got)
	}
}

type testCodecFactory struct {
	encode    driverCommon.B1Array
	decode    driver.Value
	bindOac   driverCommon.Marshallable
	defineOac driverCommon.Marshallable
}

func (t *testCodecFactory) getEncoder(_ normalizedBindValue) (encoderFunc, error) {
	return func(driver.Value) (driverCommon.B1Array, error) { return t.encode, nil }, nil
}
func (t *testCodecFactory) getDecoder(_ DtyType) (*typeDecoder, error) {
	return newTypeDecoder(func(columnContext, driverCommon.B1Array) (driver.Value, error) {
		return t.decode, nil
	}, nil), nil
}
func (t *testCodecFactory) getBindOac(_ normalizedBindValue, _ driverCommon.UB4) (driverCommon.Marshallable, error) {
	return t.bindOac, nil
}
func (t *testCodecFactory) getDefineOac(_ DtyType, _ columnContext, _ driverCommon.DriverProperties) driverCommon.Marshallable {
	return t.defineOac
}

// TestTTIShelf_StatementDrain test GetStatements API
// expectations:
//   - it returns all statements previously added
//   - second call return an empty slice
func TestTTIShelf_StatementDrain(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	sessCtx := driverCommon.NewSessionContext()
	s1, _ := newStatement(shelf, sessCtx, "SELECT * FROM DUAL")
	s2, _ := newStatement(shelf, sessCtx, "SELECT * FROM DUAL")
	s3, _ := newStatement(shelf, sessCtx, "SELECT * FROM DUAL")
	statements := []*Statement{s1, s2, s3}

	shelf.AddStatement(s1)
	shelf.AddStatement(s2)
	shelf.AddStatement(s3)

	drainedOnes := shelf.GetStatements(false)
	var found uint = 0
	for _, st := range drainedOnes {
		if s1 == st {
			found++
		}
		if s2 == st {
			found++
		}
		if s3 == st {
			found++
		}
	}

	if found != 3 {
		for _, st := range drainedOnes {
			t.Logf("drained stmt [%+v]", st)
		}
		for _, st := range statements {
			t.Logf("statements stmt [%+v]", st)
		}
		t.Fatalf("Statements do not match expected [%d] [%+v], got [%+v]", found, statements, drainedOnes)
	}

	emptyOnes := shelf.GetStatements(true)
	if len(emptyOnes) == 0 {
		t.Fatalf("statements in the shelf should not have been drained")
	}
}
