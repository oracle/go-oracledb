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
	"database/sql"
	"database/sql/driver"
	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	max_lob_length           = 4000
	define_maxlength_scalar  = 2147483647
	define_maxlength_varchar = 32767 * 4
	uacflsz                  = 0x02000000  // oacmxlc holds LOB prefetch size
	uacfsald                 = 0x800000000 // Send all LOB data for VBL
)

/*
codecFactory selects encoder, decoder, and OAC implementations based on TTC protocol version and Go types.

Description:

	A codecFactory provides lookup methods for:
	- encoders keyed by the Go reflect.Type (used when sending bind values),
	- decoders keyed by Oracle database type id (used when scanning result sets),
	- OAC (Oracle Attribute/Argument/Column) descriptor makers keyed by Go reflect.Type.
	The implementation chooses the most suitable candidate for the negotiated TTC protocol version.

	Example:
		// Construct a factory for the negotiated TTC version (example: v20).
		f := NewCodecFactoryForProtocol(20)

		// Bind/encode path: resolve OAC + encoder for a Go value.
		bindValue := "hello"

		enc, _ := f.getEncoder(bindValue)
		wireBytes, _ := enc.encodeToType(enc.encodeValue)
		_ = wireBytes // send over TTC

		oac, _ := f.getBindOac(bindValue, common.UB4(len(wireBytes)))
		_ = oac // e.g. used to describe bind metadata (type/length) in TTC messages

		// Row/scan path: resolve a decoder for an Oracle database type id.
		decoder, _ := f.getDecoder(DtyVCS)
		v, _ := decoder.decodeToType(columnContext{Name: "C1", Index: 0}, wireBytes)
		_ = v // decoded driver.Value (string, number, time.Time, etc.)
*/
type codecFactory interface {
	getEncoder(normalizedBindValue) (encoderFunc, error)
	getDecoder(DtyType) (*typeDecoder, error)
	getBindOac(normalizedBindValue, driverCommon.UB4) (driverCommon.Marshallable, error)
	getDefineOac(DtyType, columnContext, driverCommon.DriverProperties) driverCommon.Marshallable
}

// encoderFunc defines the function signature of encoder functions.
// This function is used during registration to instantiate new encoders.
// implementor.
type encoderFunc func(driver.Value) (driverCommon.B1Array, error)

// decoderFunc defines the signature for TTC decoder implementors registered with the
// codec factory. The function receives the column context and raw TTC data bytes and is
// expected to return the decoded database value or an error.
type decoderFunc func(columnContext, driverCommon.B1Array) (driver.Value, error)

// bindOacFunc defines the signature for bind OAC constructor functions registered
// with the codec factory. The function receives the requested maximum bind length
// and must return a TTC OAC descriptor suitable for marshalling bind metadata.
type bindOacFunc func(driverCommon.UB4) driverCommon.Marshallable

// bindOacType stores a bind OAC constructor together with the default max-length
// and scale metadata that should be applied for OUT bind handling.
type bindOacType struct {
	bindOacFunc
	maxLength driverCommon.UB4
	scale     driverCommon.SB1
}

// defineOacFunc defines the signature for define OAC constructor functions
// registered with the codec factory. The function receives the result-set column
// context plus the effective LOB prefetch size and must return a TTC OAC
// descriptor suitable for marshalling define/fetch metadata.
type defineOacFunc func(columnContext, driverCommon.UB4) driverCommon.Marshallable

type scanTypeFunc func(columnContext) reflect.Type

type typeDecoder struct {
	decodeToType decoderFunc
	getScanType  scanTypeFunc
}

func newTypeDecoder(f decoderFunc, sf scanTypeFunc) *typeDecoder {
	_n := &typeDecoder{}
	_n.decodeToType = f
	_n.getScanType = sf
	return _n
}

