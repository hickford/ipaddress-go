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

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
)

type MACSegInt = uint8
type MACSegmentValueProvider func(segmentIndex int) MACSegInt

func WrappedMACSegmentValueProvider(f MACSegmentValueProvider) SegmentValueProvider {
	if f == nil {
		return nil
	}
	return func(segmentIndex int) SegInt {
		return SegInt(f(segmentIndex))
	}
}

func WrappedSegmentValueProviderForMAC(f SegmentValueProvider) MACSegmentValueProvider {
	if f == nil {
		return nil
	}
	return func(segmentIndex int) MACSegInt {
		return MACSegInt(f(segmentIndex))
	}
}

const useMACSegmentCache = true

type macSegmentValues struct {
	value      MACSegInt
	upperValue MACSegInt
	cache      divCache
}

func (seg *macSegmentValues) getAddrType() addrType {
	return macType
}

func (seg *macSegmentValues) includesZero() bool {
	return seg.value == 0
}

func (seg *macSegmentValues) includesMax() bool {
	return seg.upperValue == 0xff
}

func (seg *macSegmentValues) isMultiple() bool {
	return seg.value != seg.upperValue
}

func (seg *macSegmentValues) getCount() *big.Int {
	return big.NewInt(int64(seg.upperValue-seg.value) + 1)
}

func (seg *macSegmentValues) getBitCount() BitCount {
	return MACBitsPerSegment
}

func (seg *macSegmentValues) getByteCount() int {
	return MACBytesPerSegment
}

func (seg *macSegmentValues) getValue() *BigDivInt {
	return big.NewInt(int64(seg.value))
}

func (seg *macSegmentValues) getUpperValue() *BigDivInt {
	return big.NewInt(int64(seg.upperValue))
}

func (seg *macSegmentValues) getDivisionValue() DivInt {
	return DivInt(seg.value)
}

func (seg *macSegmentValues) getUpperDivisionValue() DivInt {
	return DivInt(seg.upperValue)
}

func (seg *macSegmentValues) getDivisionPrefixLength() PrefixLen {
	return nil
}

func (seg *macSegmentValues) getSegmentValue() SegInt {
	return SegInt(seg.value)
}

func (seg *macSegmentValues) getUpperSegmentValue() SegInt {
	return SegInt(seg.upperValue)
}

func (seg *macSegmentValues) calcBytesInternal() (bytes, upperBytes []byte) {
	bytes = []byte{byte(seg.value)}
	if seg.isMultiple() {
		upperBytes = []byte{byte(seg.upperValue)}
	} else {
		upperBytes = bytes
	}
	return
}

func (seg *macSegmentValues) deriveNew(val, upperVal DivInt, _ PrefixLen) divisionValues {
	return newMACSegmentValues(MACSegInt(val), MACSegInt(upperVal))
}

func (seg *macSegmentValues) deriveNewSeg(val SegInt, _ PrefixLen) divisionValues {
	return newMACSegmentVal(MACSegInt(val))
}

func (seg *macSegmentValues) deriveNewMultiSeg(val, upperVal SegInt, _ PrefixLen) divisionValues {
	return newMACSegmentValues(MACSegInt(val), MACSegInt(upperVal))
}

func (seg *macSegmentValues) getCache() *divCache {
	return &seg.cache
}

var _ divisionValues = &macSegmentValues{}

var zeroMACSeg = NewMACSegment(0)
var allRangeMACSeg = NewMACRangeSegment(0, MACMaxValuePerSegment)

type MACAddressSegment struct {
	addressSegmentInternal
}

// GetMACSegmentValue returns the lower value.  Same as GetSegmentValue but returned as a MACSegInt.
func (seg *MACAddressSegment) GetMACSegmentValue() MACSegInt {
	return MACSegInt(seg.GetSegmentValue())
}

// GetMACUpperSegmentValue returns the lower value.  Same as GetUpperSegmentValue but returned as a MACSegInt.
func (seg *MACAddressSegment) GetMACUpperSegmentValue() MACSegInt {
	return MACSegInt(seg.GetUpperSegmentValue())
}

func (seg *MACAddressSegment) init() *MACAddressSegment {
	if seg.divisionValues == nil {
		return zeroMACSeg
	}
	return seg
}

func (seg *MACAddressSegment) Contains(other AddressSegmentType) bool {
	if seg == nil {
		return other == nil || other.ToSegmentBase() == nil
	}
	return seg.init().contains(other)
}

func (seg *MACAddressSegment) Equal(other AddressSegmentType) bool {
	if seg == nil {
		return other == nil || other.ToDiv() == nil
		//return seg.getAddrType() == macType && other.(StandardDivisionType).ToDiv() == nil
	}
	return seg.init().equal(other)
}

// PrefixContains returns whether the range of the given prefix bits contains the same bits of the given segment.
func (seg *MACAddressSegment) PrefixContains(other AddressSegmentType, prefixLength BitCount) bool {
	return seg.init().addressSegmentInternal.PrefixContains(other, prefixLength)
}

