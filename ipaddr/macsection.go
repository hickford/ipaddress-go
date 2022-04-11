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
	"github.com/seancfoley/ipaddress-go/ipaddr/addrstr"
)

func createMACSection(segments []*AddressDivision) *MACAddressSection {
	return &MACAddressSection{
		addressSectionInternal{
			addressDivisionGroupingInternal{
				addressDivisionGroupingBase: addressDivisionGroupingBase{
					divisions: standardDivArray{segments},
					addrType:  macType,
					cache: &valueCache{
						stringCache: stringCache{
							macStringCache: &macStringCache{},
						},
					},
				},
			},
		},
	}
}

func NewMACSection(segments []*MACAddressSegment) *MACAddressSection {
	return createMACSectionFromSegs(segments)
}

func createMACSectionFromSegs(orig []*MACAddressSegment) *MACAddressSection {
	segCount := len(orig)
	newSegs := make([]*AddressDivision, segCount)
	var newPref PrefixLen
	isMultiple := false
	if segCount != 0 {
		isBlock := true
		for i := segCount - 1; i >= 0; i-- {
			segment := orig[i]
			if segment == nil {
				segment = zeroMACSeg
				if isBlock && i != segCount-1 {
					newPref = getNetworkPrefixLen(MACBitsPerSegment, MACBitsPerSegment, i)
					isBlock = false
				}
			} else {
				if isBlock {
					minPref := segment.GetMinPrefixLenForBlock()
					if minPref > 0 {
						if minPref != MACBitsPerSegment || i != segCount-1 {
							newPref = getNetworkPrefixLen(MACBitsPerSegment, minPref, i)
						}
						isBlock = false
					}
				}
				isMultiple = isMultiple || segment.isMultiple()
			}
			newSegs[i] = segment.ToDiv()
		}
		if isBlock {
			newPref = cacheBitCount(0)
		}
	}
	res := createMACSection(newSegs)
	res.isMult = isMultiple
	res.prefixLength = newPref
	return res
}

func newMACSectionParsed(segments []*AddressDivision, isMultiple bool) (res *MACAddressSection) {
	res = createMACSection(segments)
	res.initImplicitPrefLen(MACBitsPerSegment)
	res.isMult = isMultiple
	return
}

func newMACSectionEUI(segments []*AddressDivision) (res *MACAddressSection) {
	res = createMACSection(segments)
	res.initMultAndImplicitPrefLen(MACBitsPerSegment)
	return
}

func NewMACSectionFromBytes(bytes []byte, segmentCount int) (res *MACAddressSection, err addrerr.AddressValueError) {
	if segmentCount < 0 {
		segmentCount = len(bytes)
	}
	expectedByteCount := segmentCount
	segments, err := toSegments(
		bytes,
		segmentCount,
		MACBytesPerSegment,
		MACBitsPerSegment,
		macNetwork.getAddressCreator(),
		nil)
	if err == nil {
		// note prefix len is nil
		res = createMACSection(segments)
		if expectedByteCount == len(bytes) {
			bytes = cloneBytes(bytes)
			res.cache.bytesCache = &bytesCache{lowerBytes: bytes}
			if !res.isMult { // not a prefix block
				res.cache.bytesCache.upperBytes = bytes
			}
		}
	}
	return
}

func NewMACSectionFromUint64(bytes uint64, segmentCount int) (res *MACAddressSection) {
	if segmentCount < 0 {
		segmentCount = MediaAccessControlSegmentCount
	}
	segments := createSegmentsUint64(
		segmentCount,
		0,
		uint64(bytes),
		MACBytesPerSegment,
		MACBitsPerSegment,
		macNetwork.getAddressCreator(),
		nil)
	// note prefix len is nil
	res = createMACSection(segments)
	return
}

func NewMACSectionFromVals(vals MACSegmentValueProvider, segmentCount int) (res *MACAddressSection) {
	res = NewMACSectionFromRange(vals, nil, segmentCount)
	return
}