/*
normalizedBindValue stores the canonical bind metadata derived from an input bind value.

Description:
  - Captures the effective Go type used for registry lookup after unwrapping sql.Out
    and dereferencing pointer values.
  - Stores the normalized driver.Value that should be passed to the selected encoder.
  - Records whether the original bind was an sql.Out and whether it represents a pure
    OUT bind (sql.Out with In == false), which affects encoder and bind OAC behavior.
*/
type normalizedBindValue struct {
	goType    reflect.Type
	value     driver.Value
	isOut     bool
	isOutOnly bool
}

/*
normalizeBindValue converts a raw bind value into the canonical form used by codec
selection and bind metadata lookup.

Description:
  - Detects sql.Out binds, switching lookup to the destination value type and tracking
    whether the bind is OUT-only or IN/OUT.
  - Dereferences pointer values so encoder and bind OAC registries can match on the
    underlying concrete Go type instead of the pointer type.
  - Converts nil pointers into a nil value with no Go type so downstream lookup logic
    can handle null binds consistently.

Parameters:
  - bindValue: the raw bind value supplied by statement execution code.

Returns:
  - normalizedBindValue: the normalized bind metadata used by getEncoder and getBindOac.
*/
func normalizeBindValue(bindValue driver.Value) normalizedBindValue {
	n := normalizedBindValue{
		goType: reflect.TypeOf(bindValue),
		value:  bindValue,
	}

	if outValue, ok := bindValue.(sql.Out); ok {
		n.isOut = true
		n.value = outValue.Dest
		n.goType = reflect.TypeOf(outValue.Dest)
		n.isOutOnly = !outValue.In
	}

	if n.goType != nil && n.goType.Kind() == reflect.Ptr {
		rv := reflect.ValueOf(n.value)
		if !rv.IsValid() || rv.IsNil() {
			n.value = nil
			n.goType = nil
		} else {
			n.value = rv.Elem().Interface()
			n.goType = n.goType.Elem()
		}
	}

	switch value := n.value.(type) {
	case sql.NullString:
		if value.Valid {
			n.value = value.String
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.String)
	case sql.NullInt64:
		if value.Valid {
			n.value = value.Int64
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Int64)
	case sql.NullInt32:
		if value.Valid {
			n.value = value.Int32
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Int32)
	case sql.NullInt16:
		if value.Valid {
			n.value = value.Int16
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Int16)
	case sql.NullByte:
		if value.Valid {
			n.value = value.Byte
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Byte)
	case sql.NullFloat64:
		if value.Valid {
			n.value = value.Float64
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Float64)
	case sql.NullBool:
		if value.Valid {
			n.value = value.Bool
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Bool)
	case sql.NullTime:
		if value.Valid {
			n.value = value.Time
		} else {
			n.value = nil
		}
		n.goType = reflect.TypeOf(value.Time)
	}

	return n
}

/*
codecRegistryEntry represents a single registered implementation candidate.

Description:

	Each entry stores:
	- makeFunc: the concrete factory/function to use for encoding/decoding/OAC creation,
	- fromTTCProtocolVersion: the minimum TTC protocol version (inclusive) where the candidate is valid.
	Candidate selection prefers the entry with the highest fromTTCProtocolVersion that is <= the negotiated
	version, allowing newer implementations to override older ones while keeping backwards compatibility.
*/
type codecRegistryEntry[F any] struct {
	makeFunc               F
	fromTTCProtocolVersion int8
}

/*
codecRegistry is a version-aware registry for codec-related implementations.

Description:

	codecRegistry stores one or more candidates per key K (for example, Go reflect.Type or Oracle db type id).
	Multiple candidates are supported to allow different implementations across TTC protocol versions.
	Selection is performed by helper functions such as getBestEncoder/getBestDecoder/getBestBindOac/getBestDefineOac.
*/
type codecRegistry[K comparable, F any] struct {
	entries map[K][]codecRegistryEntry[F]
}