// PrefixEqual returns whether the given prefix bits match the same bits of the given segment.
func (seg *MACAddressSegment) PrefixEqual(other AddressSegmentType, prefixLength BitCount) bool {
	return seg.init().addressSegmentInternal.PrefixEqual(other, prefixLength)
}

func (seg *MACAddressSegment) Compare(item AddressItem) int {
	return CountComparator.Compare(seg, item)
}

// GetBitCount returns the number of bits in each value comprising this address item, which is 8.
func (seg *MACAddressSegment) GetBitCount() BitCount {
	return IPv4BitsPerSegment
}

// GetByteCount returns the number of bytes required for each value comprising this address item, which is 1.
func (seg *MACAddressSegment) GetByteCount() int {
	return IPv4BytesPerSegment
}

func (seg *MACAddressSegment) GetMaxValue() MACSegInt {
	return 0xff
}

func (seg *MACAddressSegment) GetLower() *MACAddressSegment {
	return seg.init().getLower().ToMAC()
}

func (seg *MACAddressSegment) GetUpper() *MACAddressSegment {
	return seg.init().getUpper().ToMAC()
}

// IsMultiple returns whether this segment represents multiple values
func (seg *MACAddressSegment) IsMultiple() bool {
	return seg != nil && seg.isMultiple()
}

// GetCount returns the count of possible distinct values for this item.
// If not representing multiple values, the count is 1.
//
// For instance, a segment with the value range of 3-7 has count 5.
//
// Use IsMultiple if you simply want to know if the count is greater than 1.
func (seg *MACAddressSegment) GetCount() *big.Int {
	if seg == nil {
		return bigZero()
	}
	return seg.getCount()
}

func (seg *MACAddressSegment) Bytes() []byte {
	return seg.init().addressSegmentInternal.Bytes()
}

func (seg *MACAddressSegment) UpperBytes() []byte {
	return seg.init().addressSegmentInternal.UpperBytes()
}

func (seg *MACAddressSegment) CopyBytes(bytes []byte) []byte {
	return seg.init().addressSegmentInternal.CopyBytes(bytes)
}

func (seg *MACAddressSegment) CopyUpperBytes(bytes []byte) []byte {
	return seg.init().addressSegmentInternal.CopyUpperBytes(bytes)
}

func (seg *MACAddressSegment) GetPrefixCountLen(segmentPrefixLength BitCount) *big.Int {
	return seg.init().addressSegmentInternal.GetPrefixCountLen(segmentPrefixLength)
}

func (seg *MACAddressSegment) GetPrefixValueCountLen(segmentPrefixLength BitCount) SegIntCount {
	return seg.init().addressSegmentInternal.GetPrefixValueCountLen(segmentPrefixLength)
}

// Returns true if the bit in the lower value of this segment at the given index is 1, where index 0 is the most significant bit.
func (seg *MACAddressSegment) IsOneBit(segmentBitIndex BitCount) bool {
	return seg.init().addressSegmentInternal.IsOneBit(segmentBitIndex)
}

func (seg *MACAddressSegment) setString(
	addressStr string,
	isStandardString bool,
	lowerStringStartIndex,
	lowerStringEndIndex int,
	originalLowerValue SegInt) {
	if cache := seg.getCache(); cache != nil {
		if cache.cachedString == nil && isStandardString && originalLowerValue == seg.getSegmentValue() {
			str := addressStr[lowerStringStartIndex:lowerStringEndIndex]
			cacheStrPtr(&cache.cachedString, &str)
		}
	}
}

func (seg *MACAddressSegment) setRangeString(
	addressStr string,
	isStandardRangeString bool,
	lowerStringStartIndex,
	upperStringEndIndex int,
	rangeLower,
	rangeUpper SegInt) {
	if cache := seg.getCache(); cache != nil {
		if cache.cachedString == nil {
			if seg.IsFullRange() {
				cacheStrPtr(&cache.cachedString, &segmentWildcardStr)
			} else if isStandardRangeString && rangeLower == seg.getSegmentValue() && rangeUpper == seg.getUpperSegmentValue() {
				str := addressStr[lowerStringStartIndex:upperStringEndIndex]
				cacheStrPtr(&cache.cachedString, &str)
			}
		}
	}
}

func (seg *MACAddressSegment) Iterator() MACSegmentIterator {
	if seg == nil {
		return macSegmentIterator{nilSegIterator()}
	}
	return macSegmentIterator{seg.init().iterator()}
}

func (seg *MACAddressSegment) PrefixBlockIterator(segmentPrefixLen BitCount) MACSegmentIterator {
	return macSegmentIterator{seg.init().prefixedBlockIterator(segmentPrefixLen)}
}

func (seg *MACAddressSegment) PrefixIterator(segmentPrefixLen BitCount) MACSegmentIterator {
	return macSegmentIterator{seg.init().prefixedIterator(segmentPrefixLen)}
}