func NewMACSectionFromRange(vals, upperVals MACSegmentValueProvider, segmentCount int) (res *MACAddressSection) {
	if segmentCount < 0 {
		segmentCount = 0
	}
	segments, isMultiple := createSegments(
		WrappedMACSegmentValueProvider(vals),
		WrappedMACSegmentValueProvider(upperVals),
		segmentCount,
		MACBitsPerSegment,
		macNetwork.getAddressCreator(),
		nil)
	res = createMACSection(segments)
	if isMultiple {
		res.initImplicitPrefLen(MACBitsPerSegment)
		res.isMult = true
	}
	return
}

type MACAddressSection struct {
	addressSectionInternal
}

func (section *MACAddressSection) Contains(other AddressSectionType) bool {
	if section == nil {
		return other == nil || other.ToSectionBase() == nil
	}
	return section.contains(other)
}

func (section *MACAddressSection) Equal(other AddressSectionType) bool {
	if section == nil {
		return other == nil || other.ToSectionBase() == nil
	}
	return section.equal(other)
}

func (section *MACAddressSection) Compare(item AddressItem) int {
	return CountComparator.Compare(section, item)
}

func (section *MACAddressSection) CompareSize(other StandardDivGroupingType) int {
	if section == nil {
		if other != nil && other.ToDivGrouping() != nil {
			// we have size 0, other has size >= 1
			return -1
		}
		return 0
	}
	return section.compareSize(other)
}

func (section *MACAddressSection) GetBitsPerSegment() BitCount {
	return MACBitsPerSegment
}

func (section *MACAddressSection) GetBytesPerSegment() int {
	return MACBytesPerSegment
}

// GetCount returns the count of possible distinct values for this item.
// If not representing multiple values, the count is 1,
// unless this is a division grouping with no divisions, or an address section with no segments, in which case it is 0.
func (section *MACAddressSection) GetCount() *big.Int {
	if section == nil {
		return bigZero()
	}
	return section.cacheCount(func() *big.Int {
		return count(func(index int) uint64 {
			return section.GetSegment(index).GetValueCount()
		}, section.GetSegmentCount(), 6, 0x7fffffffffffff)
	})
}

// IsMultiple returns  whether this section represents multiple values
func (section *MACAddressSection) IsMultiple() bool {
	return section != nil && section.isMultiple()
}

// IsPrefixed returns whether this section has an associated prefix length
func (section *MACAddressSection) IsPrefixed() bool {
	return section != nil && section.isPrefixed()
}

// GetPrefixCount returns the number of distinct prefix values in this item.
//
// The prefix length is given by GetPrefixLen.
//
// If this has a non-nil prefix length, returns the number of distinct prefix values.
//
// If this has a nil prefix length, returns the same value as GetCount
func (section *MACAddressSection) GetPrefixCount() *big.Int {
	return section.cachePrefixCount(func() *big.Int {
		return section.GetPrefixCountLen(section.getPrefixLen().bitCount())
	})
}

// GetPrefixCountLen returns the number of distinct prefix values in this item for the given prefix length
func (section *MACAddressSection) GetPrefixCountLen(prefixLen BitCount) *big.Int {
	if prefixLen <= 0 {
		return bigOne()
	} else if bc := section.GetBitCount(); prefixLen >= bc {
		return section.GetCount()
	}
	networkSegmentIndex := getNetworkSegmentIndex(prefixLen, section.GetBytesPerSegment(), section.GetBitsPerSegment())
	hostSegmentIndex := getHostSegmentIndex(prefixLen, section.GetBytesPerSegment(), section.GetBitsPerSegment())
	return section.calcCount(func() *big.Int {
		return count(func(index int) uint64 {
			if (networkSegmentIndex == hostSegmentIndex) && index == networkSegmentIndex {
				segmentPrefixLength := getPrefixedSegmentPrefixLength(section.GetBitsPerSegment(), prefixLen, index)
				return getPrefixValueCount(section.GetSegment(index).ToSegmentBase(), segmentPrefixLength.bitCount())
			}
			return section.GetSegment(index).GetValueCount()
		}, networkSegmentIndex+1, 6, 0x7fffffffffffff)
	})
}

