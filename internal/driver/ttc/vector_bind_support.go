package ttc

import (
	"context"
	"database/sql/driver"
	"reflect"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

var (
	vectorFloat64Type = reflect.TypeOf(driverCommon.VectorFloat64(nil))
	vectorFloat32Type = reflect.TypeOf(driverCommon.VectorFloat32(nil))
	vectorInt8Type    = reflect.TypeOf(driverCommon.VectorInt8(nil))
	vectorBinaryType  = reflect.TypeOf(driverCommon.VectorBinary(nil))
)

func isVectorBindType(goType reflect.Type) bool {
	switch goType {
	case vectorFloat64Type, vectorFloat32Type, vectorInt8Type, vectorBinaryType:
		return true
	default:
		return false
	}
}

func checkVectorNamedValue(nv *driver.NamedValue) error {
	if nv == nil {
		return driver.ErrSkip
	}
	if isVectorBindType(reflect.TypeOf(nv.Value)) {
		return nil
	}
	return driver.ErrSkip
}

// newVectorBindEncoder associates VECTOR payload encoding with its native
// quasi-locator bind representation.
func newVectorBindEncoder(encode encoderFunc) *typeEncoder {
	return newTypeEncoder(encode, wrapVectorColumn)
}

// wrapVectorColumn writes the native VECTOR payload with its BLOB quasi-locator.
func wrapVectorColumn(ctx context.Context, engine driverCommon.Marshaller, payload driverCommon.B1Array) error {
	if payload == nil {
		return engine.MarshalUB4(ctx, 0)
	}

	locator := buildVectorQuasiLocator(len(payload))
	locLen := len(locator)
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(locLen)); err != nil {
		return err
	}
	if err := engine.MarshalCLR(ctx, locator, 0, locLen); err != nil {
		return err
	}
	if err := engine.MarshalCLR(ctx, payload, 0, len(payload)); err != nil {
		return err
	}
	return nil
}