func (seg *MACAddressSegment) ReverseBits(_ bool) (res *MACAddressSegment, err addrerr.IncompatibleAddressError) {
	if seg.divisionValues == nil {
		res = seg
		return
	}
	if seg.isMultiple() {
		if isReversible := seg.isReversibleRange(false); isReversible {
			res = seg
			return
		}
		err = &incompatibleAddressError{addressError{key: "ipaddress.error.reverseRange"}}
		return
	}
	oldVal := MACSegInt(seg.GetSegmentValue())
	val := MACSegInt(reverseUint8(uint8(oldVal)))
	if oldVal == val {
		res = seg
	} else {
		res = NewMACSegment(val)
	}
	return
}

func (seg *MACAddressSegment) ReverseBytes() (*MACAddressSegment, addrerr.IncompatibleAddressError) {
	return seg, nil
}

// Join joins with another MAC segment to produce a IPv6 segment.
func (seg *MACAddressSegment) Join(macSegment1 *MACAddressSegment, prefixLength PrefixLen) (*IPv6AddressSegment, addrerr.IncompatibleAddressError) {
	return seg.joinSegs(macSegment1, false, prefixLength)
}

// JoinAndFlip2ndBit joins with another MAC segment to produce a IPv6 segment with the second bit flipped from 1 to 0.
func (seg *MACAddressSegment) JoinAndFlip2ndBit(macSegment1 *MACAddressSegment, prefixLength PrefixLen) (*IPv6AddressSegment, addrerr.IncompatibleAddressError) {
	return seg.joinSegs(macSegment1, true, prefixLength)
}

func (seg *MACAddressSegment) joinSegs(macSegment1 *MACAddressSegment, flip bool, prefixLength PrefixLen) (*IPv6AddressSegment, addrerr.IncompatibleAddressError) {
	if seg.isMultiple() {
		// if the high segment has a range, the low segment must match the full range,
		// otherwise it is not possible to create an equivalent range when joining
		if !macSegment1.IsFullRange() {
			return nil, &incompatibleAddressError{addressError{key: "ipaddress.error.invalidMACIPv6Range"}}
		}
	}
	lower0 := seg.GetSegmentValue()
	upper0 := seg.GetUpperSegmentValue()
	if flip {
		mask2ndBit := SegInt(0x2)
		if !seg.MatchesWithMask(mask2ndBit&lower0, mask2ndBit) { // ensures that bit remains constant
			return nil, &incompatibleAddressError{addressError{key: "ipaddress.mac.error.not.eui.convertible"}}
		}
		lower0 ^= mask2ndBit //flip the universal/local bit
		upper0 ^= mask2ndBit
	}
	return NewIPv6RangePrefixedSegment(
		IPv6SegInt((lower0<<8)|macSegment1.getSegmentValue()),
		IPv6SegInt((upper0<<8)|macSegment1.getUpperSegmentValue()),
		prefixLength), nil
}

func (seg *MACAddressSegment) ToDiv() *AddressDivision {
	return seg.ToSegmentBase().ToDiv()
}

func (seg *MACAddressSegment) ToSegmentBase() *AddressSegment {
	if seg == nil {
		return nil
	}
	return (*AddressSegment)(seg.init())
}

func (seg *MACAddressSegment) GetString() string {
	if seg == nil {
		return nilString()
	}
	return seg.init().getString()
}

func (seg *MACAddressSegment) GetWildcardString() string {
	if seg == nil {
		return nilString()
	}
	return seg.init().getWildcardString()
}

func (seg *MACAddressSegment) String() string {
	if seg == nil {
		return nilString()
	}
	return seg.init().toString()
}

func NewMACSegment(val MACSegInt) *MACAddressSegment {
	return newMACSegment(newMACSegmentVal(val))
}

func NewMACRangeSegment(val, upperVal MACSegInt) *MACAddressSegment {
	return newMACSegment(newMACSegmentValues(val, upperVal))
}

func newMACSegment(vals *macSegmentValues) *MACAddressSegment {
	return &MACAddressSegment{
		addressSegmentInternal{
			addressDivisionInternal{
				addressDivisionBase{vals},
			},
		},
	}
}

var (
	allRangeValsMAC = &macSegmentValues{
		upperValue: MACMaxValuePerSegment,
	}
	segmentCacheMAC = makeSegmentCacheMAC()
)

func makeSegmentCacheMAC() (segmentCacheMAC []macSegmentValues) {
	if useMACSegmentCache {
		segmentCacheMAC = make([]macSegmentValues, MACMaxValuePerSegment+1)
		for i := range segmentCacheMAC {
			vals := &segmentCacheMAC[i]
			segi := MACSegInt(i)
			vals.value = segi
			vals.upperValue = segi
		}
	}
	return
}

func newMACSegmentVal(value MACSegInt) *macSegmentValues {
	if useMACSegmentCache {
		result := &segmentCacheMAC[value]
		//checkValuesMAC(value, value, result)
		return result
	}
	return &macSegmentValues{value: value, upperValue: value}
}

func newMACSegmentValues(value, upperValue MACSegInt) *macSegmentValues {
	if value == upperValue {
		return newMACSegmentVal(value)
	} else if value > upperValue {
		value, upperValue = upperValue, value
	}
	if useMACSegmentCache && value == 0 && upperValue == MACMaxValuePerSegment {
		return allRangeValsMAC
	}
	return &macSegmentValues{value: value, upperValue: upperValue}
}