// GetBlockCount returns the count of distinct values in the given number of initial (more significant) segments.
func (section *MACAddressSection) GetBlockCount(segments int) *big.Int {
	return section.calcCount(func() *big.Int {
		return count(func(index int) uint64 {
			return section.GetSegment(index).GetValueCount()
		},
			segments, 6, 0x7fffffffffffff)
	})
}

func (section *MACAddressSection) WithoutPrefixLen() *MACAddressSection {
	if !section.IsPrefixed() {
		return section
	}
	return section.withoutPrefixLen().ToMAC()
}

func (section *MACAddressSection) SetPrefixLen(prefixLen BitCount) *MACAddressSection {
	return section.setPrefixLen(prefixLen).ToMAC()
}

func (section *MACAddressSection) SetPrefixLenZeroed(prefixLen BitCount) (*MACAddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.setPrefixLenZeroed(prefixLen)
	return res.ToMAC(), err
}

func (section *MACAddressSection) AdjustPrefixLen(prefixLen BitCount) *AddressSection {
	return section.adjustPrefixLen(prefixLen).ToSectionBase()
}

func (section *MACAddressSection) AdjustPrefixLenZeroed(prefixLen BitCount) (*AddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.adjustPrefixLenZeroed(prefixLen)
	return res.ToSectionBase(), err
}

func (section *MACAddressSection) AssignPrefixForSingleBlock() *MACAddressSection {
	return section.assignPrefixForSingleBlock().ToMAC()
}

func (section *MACAddressSection) AssignMinPrefixForBlock() *MACAddressSection {
	return section.assignMinPrefixForBlock().ToMAC()
}

func (section *MACAddressSection) GetSegment(index int) *MACAddressSegment {
	return section.getDivision(index).ToMAC()
}

func (section *MACAddressSection) ToDivGrouping() *AddressDivisionGrouping {
	return section.ToSectionBase().ToDivGrouping()
}

func (section *MACAddressSection) ToSectionBase() *AddressSection {
	return (*AddressSection)(section)
}

func (section *MACAddressSection) Wrap() WrappedAddressSection {
	return WrapSection(section.ToSectionBase())
}

// GetTrailingSection gets the subsection from the series starting from the given index.
// The first segment is at index 0.
func (section *MACAddressSection) GetTrailingSection(index int) *MACAddressSection {
	return section.GetSubSection(index, section.GetSegmentCount())
}

// GetSubSection gets the subsection from the series starting from the given index and ending just before the give endIndex.
// The first segment is at index 0.
func (section *MACAddressSection) GetSubSection(index, endIndex int) *MACAddressSection {
	return section.getSubSection(index, endIndex).ToMAC()
}

// CopySubSegments copies the existing segments from the given start index until but not including the segment at the given end index,
// into the given slice, as much as can be fit into the slice, returning the number of segments copied
func (section *MACAddressSection) CopySubSegments(start, end int, segs []*MACAddressSegment) (count int) {
	return section.visitSubDivisions(start, end, func(index int, div *AddressDivision) bool { segs[index] = div.ToMAC(); return false }, len(segs))
}

// CopySubSegments copies the existing segments from the given start index until but not including the segment at the given end index,
// into the given slice, as much as can be fit into the slice, returning the number of segments copied
func (section *MACAddressSection) CopySegments(segs []*MACAddressSegment) (count int) {
	return section.visitDivisions(func(index int, div *AddressDivision) bool { segs[index] = div.ToMAC(); return false }, len(segs))
}

// GetSegments returns a slice with the address segments.  The returned slice is not backed by the same array as this section.
func (section *MACAddressSection) GetSegments() (res []*MACAddressSegment) {
	res = make([]*MACAddressSegment, section.GetSegmentCount())
	section.CopySegments(res)
	return
}

func (section *MACAddressSection) GetLower() *MACAddressSection {
	return section.getLower().ToMAC()
}

func (section *MACAddressSection) GetUpper() *MACAddressSection {
	return section.getUpper().ToMAC()
}

func (section *MACAddressSection) Uint64Value() uint64 {
	return section.getLongValue(true)
}

func (section *MACAddressSection) UpperUint64Value() uint64 {
	return section.getLongValue(false)
}

