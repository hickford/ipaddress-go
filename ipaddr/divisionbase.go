//
// Copyright 2020-2022 Sean C Foley
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ipaddr

import (
	"math/big"
	"sync/atomic"
	"unsafe"
)

// divisionValuesBase provides an interface for divisions of any bit-size.
// It is shared by standard and large divisions.
type divisionValuesBase interface {
	getBitCount() BitCount

	getByteCount() int

	// getDivisionPrefixLength provides the prefix length
	// if is aligned is true and the prefix is non-nil, any divisions that follow in the same grouping have a zero-length prefix
	getDivisionPrefixLength() PrefixLen

	// getValue gets the lower value for a large division
	getValue() *BigDivInt

	// getValue gets the upper value for a large division
	getUpperValue() *BigDivInt

	includesZero() bool

	includesMax() bool

	isMultiple() bool

	getCount() *big.Int

	// convert lower and upper values to byte arrays
	calcBytesInternal() (bytes, upperBytes []byte)

	// getCache returns a cacheBitCountx for those divisions which cacheBitCountx their values, or nil otherwise
	getCache() *divCache

	getAddrType() addrType
}

type bytesCache struct {
	lowerBytes, upperBytes []byte
}

type divCache struct {
	cachedString, cachedWildcardString, cached0xHexString, cachedHexString, cachedNormalizedString *string

	cachedBytes *bytesCache

	isSinglePrefBlock *bool

	minPrefLenForBlock PrefixLen
}

// addressDivisionBase is a division of any bit-size.
// It is shared by standard and large divisions types.
// Large divisions must not use the methods of divisionValues and use only the methods in divisionValuesBase.
type addressDivisionBase struct {
	// I've looked into making this divisionValuesBase.
	// If you do that, then to get access to the methods in divisionValues, you can either do type assertions like divisionValuesBase.(divisionValiues),
	// or you can add a method getDivisionValues to divisionValuesBase.
	// But in the end, either way you are assuming you knowe that divisionValuesBase is a divisionValues.  So no point.
	// Instead, each division type like IPAddressSegment and LargeDivision will know which value methods apply to that type.
	divisionValues
}

func (div *addressDivisionBase) getDivisionPrefixLength() PrefixLen {
	vals := div.divisionValues
	if vals == nil {
		return nil
	}
	return vals.getDivisionPrefixLength()
}

// GetBitCount returns the number of bits in each value comprising this address item
func (div *addressDivisionBase) GetBitCount() BitCount {
	vals := div.divisionValues
	if vals == nil {
		return 0
	}
	return vals.getBitCount()
}

// GetByteCount returns the number of bytes required for each value comprising this address item,
// rounding up if the bit count is not a multiple of 8.
func (div *addressDivisionBase) GetByteCount() int {
	vals := div.divisionValues
	if vals == nil {
		return 0
	}
	return vals.getByteCount()
}

func (div *addressDivisionBase) GetValue() *BigDivInt {
	vals := div.divisionValues
	if vals == nil {
		return bigZero()
	}
	return vals.getValue()
}

func (div *addressDivisionBase) GetUpperValue() *BigDivInt {
	vals := div.divisionValues
	if vals == nil {
		return bigZero()
	}
	return vals.getUpperValue()
}

func (div *addressDivisionBase) Bytes() []byte {
	if div.divisionValues == nil {
		return emptyBytes
	}
	cached := div.getBytes()
	return cloneBytes(cached)
}

func (div *addressDivisionBase) UpperBytes() []byte {
	if div.divisionValues == nil {
		return emptyBytes
	}
	cached := div.getUpperBytes()
	return cloneBytes(cached)
}

func (div *addressDivisionBase) CopyBytes(bytes []byte) []byte {
	if div.divisionValues == nil {
		if bytes != nil {
			return bytes
		}
		return emptyBytes
	}
	cached := div.getBytes()
	return getBytesCopy(bytes, cached)
}

func (div *addressDivisionBase) CopyUpperBytes(bytes []byte) []byte {
	if div.divisionValues == nil {
		if bytes != nil {
			return bytes
		}
		return emptyBytes
	}
	cached := div.getUpperBytes()
	return getBytesCopy(bytes, cached)
}

func (div *addressDivisionBase) getBytes() (bytes []byte) {
	bytes, _ = div.getBytesInternal()
	return
}

