package ttc

import "github.com/oracle/pure-go-driver/driver/common"

// buildVectorQuasiLocator builds a value-based LOB locator for VECTOR bind payloads.
func buildVectorQuasiLocator(payloadLength int) common.B1Array {
	locatorSize := kolblTempLocatorMaxLength
	locator := make(common.B1Array, locatorSize)

	length := locatorSize - kolbLocatorLengthHeaderBytes
	locator[kolbLocatorLengthOffset] = byte(length >> 8)
	locator[kolbLocatorLengthOffset+1] = byte(length)

	locator[kolbVersionOffset] = 0
	locator[kolbVersionOffset+1] = quasiLocatorVersion

	locator[koll1FlagOffset] = (kolblBlobFlag | kolblValueBasedLocatorFlag) | kolblAbstractLocatorFlag
	locator[koll2FlagOffset] = kolblInitializedFlag
	locator[koll3FlagOffset] = 0
	locator[koll4FlagOffset] = 0

	// Byte width (BYTL) for binary payloads.
	locator[8] = 0
	locator[9] = 1

	lobLen := common.UB8(payloadLength)
	locator[10] = byte(lobLen >> 56)
	locator[11] = byte(lobLen >> 48)
	locator[12] = byte(lobLen >> 40)
	locator[13] = byte(lobLen >> 32)
	locator[14] = byte(lobLen >> 24)
	locator[15] = byte(lobLen >> 16)
	locator[16] = byte(lobLen >> 8)
	locator[17] = byte(lobLen)

	// Character set id remains zero for binary locators.
	locator[20] = 0
	locator[21] = 0

	return locator
}