func (section *MACAddressSection) getLongValue(lower bool) (result uint64) {
	segCount := section.GetSegmentCount()
	if segCount == 0 {
		return
	}
	seg := section.GetSegment(0)
	if lower {
		result = uint64(seg.GetSegmentValue())
	} else {
		result = uint64(seg.GetUpperSegmentValue())
	}
	bitsPerSegment := section.GetBitsPerSegment()
	for i := 1; i < segCount; i++ {
		result = result << uint(bitsPerSegment)
		seg = section.GetSegment(i)
		if lower {
			result |= uint64(seg.GetSegmentValue())
		} else {
			result |= uint64(seg.GetUpperSegmentValue())
		}
	}
	return
}

func (section *MACAddressSection) ToPrefixBlock() *MACAddressSection {
	return section.toPrefixBlock().ToMAC()
}

func (section *MACAddressSection) ToPrefixBlockLen(prefLen BitCount) *MACAddressSection {
	return section.toPrefixBlockLen(prefLen).ToMAC()
}

func (section *MACAddressSection) ToBlock(segmentIndex int, lower, upper SegInt) *MACAddressSection {
	return section.toBlock(segmentIndex, lower, upper).ToMAC()
}

func (section *MACAddressSection) Iterator() MACSectionIterator {
	if section == nil {
		return macSectionIterator{nilSectIterator()}
	}
	return macSectionIterator{section.sectionIterator(nil)}
}

func (section *MACAddressSection) PrefixIterator() MACSectionIterator {
	return macSectionIterator{section.prefixIterator(false)}
}

func (section *MACAddressSection) PrefixBlockIterator() MACSectionIterator {
	return macSectionIterator{section.prefixIterator(true)}
}

// IncrementBoundary returns the item that is the given increment from the range boundaries of this item.
//
// If the given increment is positive, adds the value to the highest ({@link #getUpper()}) in the range to produce a new item.
// If the given increment is negative, adds the value to the lowest ({@link #getLower()}) in the range to produce a new item.
// If the increment is zero, returns this.
//
// If this represents just a single value, this item is simply incremented by the given increment value, positive or negative.
//
// On overflow or underflow, IncrementBoundary returns nil.
func (section *MACAddressSection) IncrementBoundary(increment int64) *MACAddressSection {
	return section.incrementBoundary(increment).ToMAC()
}

func (section *MACAddressSection) IsAdaptiveZero() bool {
	return section != nil && section.matchesZeroGrouping()
}

func getMacMaxValueLong(segmentCount int) uint64 {
	return macMaxValues[segmentCount]
}

var macMaxValues = []uint64{
	0,
	MACMaxValuePerSegment,
	0xffff,
	0xffffff,
	0xffffffff,
	0xffffffffff,
	0xffffffffffff,
	0xffffffffffffff,
	0xffffffffffffffff}

// Increment returns the item that is the given increment upwards into the range,
// with the increment of 0 returning the first in the range.
//
// If the increment i matches or exceeds the range count c, then i - c + 1
// is added to the upper item of the range.
// An increment matching the count gives you the item just above the highest in the range.
//
// If the increment is negative, it is added to the lowest of the range.
// To get the item just below the lowest of the range, use the increment -1.
//
// If this represents just a single value, the item is simply incremented by the given increment, positive or negative.
//
// If this item represents multiple values, a positive increment i is equivalent i + 1 values from the iterator and beyond.
// For instance, a increment of 0 is the first value from the iterator, an increment of 1 is the second value from the iterator, and so on.
// An increment of a negative value added to the count is equivalent to the same number of iterator values preceding the last value of the iterator.
// For instance, an increment of count - 1 is the last value from the iterator, an increment of count - 2 is the second last value, and so on.
//
// On overflow or underflow, Increment returns nil.
func (section *MACAddressSection) Increment(incrementVal int64) *MACAddressSection {
	if incrementVal == 0 && !section.isMultiple() {
		return section
	}
	segCount := section.GetSegmentCount()
	lowerValue := section.Uint64Value()
	upperValue := section.UpperUint64Value()
	count := section.GetCount()
	countMinus1 := count.Sub(count, bigOneConst()).Uint64()
	isOverflow := checkOverflow(incrementVal, lowerValue, upperValue, countMinus1, getMacMaxValueLong(segCount))
	if isOverflow {
		return nil
	}
	return increment(
		section.ToSectionBase(),
		incrementVal,
		macNetwork.getAddressCreator(),
		countMinus1,
		section.Uint64Value(),
		section.UpperUint64Value(),
		section.addressSectionInternal.getLower,
		section.addressSectionInternal.getUpper,
		section.getPrefixLen()).ToMAC()
}