/*
newCodecRegistry constructs an empty codecRegistry.

Description:

	Creates a registry keyed by K and storing candidates of type F.

Parameters:
  - None.

Returns:
  - *codecRegistry[K, F]: A registry instance with an initialized internal map.
*/
func newCodecRegistry[K comparable, F any]() *codecRegistry[K, F] {
	return &codecRegistry[K, F]{
		entries: make(map[K][]codecRegistryEntry[F]),
	}
}

/*
Register registers an implementation candidate for a key starting at a TTC protocol version.

Description:

	Adds a new candidate to the registry for the given key and minimum TTC protocol version (inclusive).
	If an entry already exists for the same key and fromTTCProtocolVersion, it is replaced; otherwise the
	entry is appended.

Parameters:
  - key: Registry key (for example, a Go reflect.Type or an Oracle database type id).
  - fromTTCProtocolVersion: Minimum TTC protocol version (inclusive) for which the candidate is valid.
  - f: The implementation function/factory to register.

Returns:
  - error: Currently always nil.

Errors:
  - None.
*/
func (r *codecRegistry[K, F]) Register(
	key K,
	fromTTCProtocolVersion int8,
	f F,
) error {
	item := codecRegistryEntry[F]{f, fromTTCProtocolVersion}

	// Replace any existing item with the same fromTTCProtocolVersion; otherwise append.
	foundIndex := -1
	for i, it := range r.entries[key] {
		if it.fromTTCProtocolVersion == fromTTCProtocolVersion {
			foundIndex = i
			break
		}
	}
	if foundIndex == -1 {
		r.entries[key] = append(r.entries[key], item)
	} else {
		r.entries[key][foundIndex] = item
	}
	return nil
}

/*
getCandidates returns the list of registered candidates for a key.

Description:

	Retrieves all registered candidates for a given key. If the key is not present, nil is returned.

Parameters:
  - key: Registry key to look up.

Returns:
  - []codecRegistryEntry[F]: Slice of candidates for the key, or nil if none exist.

Errors:
  - None.
*/
func (r *codecRegistry[K, F]) getCandidates(key K) []codecRegistryEntry[F] {
	if candidates, ok := r.entries[key]; ok {
		return candidates
	}
	return nil
}

// EncoderRegistry is the global registry for encoders keyed by Go type.
// Populate it during package init of encoder implementors.
var EncoderRegistry = newCodecRegistry[reflect.Type, encoderFunc]()

// DecoderRegistry is the global registry for decoders keyed by Oracle database type.
var DecoderRegistry = newCodecRegistry[DtyType, *typeDecoder]()

// BindOacRegistry is the global registry for bind OAC metadata keyed by Go type.
var BindOacRegistry = newCodecRegistry[reflect.Type, bindOacType]()

// DefineOacRegistry is the global registry for define OAC instances keyed by Oracle database type,
// column context and connection properties.
var DefineOacRegistry = newCodecRegistry[DtyType, defineOacFunc]()

/*
CodecFactoryImpl is the codecFactory implementation used by TTC to resolve encoders/decoders/OAC makers.

Description:

	Uses the negotiated TTC protocol version to select the "best" registered implementation candidate from:
	- EncoderRegistry (Go reflect.Type -> encoderFunc)
	- DecoderRegistry (Oracle db type id -> decoderFunc)
	- BindOacRegistry (Go reflect.Type -> bindOacType)
	- DefineOacRegistry (Oracle db type id -> defineOacFunc)
*/
type CodecFactoryImpl struct {
	ttcVersion int8
	encoders   *codecRegistry[reflect.Type, encoderFunc]
	decoders   *codecRegistry[DtyType, *typeDecoder]
	bindOacs   *codecRegistry[reflect.Type, bindOacType]
	defineOacs *codecRegistry[DtyType, defineOacFunc]
}

