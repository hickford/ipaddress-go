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
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
)

type ipAddressSegmentInternal struct {
	addressSegmentInternal
}

func (seg *ipAddressSegmentInternal) isPrefixed() bool {
	return seg.GetSegmentPrefixLen() != nil
}

// IsPrefixBlock returns whether the division has a prefix length and the division range includes the block of values for that prefix length.
func (seg *ipAddressSegmentInternal) IsPrefixBlock() bool {
	return seg.isPrefixBlock()
}

// IsSinglePrefixBlock returns whether the range matches the block of values for a single prefix identified by the prefix length of this address.
// This is similar to IsPrefixBlock() except that it returns false when the subnet has multiple prefixes.
//
// What distinguishes this method from ContainsSinglePrefixBlock is that this method returns
// false if the series does not have a prefix length assigned to it,
// or a prefix length that differs from the prefix length for which ContainsSinglePrefixBlock returns true.
//
// It is similar to IsPrefixBlock but returns false when there are multiple prefixes.
func (seg *ipAddressSegmentInternal) IsSinglePrefixBlock() bool {
	cache := seg.getCache()
	if cache == nil {
		if prefLen := seg.GetSegmentPrefixLen(); prefLen != nil {
			return seg.isSinglePrefixBlock(seg.getDivisionValue(), seg.getUpperDivisionValue(), prefLen.bitCount())
		}
		return false
	}
	res := cache.isSinglePrefBlock
	if res == nil {
		var result bool
		if prefLen := seg.GetSegmentPrefixLen(); prefLen != nil {
			result = seg.isSinglePrefixBlock(seg.getDivisionValue(), seg.getUpperDivisionValue(), prefLen.bitCount())
		}
		if result {
			res = &trueVal
		} else {
			res = &falseVal
		}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&cache.isSinglePrefBlock))
		atomic.StorePointer(dataLoc, unsafe.Pointer(res))
	}
	return *res
}

func (seg *ipAddressSegmentInternal) withoutPrefixLen() *IPAddressSegment {
	if seg.isPrefixed() {
		vals := seg.deriveNewMultiSeg(seg.GetSegmentValue(), seg.GetUpperSegmentValue(), nil)
		return createAddressDivision(vals).ToIP()
	}
	return seg.toIPAddressSegment()
}

func (seg *ipAddressSegmentInternal) GetPrefixValueCount() SegIntCount {
	prefixLength := seg.GetSegmentPrefixLen()
	if prefixLength == nil {
		return seg.GetValueCount()
	}
	return getPrefixValueCount(seg.toAddressSegment(), prefixLength.bitCount())
}

// GetSegmentPrefixLen returns the network prefix for the segment.
//
// The network prefix is 16 for an address like 1.2.0.0/16.
//
// When it comes to each address division or segment, the prefix for the division is the
// prefix obtained when applying the address or section prefix.
//
// For instance, consider the address 1.2.0.0/20.
// The first segment has no prefix because the address prefix 20 extends beyond the 8 bits in the first segment, it does not even apply to the segment.
// The second segment has no prefix because the address prefix extends beyond bits 9 to 16 which lie in the second segment, it does not apply to that segment either.
// The third segment has the prefix 4 because the address prefix 20 corresponds to the first 4 bits in the 3rd segment,
// which means that the first 4 bits are part of the network section of the address or segment.
// The last segment has the prefix 0 because not a single bit is in the network section of the address or segment
//
// The prefix applied across the address is nil ... nil ... (1 to segment bit length) ... 0 ... 0
//
// If the segment has no prefix then nil is returned.
func (seg *ipAddressSegmentInternal) GetSegmentPrefixLen() PrefixLen {
	return seg.getDivisionPrefixLength()
}

func (seg *ipAddressSegmentInternal) MatchesWithPrefixMask(value SegInt, networkBits BitCount) bool {
	mask := seg.GetSegmentNetworkMask(networkBits)
	matchingValue := value & mask
	return matchingValue == (seg.GetSegmentValue()&mask) && matchingValue == (seg.GetUpperSegmentValue()&mask)
}

