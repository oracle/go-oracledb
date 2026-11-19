package common

// VectorFloat64 represents dense VECTOR(FLOAT64) bind values.
type VectorFloat64 []float64

// VectorFloat32 represents dense VECTOR(FLOAT32) bind values.
type VectorFloat32 []float32

// VectorInt8 represents dense VECTOR(INT8) bind values.
type VectorInt8 []int8

// VectorBinary represents a packed BINARY VECTOR payload.
// Each byte stores 8 dimensions in MSB-first order.
type VectorBinary []byte