func (section *MACAddressSection) ReverseBits(perByte bool) (*MACAddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.reverseBits(perByte)
	return res.ToMAC(), err
}

func (section *MACAddressSection) ReverseBytes() *MACAddressSection {
	return section.ReverseSegments()
}

func (section *MACAddressSection) ReverseSegments() *MACAddressSection {
	if section.GetSegmentCount() <= 1 {
		if section.IsPrefixed() {
			return section.WithoutPrefixLen()
		}
		return section
	}
	res, _ := section.reverseSegments(
		func(i int) (*AddressSegment, addrerr.IncompatibleAddressError) {
			return section.GetSegment(i).ToSegmentBase(), nil
		},
	)
	return res.ToMAC()
}

func (section *MACAddressSection) Append(other *MACAddressSection) *MACAddressSection {
	count := section.GetSegmentCount()
	return section.ReplaceLen(count, count, other, 0, other.GetSegmentCount())
}

func (section *MACAddressSection) Insert(index int, other *MACAddressSection) *MACAddressSection {
	return section.ReplaceLen(index, index, other, 0, other.GetSegmentCount())
}

// Replace replaces the segments of this section starting at the given index with the given replacement segments
func (section *MACAddressSection) Replace(index int, replacement *MACAddressSection) *MACAddressSection {
	return section.ReplaceLen(index, index+replacement.GetSegmentCount(), replacement, 0, replacement.GetSegmentCount())
}

// ReplaceLen replaces segments starting from startIndex and ending before endIndex with the segments starting at replacementStartIndex and
// ending before replacementEndIndex from the replacement section
func (section *MACAddressSection) ReplaceLen(startIndex, endIndex int, replacement *MACAddressSection, replacementStartIndex, replacementEndIndex int) *MACAddressSection {
	return section.replaceLen(startIndex, endIndex, replacement.ToSectionBase(), replacementStartIndex, replacementEndIndex, macBitsToSegmentBitshift).ToMAC()
}

var (
	canonicalWildcards = new(addrstr.WildcardsBuilder).SetRangeSeparator(MacDashedSegmentRangeSeparatorStr).SetWildcard(SegmentWildcardStr).ToWildcards()

	macNormalizedParams  = new(addrstr.MACStringOptionsBuilder).SetExpandedSegments(true).ToOptions()
	macCanonicalParams   = new(addrstr.MACStringOptionsBuilder).SetSeparator(MACDashSegmentSeparator).SetExpandedSegments(true).SetWildcards(canonicalWildcards).ToOptions()
	macCompressedParams  = new(addrstr.MACStringOptionsBuilder).ToOptions()
	dottedParams         = new(addrstr.MACStringOptionsBuilder).SetSeparator(MacDottedSegmentSeparator).SetExpandedSegments(true).ToOptions()
	spaceDelimitedParams = new(addrstr.MACStringOptionsBuilder).SetSeparator(MacSpaceSegmentSeparator).SetExpandedSegments(true).ToOptions()
)

func (section *MACAddressSection) ToHexString(with0xPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toHexString(with0xPrefix)
}

func (section *MACAddressSection) ToOctalString(with0Prefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toOctalString(with0Prefix)
}

func (section *MACAddressSection) ToBinaryString(with0bPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toBinaryString(with0bPrefix)
}

// ToCanonicalString produces a canonical string.
//
//If this section has a prefix length, it will be included in the string.
func (section *MACAddressSection) ToCanonicalString() string {
	if section == nil {
		return nilString()
	}
	cache := section.getStringCache()
	if cache == nil {
		return section.toCustomString(macCanonicalParams)
	}
	return cacheStr(&cache.canonicalString,
		func() string {
			return section.toCustomString(macCanonicalParams)
		})
}