func (seg *ipAddressSegmentInternal) checkForPrefixMask() (networkMaskLen, hostMaskLen PrefixLen) {
	val := seg.GetSegmentValue()
	if val == 0 {
		networkMaskLen, hostMaskLen = cacheBitCount(0), cacheBitCount(seg.GetBitCount())
	} else {
		maxVal := seg.GetMaxValue()
		if val == maxVal {
			networkMaskLen, hostMaskLen = cacheBitCount(seg.GetBitCount()), cacheBitCount(0)
		} else {
			var shifted SegInt
			trailingOnes := seg.GetTrailingBitCount(true)
			if trailingOnes == 0 {
				// can only be 11110000 and not 00000000
				trailingZeros := seg.GetTrailingBitCount(false)
				shifted = (^val & maxVal) >> uint(trailingZeros)
				if shifted == 0 {
					networkMaskLen = cacheBitCount(seg.GetBitCount() - trailingZeros)
				}
			} else {
				// can only be 00001111 and not 11111111
				shifted = val >> uint(trailingOnes)
				if shifted == 0 {
					hostMaskLen = cacheBitCount(seg.GetBitCount() - trailingOnes)
				}
			}
		}
	}
	return
}

// GetBlockMaskPrefixLen returns the prefix length if this address section is equivalent to the mask for a CIDR prefix block.
// Otherwise, it returns nil.
// A CIDR network mask is an address with all 1s in the network section and then all 0s in the host section.
// A CIDR host mask is an address with all 0s in the network section and then all 1s in the host section.
// The prefix length is the length of the network section.
//
// Also, keep in mind that the prefix length returned by this method is not equivalent to the prefix length of this object,
// indicating the network and host section of this address.
// The prefix length returned here indicates the whether the value of this address can be used as a mask for the network and host
// section of any other address.  Therefore the two values can be different values, or one can be nil while the other is not.
//
// This method applies only to the lower value of the range if this section represents multiple values.
func (seg *ipAddressSegmentInternal) GetBlockMaskPrefixLen(network bool) PrefixLen {
	hostLength := seg.GetTrailingBitCount(!network)
	var shifted SegInt
	val := seg.GetSegmentValue()
	if network {
		maxVal := seg.GetMaxValue()
		shifted = (^val & maxVal) >> uint(hostLength)
	} else {
		shifted = val >> uint(hostLength)
	}
	if shifted == 0 {
		return cacheBitCount(seg.GetBitCount() - hostLength)
	}
	return nil
}

func (seg *ipAddressSegmentInternal) getUpperStringMasked(radix int, uppercase bool, appendable *strings.Builder) {
	if seg.isPrefixed() {
		upperValue := seg.GetUpperSegmentValue()
		mask := seg.GetSegmentNetworkMask(seg.GetSegmentPrefixLen().bitCount())
		upperValue &= mask
		toUnsignedStringCased(DivInt(upperValue), radix, 0, uppercase, appendable)
	} else {
		seg.getUpperString(radix, uppercase, appendable)
	}
}

func (seg *ipAddressSegmentInternal) getStringAsLower() string {
	if seg.divisionValues != nil {
		if cache := seg.getCache(); cache != nil {
			return cacheStr(&cache.cachedString, seg.getDefaultLowerString)
		}
	}
	return seg.getDefaultLowerString()
}

func (seg *ipAddressSegmentInternal) getString() string {
	stringer := func() string {
		if !seg.isMultiple() || seg.IsSinglePrefixBlock() { //covers the case of !isMult, ie single addresses, when there is no prefix or the prefix is the bit count
			return seg.getDefaultLowerString()
		} else if seg.IsFullRange() {
			return seg.getDefaultSegmentWildcardString()
		}
		upperValue := seg.getUpperSegmentValue()
		if seg.IsPrefixBlock() {
			upperValue &= seg.GetSegmentNetworkMask(seg.getDivisionPrefixLength().bitCount())
		}
		return seg.getDefaultRangeStringVals(seg.getDivisionValue(), DivInt(upperValue), seg.getDefaultTextualRadix())
	}
	if seg.divisionValues != nil {
		if cache := seg.getCache(); cache != nil {
			return cacheStr(&cache.cachedString, stringer)
		}
	}
	return stringer()
}