/*
NewCodecFactoryForProtocol constructs a codecFactory for a negotiated TTC protocol version.

Description:

	Returns a factory that resolves encoders/decoders/OAC makers using the default global registries
	(EncoderRegistry, DecoderRegistry, OacRegistry) and the provided TTC protocol version.

Parameters:
  - protocolVersion: Negotiated TTC protocol version.

Returns:
  - codecFactory: A factory implementation suitable for the given protocol version.

Errors:
  - None.
*/
func NewCodecFactoryForProtocol(protocolVersion int8) codecFactory {
	return &CodecFactoryImpl{
		ttcVersion: protocolVersion,
		encoders:   EncoderRegistry,
		decoders:   DecoderRegistry,
		bindOacs:   BindOacRegistry,
		defineOacs: DefineOacRegistry,
	}
}

/*
getEncoder returns the encoder function for a normalized bind value.

Description:

	Selects the best registered encoder candidate for the bind value type based on the factory's negotiated TTC
	protocol version. If the bind value is a driver.Out and In is false, NullEncoder is returned. If the bind
	value is a driver.Out and In is true, encoder lookup is performed using the concrete reflect.Type of Out.Dest.

Parameters:
  - normalized: The normalized bind value to encode.

Returns:
  - encoderFunc: The selected encoder implementation.
  - error: Non-nil if no compatible encoder is registered.

Errors:
  - Returns a common.OracleError with code common.InternalError when no encoder candidate exists for the bind type.
*/
func (f *CodecFactoryImpl) getEncoder(normalized normalizedBindValue) (encoderFunc, error) {
	if normalized.isOutOnly || normalized.value == nil {
		return converters.EncodeNull, nil
	}
	common.Odl.Debug("New encoder requested", "goType", normalized.goType)

	candidates := f.encoders.getCandidates(normalized.goType)
	bestCandidate := getEntryFromRegistry(f.ttcVersion, candidates)
	if bestCandidate != nil {
		common.Odl.Debug("Encoder returned", "candidate", bestCandidate)
		return bestCandidate.makeFunc, nil
	}

	err := common.NewOracleError(oracleErrors.InternalError, nil)
	common.Odl.Error("Encoder candidate missing", "goType", normalized.goType, "error", err)
	return nil, err
}

/*
getDecoder returns the typeDecoder for an Oracle database type id.

Description:

	Selects the best registered decoder candidate for dbType based on the factory's negotiated TTC protocol
	version. Candidate selection prefers the most recent registration whose fromTTCProtocolVersion is <= the
	negotiated version.

Parameters:
  - dbType: Oracle database type id (DtyType) describing the column type.

Returns:
  - *typeDecoder: The selected decoder implementation.
  - error: Non-nil if no compatible decoder is registered.

Errors:
  - Returns a common.OracleError with code common.InternalError when no decoder candidate exists for dbType.
*/
func (f *CodecFactoryImpl) getDecoder(dbType DtyType) (*typeDecoder, error) {
	common.Odl.Debug("New decoder requested", "dbType", dbType, "ttcVersion", f.ttcVersion)

	candidates := f.decoders.getCandidates(dbType)
	bestCandidate := getEntryFromRegistry(f.ttcVersion, candidates)
	if bestCandidate == nil {
		err := common.NewOracleError(oracleErrors.InternalError, nil)
		common.Odl.Error("Decoder candidate missing", "dbType", dbType, "error", err)
		return nil, err
	}

	common.Odl.Debug("Decoder returned", "candidate", bestCandidate)
	return bestCandidate.makeFunc, nil
}

/*
getEntryFromRegistry selects the best registry candidate for a TTC protocol version.

Description:

	Chooses the candidate with the highest fromTTCProtocolVersion that is <= protocolVersion.
	If protocolVersion == -1, selection treats all candidates as compatible.

	Parameters:
	  - protocolVersion: Negotiated TTC protocol version (or -1 to bypass version checks).
	  - candidates: Candidate registry entries for a given key.

	Returns:
	  - *codecRegistryEntry[F]: The chosen candidate, or nil if none are compatible.
*/
func getEntryFromRegistry[F any](protocolVersion int8, candidates []codecRegistryEntry[F]) *codecRegistryEntry[F] {
	var bestCandidate *codecRegistryEntry[F]

	for i := range candidates {
		candidate := &candidates[i]
		if protocolVersion == -1 || candidate.fromTTCProtocolVersion <= protocolVersion {
			if bestCandidate == nil || candidate.fromTTCProtocolVersion > bestCandidate.fromTTCProtocolVersion {
				bestCandidate = candidate
			}
		}
	}

	return bestCandidate
}