func (section *MACAddressSection) ToNormalizedString() string {
	if section == nil {
		return nilString()
	}
	cch := section.getStringCache()
	if cch == nil {
		return section.toCustomString(macNormalizedParams)
	}
	strp := &cch.normalizedMACString
	return cacheStr(strp,
		func() string {
			return section.toCustomString(macNormalizedParams)
		})
}

func (section *MACAddressSection) ToCompressedString() string {
	if section == nil {
		return nilString()
	}
	cache := section.getStringCache()
	if cache == nil {
		return section.toCustomString(macCompressedParams)
	}
	return cacheStr(&cache.compressedMACString,
		func() string {
			return section.toCustomString(macCompressedParams)
		})
}

// ToDottedString produces the dotted hexadecimal format aaaa.bbbb.cccc
func (section *MACAddressSection) ToDottedString() (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	dottedGrouping, err := section.GetDottedGrouping()
	if err != nil {
		return "", err
	}
	cache := section.getStringCache()
	if cache == nil {
		return toNormalizedString(dottedParams, dottedGrouping), nil
	}
	return cacheStrErr(&cache.dottedString,
		func() (string, addrerr.IncompatibleAddressError) {
			return toNormalizedString(dottedParams, dottedGrouping), nil
		})
}

func (section *MACAddressSection) GetDottedGrouping() (*AddressDivisionGrouping, addrerr.IncompatibleAddressError) {
	segmentCount := section.GetSegmentCount()
	var newSegs []*AddressDivision
	newSegmentBitCount := section.GetBitsPerSegment() << 1
	var segIndex, newSegIndex int
	newSegmentCount := (segmentCount + 1) >> 1
	newSegs = make([]*AddressDivision, newSegmentCount)
	bitsPerSeg := section.GetBitsPerSegment()
	for segIndex+1 < segmentCount {
		segment1 := section.GetSegment(segIndex)
		segIndex++
		segment2 := section.GetSegment(segIndex)
		segIndex++
		if segment1.isMultiple() && !segment2.IsFullRange() {
			return nil, &incompatibleAddressError{addressError{key: "ipaddress.error.invalid.joined.ranges"}}
		}
		val := (segment1.GetSegmentValue() << uint(bitsPerSeg)) | segment2.GetSegmentValue()
		upperVal := (segment1.GetUpperSegmentValue() << uint(bitsPerSeg)) | segment2.GetUpperSegmentValue()
		vals := NewRangeDivision(DivInt(val), DivInt(upperVal), newSegmentBitCount)
		newSegs[newSegIndex] = createAddressDivision(vals)
		newSegIndex++
	}
	if segIndex < segmentCount {
		segment := section.GetSegment(segIndex)
		val := segment.GetSegmentValue() << uint(bitsPerSeg)
		upperVal := segment.GetUpperSegmentValue() << uint(bitsPerSeg)
		vals := NewRangeDivision(DivInt(val), DivInt(upperVal), newSegmentBitCount)
		newSegs[newSegIndex] = createAddressDivision(vals)
	}
	grouping := createInitializedGrouping(newSegs, section.getPrefixLen())
	return grouping, nil
}

// ToSpaceDelimitedString produces a string delimited by spaces: aa bb cc dd ee ff
func (section *MACAddressSection) ToSpaceDelimitedString() string {
	if section == nil {
		return nilString()
	}
	cache := section.getStringCache()
	if cache == nil {
		return section.toCustomString(spaceDelimitedParams)
	}
	return cacheStr(&cache.spaceDelimitedString,
		func() string {
			return section.toCustomString(spaceDelimitedParams)
		})
}

func (section *MACAddressSection) ToDashedString() string {
	if section == nil {
		return nilString()
	}
	return section.ToCanonicalString()
}

func (section *MACAddressSection) ToColonDelimitedString() string {
	if section == nil {
		return nilString()
	}
	return section.ToNormalizedString()
}

func (section *MACAddressSection) String() string {
	if section == nil {
		return nilString()
	}
	return section.toString()
}

func (section *MACAddressSection) GetSegmentStrings() []string {
	if section == nil {
		return nil
	}
	return section.getSegmentStrings()
}