func (seg *ipAddressSegmentInternal) getWildcardString() string {
	stringer := func() string {
		if !seg.isPrefixed() || !seg.isMultiple() {
			return seg.getString()
		} else if seg.IsFullRange() {
			return seg.getDefaultSegmentWildcardString()
		}
		return seg.getDefaultRangeString()
	}
	if seg.divisionValues != nil {
		if cache := seg.getCache(); cache != nil {
			return cacheStr(&cache.cachedWildcardString, stringer)
		}
	}
	return stringer()
}

func (seg *ipAddressSegmentInternal) setStandardString(
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

func (seg *ipAddressSegmentInternal) setWildcardString(
	addressStr string,
	isStandardString bool,
	lowerStringStartIndex,
	lowerStringEndIndex int,
	lowerValue SegInt) {
	if cache := seg.getCache(); cache != nil {
		if cache.cachedWildcardString == nil && isStandardString && lowerValue == seg.getSegmentValue() && lowerValue == seg.getUpperSegmentValue() {
			str := addressStr[lowerStringStartIndex:lowerStringEndIndex]
			cacheStrPtr(&cache.cachedWildcardString, &str)
		}
	}
}

func (seg *ipAddressSegmentInternal) setRangeStandardString(
	addressStr string,
	isStandardString,
	isStandardRangeString bool,
	lowerStringStartIndex,
	lowerStringEndIndex,
	upperStringEndIndex int,
	rangeLower,
	rangeUpper SegInt) {
	if cache := seg.getCache(); cache != nil {
		if cache.cachedString == nil {
			if seg.IsSinglePrefixBlock() {
				if isStandardString && rangeLower == seg.getSegmentValue() {
					str := addressStr[lowerStringStartIndex:lowerStringEndIndex]
					cacheStrPtr(&cache.cachedString, &str)
				}
			} else if seg.IsFullRange() {
				cacheStrPtr(&cache.cachedString, &segmentWildcardStr)
			} else if isStandardRangeString && rangeLower == seg.getSegmentValue() {
				upper := seg.getUpperSegmentValue()
				if seg.isPrefixed() {
					upper &= seg.GetSegmentNetworkMask(seg.getDivisionPrefixLength().bitCount())
				}
				if rangeUpper == upper {
					str := addressStr[lowerStringStartIndex:upperStringEndIndex]
					cacheStrPtr(&cache.cachedString, &str)
				}
			}
		}
	}
}

func (seg *ipAddressSegmentInternal) setRangeWildcardString(
	addressStr string,
	isStandardRangeString bool,
	lowerStringStartIndex,
	upperStringEndIndex int,
	rangeLower,
	rangeUpper SegInt) {
	if cache := seg.getCache(); cache != nil {
		if cache.cachedWildcardString == nil {
			if seg.IsFullRange() {
				cacheStrPtr(&cache.cachedWildcardString, &segmentWildcardStr)
			} else if isStandardRangeString && rangeLower == seg.getSegmentValue() && rangeUpper == seg.getUpperSegmentValue() {
				str := addressStr[lowerStringStartIndex:upperStringEndIndex]
				cacheStrPtr(&cache.cachedWildcardString, &str)
			}
		}
	}
}

func (seg *ipAddressSegmentInternal) toIPAddressSegment() *IPAddressSegment {
	return (*IPAddressSegment)(unsafe.Pointer(seg))
}

//// only needed for godoc / pkgsite

// GetBitCount returns the number of bits in each value comprising this address item
func (seg *ipAddressSegmentInternal) GetBitCount() BitCount {
	return seg.addressSegmentInternal.GetBitCount()
}

// GetByteCount returns the number of bytes required for each value comprising this address item.
func (seg *ipAddressSegmentInternal) GetByteCount() int {
	return seg.addressSegmentInternal.GetByteCount()
}

func (seg *ipAddressSegmentInternal) GetValue() *BigDivInt {
	return seg.addressSegmentInternal.GetValue()
}

func (seg *ipAddressSegmentInternal) GetUpperValue() *BigDivInt {
	return seg.addressSegmentInternal.GetUpperValue()
}

func (seg *ipAddressSegmentInternal) Bytes() []byte {
	return seg.addressSegmentInternal.Bytes()
}

func (seg *ipAddressSegmentInternal) UpperBytes() []byte {
	return seg.addressSegmentInternal.UpperBytes()
}

func (seg *ipAddressSegmentInternal) CopyBytes(bytes []byte) []byte {
	return seg.addressSegmentInternal.CopyBytes(bytes)
}

func (seg *ipAddressSegmentInternal) CopyUpperBytes(bytes []byte) []byte {
	return seg.addressSegmentInternal.CopyUpperBytes(bytes)
}

// IsZero returns whether this segment matches exactly the value of zero
func (seg *ipAddressSegmentInternal) IsZero() bool {
	return seg.addressSegmentInternal.IsZero()
}

// IncludesZero returns whether this segment includes the value of zero within its range
func (seg *ipAddressSegmentInternal) IncludesZero() bool {
	return seg.addressSegmentInternal.IncludesZero()
}

// IsMax returns whether this segment matches exactly the maximum possible value, the value whose bits are all ones
func (seg *ipAddressSegmentInternal) IsMax() bool {
	return seg.addressSegmentInternal.IsMax()
}

// IncludesMax returns whether this segment includes the max value, the value whose bits are all ones, within its range
func (seg *ipAddressSegmentInternal) IncludesMax() bool {
	return seg.addressSegmentInternal.IncludesMax()
}

// IsFullRange returns whether the segment range includes all possible values for its bit length.
//
//  This is true if and only if both IncludesZero and IncludesMax return true.
func (seg *ipAddressSegmentInternal) IsFullRange() bool {
	return seg.addressSegmentInternal.IsFullRange()
}

// ContainsPrefixBlock returns whether the division range includes the block of values for the given prefix length
func (seg *ipAddressSegmentInternal) ContainsPrefixBlock(prefixLen BitCount) bool {
	return seg.addressSegmentInternal.ContainsPrefixBlock(prefixLen)
}

// ContainsSinglePrefixBlock returns whether the segment range matches exactly the block of values for the given prefix length and has just a single prefix for that prefix length.
func (seg *ipAddressSegmentInternal) ContainsSinglePrefixBlock(prefixLen BitCount) bool {
	return seg.addressSegmentInternal.ContainsSinglePrefixBlock(prefixLen)
}

// GetMinPrefixLenForBlock returns the smallest prefix length such that this segment includes the block of all values for that prefix length.
//
// If the entire range can be described this way, then this method returns the same value as GetPrefixLenForSingleBlock.
//
// There may be a single prefix, or multiple possible prefix values in this item for the returned prefix length.
// Use GetPrefixLenForSingleBlock to avoid the case of multiple prefix values.
//
// If this segment represents a single value, this returns the bit count.
func (seg *ipAddressSegmentInternal) GetMinPrefixLenForBlock() BitCount {
	return seg.addressSegmentInternal.GetMinPrefixLenForBlock()
}

// GetPrefixLenForSingleBlock returns a prefix length for which there is only one prefix in this segment,
// and the range of values in this segment matches the block of all values for that prefix.
//
// If the range of segment values can be described this way, then this method returns the same value as GetMinPrefixLenForBlock.
//
// If no such prefix length exists, returns nil.
//
// If this segment represents a single value, this returns the bit count of the segment.
func (seg *ipAddressSegmentInternal) GetPrefixLenForSingleBlock() PrefixLen {
	return seg.addressSegmentInternal.GetPrefixLenForSingleBlock()
}

func (seg *ipAddressSegmentInternal) IsSinglePrefix(divisionPrefixLength BitCount) bool {
	return seg.addressSegmentInternal.IsSinglePrefix(divisionPrefixLength)
}

// PrefixContains returns whether the prefix values in the prefix of the given segment are also prefix values in this segment.
// It returns whether the prefix of this segment contains the prefix of the given segment.
func (seg *ipAddressSegmentInternal) PrefixContains(other AddressSegmentType, prefixLength BitCount) bool {
	return seg.addressSegmentInternal.PrefixContains(other, prefixLength)
}

// PrefixEqual returns whether the prefix bits of this segment match the same bits of the given segment.
// It returns whether the two segments share the same range of prefix values using the given prefix length.
func (seg *ipAddressSegmentInternal) PrefixEqual(other AddressSegmentType, prefixLength BitCount) bool {
	return seg.addressSegmentInternal.PrefixEqual(other, prefixLength)
}

func (seg *ipAddressSegmentInternal) GetSegmentValue() SegInt {
	return seg.addressSegmentInternal.GetSegmentValue()
}

func (seg *ipAddressSegmentInternal) GetUpperSegmentValue() SegInt {
	return seg.addressSegmentInternal.GetUpperSegmentValue()
}

func (seg *ipAddressSegmentInternal) Matches(value SegInt) bool {
	return seg.addressSegmentInternal.Matches(value)
}

func (seg *ipAddressSegmentInternal) MatchesWithMask(value, mask SegInt) bool {
	return seg.addressSegmentInternal.MatchesWithMask(value, mask)
}

func (seg *ipAddressSegmentInternal) MatchesValsWithMask(lowerValue, upperValue, mask SegInt) bool {
	return seg.addressSegmentInternal.MatchesValsWithMask(lowerValue, upperValue, mask)
}

func (seg *ipAddressSegmentInternal) GetPrefixCountLen(segmentPrefixLength BitCount) *big.Int {
	return seg.addressSegmentInternal.GetPrefixCountLen(segmentPrefixLength)
}

func (seg *ipAddressSegmentInternal) GetPrefixValueCountLen(segmentPrefixLength BitCount) SegIntCount {
	return seg.addressSegmentInternal.GetPrefixValueCountLen(segmentPrefixLength)
}

func (seg *ipAddressSegmentInternal) GetValueCount() SegIntCount {
	return seg.addressSegmentInternal.GetValueCount()
}

// GetMaxValue gets the maximum possible value for this type or version of segment, determined by the number of bits.
//
// For the highest range value of this particular segment, use GetUpperSegmentValue.
func (seg *ipAddressSegmentInternal) GetMaxValue() SegInt {
	return seg.addressSegmentInternal.GetMaxValue()
}

// TestBit returns true if the bit in the lower value of this segment at the given index is 1, where index 0 refers to the least significant bit.
// In other words, it computes (bits & (1 << n)) != 0), using the lower value of this section.
// TestBit will panic if n < 0, or if it matches or exceeds the bit count of this item.
func (seg *ipAddressSegmentInternal) TestBit(n BitCount) bool {
	return seg.addressSegmentInternal.TestBit(n)
}

// IsOneBit returns true if the bit in the lower value of this segment at the given index is 1, where index 0 refers to the most significant bit.
// IsOneBit will panic if bitIndex < 0, or if it is larger than the bit count of this item.
func (seg *ipAddressSegmentInternal) IsOneBit(segmentBitIndex BitCount) bool {
	return seg.addressSegmentInternal.IsOneBit(segmentBitIndex)
}

func (seg *ipAddressSegmentInternal) ToNormalizedString() string {
	return seg.addressSegmentInternal.ToNormalizedString()
}

func (seg *ipAddressSegmentInternal) ToHexString(with0xPrefix bool) (string, addrerr.IncompatibleAddressError) {
	return seg.addressSegmentInternal.ToHexString(with0xPrefix)
}

// ReverseBits returns a segment with the bits reversed.
//
// If this segment represents a range of values that cannot be reversed, then this returns an error.
//
// To be reversible, a range must include all values except possibly the largest and/or smallest, which reverse to themselves.
// Otherwise the result is not contiguous and thus cannot be represented by a sequential range of values.
//
// If perByte is true, the bits are reversed within each byte, otherwise all the bits are reversed.
func (seg *ipAddressSegmentInternal) ReverseBits(perByte bool) (res *AddressSegment, err addrerr.IncompatibleAddressError) {
	return seg.addressSegmentInternal.ReverseBits(perByte)
}

// ReverseBytes returns a segment with the bytes reversed.
//
// If this segment represents a range of values that cannot be reversed, then this returns an error.
//
// To be reversible, a range must include all values except possibly the largest and/or smallest, which reverse to themselves.
// Otherwise the result is not contiguous and thus cannot be represented by a sequential range of values.
func (seg *ipAddressSegmentInternal) ReverseBytes() (res *AddressSegment, err addrerr.IncompatibleAddressError) {
	return seg.addressSegmentInternal.ReverseBytes()
}

//// end needed for godoc / pkgsite

type IPAddressSegment struct {
	ipAddressSegmentInternal
}

// GetLower returns a segment representing just the lowest value in the range, which will be the same segment if it represents a single value.
func (seg *IPAddressSegment) GetLower() *IPAddressSegment {
	return seg.getLower().ToIP()
}

// GetUpper returns a segment representing just the highest value in the range, which will be the same segment if it represents a single value.
func (seg *IPAddressSegment) GetUpper() *IPAddressSegment {
	return seg.getUpper().ToIP()
}

// IsMultiple returns whether this segment represents multiple values
func (seg *IPAddressSegment) IsMultiple() bool {
	return seg != nil && seg.isMultiple()
}

// GetCount returns the count of possible distinct values for this item.
// If not representing multiple values, the count is 1.
//
// For instance, a segment with the value range of 3-7 has count 5.
//
// Use IsMultiple if you simply want to know if the count is greater than 1.
func (seg *IPAddressSegment) GetCount() *big.Int {
	if seg == nil {
		return bigZero()
	}
	return seg.getCount()
}

// Contains returns whether this is same type and version as the given segment and whether it contains all values in the given segment.
func (seg *IPAddressSegment) Contains(other AddressSegmentType) bool {
	if seg == nil {
		return other == nil || other.ToSegmentBase() == nil
	}
	return seg.contains(other)
}

// Equal returns whether the given segment is equal to this segment.
// Two segments are equal if they match:
//  - type/version IPv4, IPv6
//  - value range
// Prefix lengths are ignored.
func (seg *IPAddressSegment) Equal(other AddressSegmentType) bool {
	if seg == nil {
		return other == nil || other.ToDiv() == nil
		//return seg.getAddrType() == zeroType && other.(StandardDivisionType).ToDiv() == nil
	}
	return seg.equal(other)
}

func (seg *IPAddressSegment) Compare(item AddressItem) int {
	return CountComparator.Compare(seg, item)
}

// ContainsPrefixBlock returns whether the division range includes the block of values for the given prefix length
func (seg *IPAddressSegment) ContainsPrefixBlock(divisionPrefixLen BitCount) bool {
	return seg.containsPrefixBlock(divisionPrefixLen)
}

func (seg *IPAddressSegment) ToPrefixedNetworkSegment(segmentPrefixLength PrefixLen) *IPAddressSegment {
	return seg.toPrefixedNetworkDivision(segmentPrefixLength).ToIP()
}

func (seg *IPAddressSegment) ToNetworkSegment(segmentPrefixLength PrefixLen) *IPAddressSegment {
	return seg.toNetworkDivision(segmentPrefixLength, false).ToIP()
}

func (seg *IPAddressSegment) ToPrefixedHostSegment(segmentPrefixLength PrefixLen) *IPAddressSegment {
	return seg.toPrefixedHostDivision(segmentPrefixLength).ToIP()
}

func (seg *IPAddressSegment) ToHostSegment(segmentPrefixLength PrefixLen) *IPAddressSegment {
	return seg.toHostDivision(segmentPrefixLength, false).ToIP()
}

func (seg *IPAddressSegment) Iterator() IPSegmentIterator {
	if seg == nil {
		return ipSegmentIterator{nilSegIterator()}
	}
	return ipSegmentIterator{seg.iterator()}
}

func (seg *IPAddressSegment) PrefixBlockIterator() IPSegmentIterator {
	return ipSegmentIterator{seg.prefixBlockIterator()}
}

func (seg *IPAddressSegment) PrefixedBlockIterator(segmentPrefixLen BitCount) IPSegmentIterator {
	return ipSegmentIterator{seg.prefixedBlockIterator(segmentPrefixLen)}
}

func (seg *IPAddressSegment) PrefixIterator() IPSegmentIterator {
	return ipSegmentIterator{seg.prefixIterator()}
}

// IsPrefixed returns whether this section has an associated prefix length
func (seg *IPAddressSegment) IsPrefixed() bool {
	return seg != nil && seg.isPrefixed()
}

// WithoutPrefixLen returns a segment with the same value range but without a prefix length.
func (seg *IPAddressSegment) WithoutPrefixLen() *IPAddressSegment {
	if !seg.IsPrefixed() {
		return seg
	}
	return seg.withoutPrefixLen()
}

// IsIPv4 returns true if this segment originated as an IPv4 segment.  If so, use ToIPv4 to convert back to the IPv4-specific type.
func (seg *IPAddressSegment) IsIPv4() bool {
	return seg != nil && seg.matchesIPv4Segment()
}

// IsIPv6 returns true if this segment originated as an IPv6 segment.  If so, use ToIPv6 to convert back to the IPv6-specific type.
func (seg *IPAddressSegment) IsIPv6() bool {
	return seg != nil && seg.matchesIPv6Segment()
}

func (seg *IPAddressSegment) ToSegmentBase() *AddressSegment {
	return (*AddressSegment)(unsafe.Pointer(seg))
}

func (seg *IPAddressSegment) ToDiv() *AddressDivision {
	return seg.ToSegmentBase().ToDiv()
}

func (seg *IPAddressSegment) ToIPv4() *IPv4AddressSegment {
	if seg.IsIPv4() {
		return (*IPv4AddressSegment)(seg)
	}
	return nil
}

func (seg *IPAddressSegment) ToIPv6() *IPv6AddressSegment {
	if seg.IsIPv6() {
		return (*IPv6AddressSegment)(seg)
	}
	return nil
}

func (seg *IPAddressSegment) GetString() string {
	if seg == nil {
		return nilString()
	}
	return seg.getString()
}

func (seg *IPAddressSegment) GetWildcardString() string {
	if seg == nil {
		return nilString()
	}
	return seg.getWildcardString()
}

// String produces a string that is useful when a segment string is provided with no context.
// If the segment was originally constructed as an IPv4 address segment it uses decimal, otherwise hexadecimal.
// It uses a string prefix for hex (0x), and does not use the wildcard '*', because division size is variable, so '*' is ambiguous.
// GetWildcardString is more appropriate in context with other segments or divisions.  It does not use a string prefix and uses '*' for full-range segments.
// GetString is more appropriate in context with prefix lengths, it uses zeros instead of wildcards with full prefix block ranges alongside prefix lengths.
func (seg *IPAddressSegment) String() string {
	if seg == nil {
		return nilString()
	}
	return seg.toString()
}