func (div *addressDivisionBase) getUpperBytes() (bytes []byte) {
	_, bytes = div.getBytesInternal()
	return
}

func (div *addressDivisionBase) getBytesInternal() (bytes, upperBytes []byte) {
	cache := div.getCache()
	if cache == nil {
		return div.calcBytesInternal()
	}
	cached := cache.cachedBytes
	if cached == nil {
		bytes, upperBytes = div.calcBytesInternal()
		cached = &bytesCache{
			lowerBytes: bytes,
			upperBytes: upperBytes,
		}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&cache.cachedBytes))
		atomic.StorePointer(dataLoc, unsafe.Pointer(cached))
	}
	return cached.lowerBytes, cached.upperBytes
}

func (div *addressDivisionBase) getCount() *big.Int {
	if !div.isMultiple() {
		return bigOne()
	}
	return div.divisionValues.getCount()
}

func (div *addressDivisionBase) isMultiple() bool {
	vals := div.divisionValues
	if vals == nil {
		return false
	}
	return vals.isMultiple()
}

// The count of the number of distinct values within the prefix part of the address item, the bits that appear within the prefix length.
func (div *addressDivisionBase) GetPrefixCountLen(prefixLength BitCount) *big.Int {
	if prefixLength < 0 {
		return bigOne()
	}
	bitCount := div.GetBitCount()
	if prefixLength >= bitCount {
		return div.getCount()
	}
	ushiftAdjustment := uint(bitCount - prefixLength)
	lower := div.GetValue()
	upper := div.GetUpperValue()
	upper.Rsh(upper, ushiftAdjustment)
	lower.Rsh(lower, ushiftAdjustment)
	upper.Sub(upper, lower).Add(upper, bigOneConst())
	return upper
}

// IsZero returns whether this division matches exactly the value of zero
func (div *addressDivisionBase) IsZero() bool {
	return !div.isMultiple() && div.IncludesZero()
}

// IncludesZero returns whether this item includes the value of zero within its range
func (div *addressDivisionBase) IncludesZero() bool {
	vals := div.divisionValues
	if vals == nil {
		return true
	}
	return vals.includesZero()
}

// IsMax returns whether this item matches the maximum possible value for the address type or version
func (div *addressDivisionBase) IsMax() bool {
	return !div.isMultiple() && div.includesMax()
}

// IncludesMax returns whether this item includes the maximum possible value for the address type or version within its range
func (div *addressDivisionBase) IncludesMax() bool {
	vals := div.divisionValues
	if vals == nil {
		return false
	}
	return vals.includesMax()
}

// IsFullRange returns whether the division range includes all possible values for its bit length.
//
//  This is true if and only if both IncludesZero and IncludesMax return true.
func (div *addressDivisionBase) IsFullRange() bool {
	return div.includesZero() && div.includesMax()
}

func (div *addressDivisionBase) getAddrType() addrType {
	vals := div.divisionValues
	if vals == nil {
		return zeroType
	}
	return vals.getAddrType()
}

func (div *addressDivisionBase) matchesStructure(other DivisionType) (res bool, addrType addrType) {
	addrType = div.getAddrType()
	if addrType != other.getAddrType() || (addrType.isNil() && (div.GetBitCount() != other.GetBitCount())) {
		return
	}
	res = true
	return
}

// returns the default radix for textual representations of addresses (10 for IPv4, 16 for IPv6, MAC and other)
func (div *addressDivisionBase) getDefaultTextualRadix() int {
	addrType := div.getAddrType()
	if addrType.isIPv4() {
		return IPv4DefaultTextualRadix
	}
	return 16
}

func bigDivsSame(onePref, twoPref PrefixLen, oneVal, twoVal, oneUpperVal, twoUpperVal *BigDivInt) bool {
	return onePref.Equal(twoPref) &&
		oneVal.CmpAbs(twoVal) == 0 && oneUpperVal.CmpAbs(twoUpperVal) == 0
}

func bigDivValsSame(oneVal, twoVal, oneUpperVal, twoUpperVal *BigDivInt) bool {
	return oneVal.CmpAbs(twoVal) == 0 && oneUpperVal.CmpAbs(twoUpperVal) == 0
}

func bigDivValSame(oneVal, twoVal *big.Int) bool {
	return oneVal.CmpAbs(twoVal) == 0
}