/*
getBindOac returns the bind OAC for a bind value.

Description:

	Selects the best registered OAC constructor candidate for the bind value type based on the factory's
	negotiated TTC protocol version, then constructs the bind OAC immediately using the supplied maxLength.
	If the bind value is a driver.Out, the lookup is performed using the concrete reflect.Type of Out.Dest.

Parameters:
  - bindValue: The bind value for which a bind OAC is requested.
  - maxLength: Maximum length to apply when constructing the bind OAC.

Returns:
  - common.Marshallable: The selected bind OAC instance.
  - error: Non-nil if no compatible bind OAC constructor is registered.

Errors:
  - Returns a common.OracleError with code common.InternalError when no OAC candidate exists for the bind type.
*/
func (f *CodecFactoryImpl) getBindOac(normalized normalizedBindValue, maxLength driverCommon.UB4) (driverCommon.Marshallable, error) {
	common.Odl.Debug("New bind OAC requested", "goType", normalized.goType, "maxLength", maxLength)

	candidates := f.bindOacs.getCandidates(normalized.goType)
	bestCandidate := getEntryFromRegistry(f.ttcVersion, candidates)
	if bestCandidate != nil {
		// bindOacTypeObj is the instance of bindOacType, that contains
		// the function that can build the OAC, and type specific data
		// to build the oac for out parameters
		bindOacTypeObj := bestCandidate.makeFunc
		oac := bindOacTypeObj.bindOacFunc(maxLength)
		// In case of out parameters, we need to override the oac maxlength
		// scale as per the type we are sending. This must be the max possible
		// values to enable the server to return the scalar type.
		if normalized.isOut {
			oac.(*tTIoac).maxLength = bindOacTypeObj.maxLength
			oac.(*tTIoac).scale = bindOacTypeObj.scale
		}
		common.Odl.Debug("Bind OAC returned", "candidate", bestCandidate)
		return oac, nil
	}

	err := common.NewOracleError(oracleErrors.InternalError, nil)
	common.Odl.Error("Bind OAC candidate missing", "goType", normalized.goType, "error", err)
	return nil, err
}

/*
getDefineOac returns the define/fetch OAC for an Oracle database type id.

Description:

	Selects the best registered define OAC constructor candidate for dbType based on the factory's negotiated TTC
	protocol version, then constructs the define OAC immediately using the supplied column context and connection
	properties. If no compatible candidate is registered, a scalar define OAC is used as fallback.

Parameters:
  - dbType: Oracle database type id (DtyType) describing the column type.
  - columnContext: Column metadata used to build the define OAC.
  - connectionProperties: Connection properties used to derive define settings such as LOB prefetch size.

Returns:
  - common.Marshallable: The selected define OAC instance.
*/
func (f *CodecFactoryImpl) getDefineOac(
	dbType DtyType,
	columnContext columnContext,
	connectionProperties driverCommon.DriverProperties,
) driverCommon.Marshallable {
	common.Odl.Debug("New define OAC requested", "dbType", dbType, "ttcVersion", f.ttcVersion)

	candidates := f.defineOacs.getCandidates(dbType)
	bestCandidate := getEntryFromRegistry(f.ttcVersion, candidates)
	var lobPrefetchSize driverCommon.UB4
	if connectionProperties != nil {
		lobPrefetchSize = driverCommon.UB4(connectionProperties.GetDefaultLobPrefetchSize())
	}

	if bestCandidate != nil {
		oac := bestCandidate.makeFunc(columnContext, lobPrefetchSize)
		common.Odl.Debug("Define OAC returned", "candidate", bestCandidate)
		return oac
	}

	return newTTIOacScalarDefine(columnContext)
}
