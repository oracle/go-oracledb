package oracle

import driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"

// VectorFloat64 is an alias for dense VECTOR(FLOAT64) bind values.
// VECTOR query results are decoded as []float64.
type VectorFloat64 = driverCommon.VectorFloat64

// VectorFloat32 is an alias for dense VECTOR(FLOAT32) bind values.
// VECTOR query results are decoded as []float32.
type VectorFloat32 = driverCommon.VectorFloat32

// VectorInt8 is an alias for dense VECTOR(INT8) bind values.
// VECTOR query results are decoded as []int8.
type VectorInt8 = driverCommon.VectorInt8

// VectorBinary is an alias for packed BINARY VECTOR bind values.
// Each byte stores 8 dimensions in MSB-first order.
// VECTOR query results are decoded as []byte using the same packed layout.
type VectorBinary = driverCommon.VectorBinary
