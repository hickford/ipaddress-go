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

import "math/big"

var (
	// CountComparator compares by count first, then by value
	CountComparator = AddressComparator{countComparator{}}

	// HighValueComparator compares by high value first, then low, then count
	HighValueComparator = AddressComparator{valueComparator{compareHighValue: true}}

	// LowValueComparator compares by low value first, then high, then count
	LowValueComparator = AddressComparator{valueComparator{}}

	// With the reverse comparators, ordering with the secondary values (higher or lower) follow a reverse ordering than the primary values (lower or higher)

	// ReverseHighValueComparator is like HighValueComparator but when comparing the low value, reverses the comparison
	ReverseHighValueComparator = AddressComparator{valueComparator{compareHighValue: true, flipSecond: true}}

	// ReverseLowValueComparator is like LowValueComparator but when comparing the high value, reverses the comparison
	ReverseLowValueComparator = AddressComparator{valueComparator{flipSecond: true}}
)

type componentComparator interface {
	compareSectionParts(one, two *AddressSection) int

	compareParts(one, two AddressDivisionSeries) int

	compareSegValues(oneUpper, oneLower, twoUpper, twoLower SegInt) int

	compareValues(oneUpper, oneLower, twoUpper, twoLower uint64) int

	compareLargeValues(oneUpper, oneLower, twoUpper, twoLower *big.Int) int
}

const (
	ipv6sectype          = 7
	ipv4sectype          = 6
	ipsectype            = 5
	macsectype           = 4
	sectype              = 3
	ipv6v4groupingtype   = 2
	largegroupingtype    = -2
	standardgroupingtype = -3
	adaptivezerotype     = -4
)

const (
	ipv6segtype     = 6
	ipv4segtype     = 5
	ipsegtype       = 4
	macsegtype      = 3
	segtype         = 1
	largedivtype    = -2
	standarddivtype = 0
)

const (
	ipv6rangetype = 2
	ipv4rangetype = 1
	iprangetype   = 0
)

func mapDivision(genericDiv DivisionType) int {
	if div, ok := genericDiv.(StandardDivisionType); ok {
		addrDiv := div.ToDiv()
		if addrDiv.IsIPv6() {
			return ipv6segtype
		} else if addrDiv.IsIPv4() {
			return ipv4segtype
		} else if addrDiv.IsMAC() {
			return macsegtype
		} else if addrDiv.IsIP() {
			return ipsegtype
		} else if addrDiv.IsSegmentBase() {
			return segtype
		}
		return standarddivtype
	}
	//else if(div instanceof IPAddressLargeDivision) { //TODO LATER IPAddressLargeDivisionGrouping
	//	return -1;
	//}
	return standarddivtype
}

func mapGrouping(grouping StandardDivGroupingType) int {
	group := grouping.ToDivGrouping()
	if group.IsAdaptiveZero() {
		// The zero grouping can represent a zero-length section of any address type.
		// This is necessary because sections and groupings have no init() method to ensure zero-sections are always assigned an address type.
		// We need the zero grouping to be less than everything else or more than everything else for comparison consistency.
		// Empty sections org groupings that have an address type are not considered equal.  They can represent only one address type.
		// This is similar to the fact that a MAC section and an IPv4 section can be structurally identical but not equal due to the type.
		return adaptivezerotype
	} else if group.IsIPv6() {
		return ipv6sectype
	} else if group.IsMixedIPv6v4() {
		return ipv6v4groupingtype
	} else if group.IsIPv4() {
		return ipv4sectype
	} else if group.IsMAC() {
		return macsectype
	} else if group.IsIP() {
		return ipsectype
	} else if group.isAddressSection() {
		return sectype
	}
	return standardgroupingtype
	//} //} else if(series instanceof IPAddressLargeDivisionGrouping) {
	//	return -2;
	//}
	//return 0
}

func mapRange(rng *IPAddressSeqRange) int {
	//rng := rngType.ToIP()
	if rng.IsIPv4() {
		return ipv4rangetype
	} else if rng.IsIPv6() {
		return ipv6rangetype
	}
	return iprangetype
}

// AddressComparator has methods to compare addresses, or sections, or division series, or segments, or divisions, or sequential ranges.
// AddressComparator also allows you to compare any two instances of any such address items, using the Compare method.
// The zero value acts like CountComparator, the default comparator.
type AddressComparator struct {
	componentComparator
}

// CompareAddresses compares any two addresses (including different versions or address types)
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareAddresses(one, two AddressType) int {
	var oneAddr, twoAddr *Address
	if one != nil {
		oneAddr = one.ToAddressBase()
	}
	if two != nil {
		twoAddr = two.ToAddressBase()
	}
	if oneAddr == nil {
		if twoAddr == nil {
			return 0
		}
		return -1
	} else if twoAddr == nil {
		return 1
	}
	result := comp.CompareAddressSections(oneAddr.GetSection(), twoAddr.GetSection())
	if result == 0 {
		if oneIPv6 := oneAddr.ToIPv6(); oneIPv6 != nil {
			twoIPv6 := twoAddr.ToIPv6()
			oneZone := oneIPv6.zone
			twoZone := twoIPv6.zone
			if oneZone == twoZone {
				return 0
			} else if oneZone < twoZone {
				return -1
			}
			return 1
		}
	}
	return result
}

// CompareAddressSections compares any two address sections (including from different versions or address types)
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareAddressSections(one, two AddressSectionType) int {
	var oneSec, twoSec *AddressSection
	if one != nil {
		oneSec = one.ToSectionBase()
	}
	if two != nil {
		twoSec = two.ToSectionBase()
	}
	if oneSec == nil {
		if twoSec == nil {
			return 0
		}
		return -1
	} else if twoSec == nil {
		return 1
	}
	result := mapGrouping(oneSec) - mapGrouping(twoSec)
	if result != 0 {
		return result
	}
	if comp.componentComparator == nil {
		comp.componentComparator = countComparator{}
	}
	return comp.compareSectionParts(oneSec, twoSec)
}

func unwrapWrapper(item AddressDivisionSeries) AddressDivisionSeries {
	if wrapper, ok := item.(ExtendedIPSegmentSeries); ok {
		return wrapper.Unwrap()
	}
	return item
}

// CompareSeries compares any two address division series (including from different versions or address types)
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareSeries(one, two AddressDivisionSeries) int {
	one = unwrapWrapper(one)
	two = unwrapWrapper(two)
	if addrSeries1, ok := one.(AddressType); ok {
		if addrSeries2, ok := two.(AddressType); ok {
			return comp.CompareAddresses(addrSeries1, addrSeries2)
		}
		return 1
	} else if _, ok := two.(AddressType); ok {
		return -1
	}
	// at this point they must be both groupings if not nil
	if addrSection1, ok := one.(AddressSectionType); ok {
		if addrSection2, ok := two.(AddressSectionType); ok {
			return comp.CompareAddressSections(addrSection1, addrSection2)
		}
	}
	// TODO LATER when supporting large divisions, must figure out here whether they are standard div groupings or both are large div groupings - note that if the interface is nil they can be neither
	// If they were not the same, you'd be done.  If both were standard or both were large, then you would take separate paths.
	// For now, we can be certain they are both standard.
	grouping1, _ := one.(StandardDivGroupingType) // the underscore is needed to avoid panic on nil
	grouping2, _ := two.(StandardDivGroupingType)
	var oneGrouping, twoGrouping *AddressDivisionGrouping
	if grouping1 != nil {
		oneGrouping = grouping1.ToDivGrouping()
	}
	if grouping2 != nil {
		twoGrouping = grouping2.ToDivGrouping()
	}
	if oneGrouping == nil {
		if twoGrouping == nil {
			return 0
		}
		return -1
	} else if twoGrouping == nil {
		return 1
	}
	result := mapGrouping(oneGrouping) - mapGrouping(twoGrouping)
	if result != 0 {
		return result
	}
	if comp.componentComparator == nil {
		comp.componentComparator = countComparator{}
	}
	return comp.compareParts(oneGrouping, twoGrouping)
}

// CompareSegments compares any two address segments (including from different versions or address types)
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareSegments(one, two AddressSegmentType) int {
	var oneSeg, twoSeg *AddressSegment
	if one != nil {
		oneSeg = one.ToSegmentBase()
	}
	if two != nil {
		twoSeg = two.ToSegmentBase()
	}
	if oneSeg == nil {
		if twoSeg == nil {
			return 0
		}
		return -1
	} else if twoSeg == nil {
		return 1
	}
	result := mapDivision(one) - mapDivision(two)
	if result != 0 {
		return result
	}
	if comp.componentComparator == nil {
		comp.componentComparator = countComparator{}
	}
	return comp.compareSegValues(oneSeg.GetUpperSegmentValue(), oneSeg.GetSegmentValue(), twoSeg.GetUpperSegmentValue(), twoSeg.GetSegmentValue())
}

// CompareDivisions compares any two address divisions (including from different versions or address types)
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareDivisions(one, two DivisionType) int {
	if addrSeg1, ok := one.(AddressSegmentType); ok {
		if addrSeg2, ok := two.(AddressSegmentType); ok {
			return comp.CompareSegments(addrSeg1, addrSeg2)
		}
	}
	// TODO LATER when supporting large divisions, must figure out here whether they are standard div groupings or both are large div groupings - note that if the interface is nil they can be neither
	// If they were not the same, you'd be done.  If both were standard or both were large, then you would take separate paths.
	// For now, we can be certain they are both standard.
	// The large div path would use this code after the nil checks:
	/*
		result := mapDivision(one) - mapDivision(two)
		if result != 0 {
			return result
		}
		result = int(one.GetBitCount()) - int(two.GetBitCount())
		if result != 0 {
			return result
		}
		return comp.compareLargeValues(one.GetUpperValue(), one.GetValue(), two.GetUpperValue(), two.GetValue())
	*/
	addrDiv1, _ := one.(StandardDivisionType) // the underscore is needed to avoid panic on nil
	addrDiv2, _ := two.(StandardDivisionType)
	var div1, div2 *AddressDivision
	if addrDiv1 != nil {
		div1 = addrDiv1.ToDiv()
	}
	if addrDiv2 != nil {
		div2 = addrDiv2.ToDiv()
	}
	if div1 == nil {
		if div2 == nil {
			return 0
		}
		return -1
	} else if div2 == nil {
		return 1
	}
	result := mapDivision(one) - mapDivision(two)
	if result != 0 {
		return result
	}
	result = int(one.GetBitCount() - two.GetBitCount())
	if result != 0 {
		return result
	}
	if comp.componentComparator == nil {
		comp.componentComparator = countComparator{}
	}
	return comp.compareValues(div1.GetUpperDivisionValue(), div1.GetDivisionValue(), div2.GetUpperDivisionValue(), div2.GetDivisionValue())
}

// CompareRanges compares any two IP address sequential ranges (including from different IP versions).
// It returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
func (comp AddressComparator) CompareRanges(one, two IPAddressSeqRangeType) int {
	var r1, r2 *IPAddressSeqRange
	if one != nil {
		r1 = one.ToIP()
	}
	if two != nil {
		r2 = two.ToIP()
	}
	if r1 == nil {
		if r2 == nil {
			return 0
		}
		return -1
	} else if r2 == nil {
		return 1
	}
	r1Type := mapRange(r1)
	result := r1Type - mapRange(r2)
	if result != 0 {
		return result
	}
	if comp.componentComparator == nil {
		comp.componentComparator = countComparator{}
	}
	if r1Type == ipv4rangetype { // avoid using the large values
		r1ipv4 := r1.ToIPv4()
		r2ipv4 := r2.ToIPv4()
		return comp.compareValues(uint64(r1ipv4.GetUpper().Uint32Value()), uint64(r1ipv4.GetLower().Uint32Value()), uint64(r2ipv4.GetUpper().Uint32Value()), uint64(r2ipv4.GetLower().Uint32Value()))
	}
	return comp.compareLargeValues(r1.GetUpperValue(), r1.GetValue(), r2.GetUpperValue(), r2.GetValue())
}

// Compare returns a negative integer, zero, or a positive integer if address item one is less than, equal, or greater than address item two.
// Any address item is comparable to any other.
func (comp AddressComparator) Compare(one, two AddressItem) int {
	if one == nil {
		if two == nil {
			return 0
		}
		return -1
	} else if two == nil {
		return 1
	}

	if divSeries1, ok := one.(AddressDivisionSeries); ok {
		if divSeries2, ok := two.(AddressDivisionSeries); ok {
			return comp.CompareSeries(divSeries1, divSeries2)
		} else {
			return 1
		}
	} else if div1, ok := one.(DivisionType); ok {
		if div2, ok := two.(DivisionType); ok {
			return comp.CompareDivisions(div1, div2)
		} else {
			return -1
		}
	} else if rng1, ok := one.(IPAddressSeqRangeType); ok {
		if rng2, ok := two.(IPAddressSeqRangeType); ok {
			return comp.CompareRanges(rng1, rng2)
		} else if _, ok := two.(AddressDivisionSeries); ok {
			return -1
		}
		return 1
	}
	// we've covered all known address items for one, so check two
	if _, ok := two.(AddressDivisionSeries); ok {
		return -1
	} else if _, ok := two.(DivisionType); ok {
		return 1
	} else if _, ok := two.(IPAddressSeqRangeType); ok {
		return -1
	}
	// neither are a known AddressItem type
	return int(one.GetBitCount() - two.GetBitCount())
}

type valueComparator struct {
	compareHighValue, flipSecond bool
}

func (comp valueComparator) compareSectionParts(one, two *AddressSection) int {
	sizeResult := one.GetByteCount() - two.GetByteCount()
	if sizeResult != 0 {
		return sizeResult
	}
	compareHigh := comp.compareHighValue
	for {
		segCount := one.GetSegmentCount()
		for i := 0; i < segCount; i++ {
			segOne := one.GetSegment(i)
			segTwo := two.GetSegment(i)
			var s1, s2 SegInt
			if compareHigh {
				s1 = segOne.GetUpperSegmentValue()
				s2 = segTwo.GetUpperSegmentValue()
			} else {
				s1 = segOne.GetSegmentValue()
				s2 = segTwo.GetSegmentValue()
			}
			if s1 != s2 {
				var result int
				if s1 > s2 {
					result = 1
				} else {
					result = -1
				}
				if comp.flipSecond && compareHigh != comp.compareHighValue {
					result = -result
				}
				return result
			}
		}
		compareHigh = !compareHigh
		if compareHigh == comp.compareHighValue {
			break
		}
	}
	return 0
}

func (comp valueComparator) compareParts(oneSeries, twoSeries AddressDivisionSeries) int {
	sizeResult := int(oneSeries.GetBitCount() - twoSeries.GetBitCount())
	if sizeResult != 0 {
		return sizeResult
	}
	result := compareDivBitCounts(oneSeries, twoSeries)
	if result != 0 {
		return result
	}
	compareHigh := comp.compareHighValue
	var one, two *AddressDivisionGrouping
	if o, ok := oneSeries.(StandardDivGroupingType); ok {
		if t, ok := twoSeries.(StandardDivGroupingType); ok {
			one = o.ToDivGrouping()
			two = t.ToDivGrouping()
		}
	}
	oneSeriesByteCount := oneSeries.GetByteCount()
	twoSeriesByteCount := twoSeries.GetByteCount()
	oneBytes := make([]byte, oneSeriesByteCount)
	twoBytes := make([]byte, twoSeriesByteCount)
	for {
		var oneByteCount, twoByteCount, oneByteIndex, twoByteIndex, oneIndex, twoIndex int
		var oneBitCount, twoBitCount, oneTotalBitCount, twoTotalBitCount BitCount
		var oneValue, twoValue uint64
		for oneIndex < oneSeries.GetDivisionCount() || twoIndex < twoSeries.GetDivisionCount() {
			if one != nil {
				if oneBitCount == 0 {
					oneCombo := one.GetDivision(oneIndex)
					oneIndex++
					oneBitCount = oneCombo.GetBitCount()
					if compareHigh {
						oneValue = oneCombo.GetUpperDivisionValue()
					} else {
						oneValue = oneCombo.GetDivisionValue()
					}
				}
				if twoBitCount == 0 {
					twoCombo := two.GetDivision(twoIndex)
					twoIndex++
					twoBitCount = twoCombo.GetBitCount()
					if compareHigh {
						twoValue = twoCombo.GetUpperDivisionValue()
					} else {
						twoValue = twoCombo.GetDivisionValue()
					}
				}
			} else {
				if oneBitCount == 0 {
					if oneByteCount == 0 {
						oneCombo := oneSeries.GetGenericDivision(oneIndex)
						oneIndex++
						if compareHigh {
							oneBytes = oneCombo.CopyUpperBytes(oneBytes)
						} else {
							oneBytes = oneCombo.CopyBytes(oneBytes)
						}
						oneTotalBitCount = oneCombo.GetBitCount()
						oneByteCount = oneCombo.GetByteCount()
						oneByteIndex = 0
					}
					//put some or all of the bytes into a long
					count := 8
					oneValue = 0
					if count < oneByteCount {
						oneBitCount = BitCount(count) << 3
						oneTotalBitCount -= oneBitCount
						oneByteCount -= count
						for count > 0 {
							count--

							oneValue = (oneValue << 8) | uint64(oneBytes[oneByteIndex])
							oneByteIndex++
						}
					} else {
						shortCount := oneByteCount - 1
						lastBitsCount := oneTotalBitCount - (BitCount(shortCount) << 3)
						for shortCount > 0 {
							shortCount--
							oneValue = (oneValue << 8) | uint64(oneBytes[oneByteIndex])
							oneByteIndex++
						}
						oneValue = (oneValue << uint64(lastBitsCount)) | uint64(oneBytes[oneByteIndex]>>uint64(8-lastBitsCount))
						oneByteIndex++
						oneBitCount = oneTotalBitCount
						oneTotalBitCount = 0
						oneByteCount = 0
					}
				}
				if twoBitCount == 0 {
					if twoByteCount == 0 {
						twoCombo := twoSeries.GetGenericDivision(twoIndex)
						twoIndex++
						if compareHigh {
							twoBytes = twoCombo.CopyUpperBytes(twoBytes)
						} else {
							twoBytes = twoCombo.CopyBytes(twoBytes)
						}
						twoTotalBitCount = twoCombo.GetBitCount()
						twoByteCount = twoCombo.GetByteCount()
						twoByteIndex = 0
					}
					//put some or all of the bytes into a long
					count := 8
					twoValue = 0
					if count < twoByteCount {
						twoBitCount = BitCount(count) << 3
						twoTotalBitCount -= twoBitCount
						twoByteCount -= count
						for count > 0 {
							count--

							twoValue = (twoValue << 8) | uint64(twoBytes[twoByteIndex])
							twoByteIndex++
						}
					} else {
						shortCount := twoByteCount - 1
						lastBitsCount := twoTotalBitCount - (BitCount(shortCount) << 3)
						for shortCount > 0 {
							shortCount--

							twoValue = (twoValue << 8) | uint64(twoBytes[twoByteIndex])
							twoByteIndex++
						}
						twoValue = (twoValue << uint(lastBitsCount)) | uint64(twoBytes[twoByteIndex]>>uint(8-lastBitsCount))
						twoByteIndex++
						twoBitCount = twoTotalBitCount
						twoTotalBitCount = 0
						twoByteCount = 0
					}
				}
			}
			oneResultValue := oneValue
			twoResultValue := twoValue
			if twoBitCount == oneBitCount {
				//no adjustment required, compare the values straight up
				oneBitCount = 0
				twoBitCount = 0
			} else {
				diffBits := twoBitCount - oneBitCount
				if diffBits > 0 {
					twoResultValue >>= uint(diffBits)
					twoValue &= ^(^uint64(0) << uint(diffBits))
					twoBitCount = diffBits
					oneBitCount = 0
				} else {
					diffBits = -diffBits
					oneResultValue >>= uint(diffBits)
					oneValue &= ^(^uint64(0) << uint(diffBits))
					oneBitCount = diffBits
					twoBitCount = 0
				}
			}
			if oneResultValue != twoResultValue {
				if comp.flipSecond && compareHigh != comp.compareHighValue {
					if oneResultValue > twoResultValue {
						return -1
					}
					return 1
				}
				if oneResultValue > twoResultValue {
					return 1
				}
				return -1
			}
		}
		compareHigh = !compareHigh
		if compareHigh == comp.compareHighValue {
			break
		}
	}
	return 0
}

func (comp valueComparator) compareSegValues(oneUpper, oneLower, twoUpper, twoLower SegInt) int {
	if comp.compareHighValue {
		if oneUpper == twoUpper {
			if oneLower == twoLower {
				return 0
			} else if oneLower > twoLower {
				if !comp.flipSecond {
					return 1
				}
			}
		} else if oneUpper > twoUpper {
			return 1
		}
	} else {
		if oneLower == twoLower {
			if oneUpper == twoUpper {
				return 0
			} else if oneUpper > twoUpper {
				if !comp.flipSecond {
					return 1
				}
			}
		} else if oneLower > twoLower {
			return 1
		}
	}
	return -1
}

func (comp valueComparator) compareValues(oneUpper, oneLower, twoUpper, twoLower uint64) int {
	if comp.compareHighValue {
		if oneUpper == twoUpper {
			if oneLower == twoLower {
				return 0
			} else if oneLower > twoLower {
				if !comp.flipSecond {
					return 1
				}
			}
		} else if oneUpper > twoUpper {
			return 1
		}
	} else {
		if oneLower == twoLower {
			if oneUpper == twoUpper {
				return 0
			} else if oneUpper > twoUpper {
				if !comp.flipSecond {
					return 1
				}
			}
		} else if oneLower > twoLower {
			return 1
		}
	}
	return -1
}

func (comp valueComparator) compareLargeValues(oneUpper, oneLower, twoUpper, twoLower *big.Int) int {
	var result int
	if comp.compareHighValue {
		result = oneUpper.CmpAbs(twoUpper)
		if result == 0 {
			result = oneLower.CmpAbs(twoLower)
			if comp.flipSecond {
				result = -result
			}
		}
	} else {
		result = oneLower.CmpAbs(twoLower)
		if result == 0 {
			result = oneUpper.CmpAbs(twoUpper)
			if comp.flipSecond {
				result = -result
			}
		}
	}
	return result
}

type countComparator struct{}

func (comp countComparator) compareSectionParts(one, two *AddressSection) int {
	result := int(one.GetBitCount() - two.GetBitCount())
	if result == 0 {
		result = compareSectionCount(one, two)
		if result == 0 {
			result = comp.compareEqualSizedSections(one, two)
		}
	}
	return result
}

func (comp countComparator) compareEqualSizedSections(one, two *AddressSection) int {
	segCount := one.GetSegmentCount()
	for i := 0; i < segCount; i++ {
		segOne := one.GetSegment(i)
		segTwo := two.GetSegment(i)
		oneUpper := segOne.GetUpperSegmentValue()
		twoUpper := segTwo.GetUpperSegmentValue()
		oneLower := segOne.GetSegmentValue()
		twoLower := segTwo.GetSegmentValue()
		result := comp.compareSegValues(oneUpper, oneLower, twoUpper, twoLower)
		if result != 0 {
			return result
		}
	}
	return 0
}

func (comp countComparator) compareParts(one, two AddressDivisionSeries) int {
	result := int(one.GetBitCount() - two.GetBitCount())
	if result == 0 {
		result = compareCount(one, two)
		if result == 0 {
			result = comp.compareDivisionGroupings(one, two)
		}
	}
	return result
}

func (comp countComparator) compareDivisionGroupings(oneSeries, twoSeries AddressDivisionSeries) int {
	var one, two *AddressDivisionGrouping
	if o, ok := oneSeries.(StandardDivGroupingType); ok {
		if t, ok := twoSeries.(StandardDivGroupingType); ok {
			one = o.ToDivGrouping()
			two = t.ToDivGrouping()
		}
	}
	result := compareDivBitCounts(oneSeries, twoSeries)
	if result != 0 {
		return result
	}

	oneSeriesByteCount := oneSeries.GetByteCount()
	twoSeriesByteCount := twoSeries.GetByteCount()

	oneUpperBytes := make([]byte, oneSeriesByteCount)
	oneLowerBytes := make([]byte, oneSeriesByteCount)
	twoUpperBytes := make([]byte, twoSeriesByteCount)
	twoLowerBytes := make([]byte, twoSeriesByteCount)

	var oneByteCount, twoByteCount, oneByteIndex, twoByteIndex, oneIndex, twoIndex int
	var oneBitCount, twoBitCount, oneTotalBitCount, twoTotalBitCount BitCount
	var oneUpper, oneLower, twoUpper, twoLower uint64
	for oneIndex < oneSeries.GetDivisionCount() || twoIndex < twoSeries.GetDivisionCount() {
		if one != nil {
			if oneBitCount == 0 {
				oneCombo := one.getDivision(oneIndex)
				oneIndex++
				oneBitCount = oneCombo.GetBitCount()
				oneUpper = oneCombo.GetUpperDivisionValue()
				oneLower = oneCombo.GetDivisionValue()
			}
			if twoBitCount == 0 {
				twoCombo := two.getDivision(twoIndex)
				twoIndex++
				twoBitCount = twoCombo.GetBitCount()
				twoUpper = twoCombo.GetUpperDivisionValue()
				twoLower = twoCombo.GetDivisionValue()
			}
		} else {
			if oneBitCount == 0 {
				if oneByteCount == 0 {
					oneCombo := oneSeries.GetGenericDivision(oneIndex)
					oneIndex++
					oneUpperBytes = oneCombo.CopyUpperBytes(oneUpperBytes)
					oneLowerBytes = oneCombo.CopyBytes(oneLowerBytes)
					oneTotalBitCount = oneCombo.GetBitCount()
					oneByteCount = oneCombo.GetByteCount()
					oneByteIndex = 0
				}
				//put some or all of the bytes into a uint64
				count := 8
				oneUpper = 0
				oneLower = 0
				if count < oneByteCount {
					oneBitCount = BitCount(count << 3)
					oneTotalBitCount -= oneBitCount
					oneByteCount -= count
					for count > 0 {
						count--
						upperByte := oneUpperBytes[oneByteIndex]
						lowerByte := oneLowerBytes[oneByteIndex]
						oneByteIndex++
						oneUpper = (oneUpper << 1) | uint64(upperByte)
						oneLower = (oneLower << 1) | uint64(lowerByte)
					}
				} else {
					shortCount := oneByteCount - 1
					lastBitsCount := oneTotalBitCount - (BitCount(shortCount) << 3)
					for shortCount > 0 {
						shortCount--
						upperByte := oneUpperBytes[oneByteIndex]
						lowerByte := oneLowerBytes[oneByteIndex]
						oneByteIndex++
						oneUpper = (oneUpper << 8) | uint64(upperByte)
						oneLower = (oneLower << 8) | uint64(lowerByte)
					}
					upperByte := oneUpperBytes[oneByteIndex]
					lowerByte := oneLowerBytes[oneByteIndex]
					oneByteIndex++
					oneUpper = (oneUpper << uint(lastBitsCount)) | uint64(upperByte>>uint(8-lastBitsCount))
					oneLower = (oneLower << uint(lastBitsCount)) | uint64(lowerByte>>uint(8-lastBitsCount))
					oneBitCount = oneTotalBitCount
					oneTotalBitCount = 0
					oneByteCount = 0
				}
			}
			if twoBitCount == 0 {
				if twoByteCount == 0 {
					twoCombo := twoSeries.GetGenericDivision(twoIndex)
					twoIndex++
					twoUpperBytes = twoCombo.CopyUpperBytes(twoUpperBytes)
					twoLowerBytes = twoCombo.CopyBytes(twoLowerBytes)
					twoTotalBitCount = twoCombo.GetBitCount()
					twoByteCount = twoCombo.GetByteCount()
					twoByteIndex = 0
				}
				//put some or all of the bytes into a long
				count := 8
				twoUpper = 0
				twoLower = 0
				if count < twoByteCount {
					twoBitCount = BitCount(count << 3)
					twoTotalBitCount -= twoBitCount
					twoByteCount -= count
					for count > 0 {
						count--
						upperByte := twoUpperBytes[twoByteIndex]
						lowerByte := twoLowerBytes[twoByteIndex]
						twoByteIndex++
						twoUpper = (twoUpper << 8) | uint64(upperByte)
						twoLower = (twoLower << 8) | uint64(lowerByte)
					}
				} else {
					shortCount := twoByteCount - 1
					lastBitsCount := twoTotalBitCount - (BitCount(shortCount) << 3)
					for shortCount > 0 {
						shortCount--
						upperByte := twoUpperBytes[twoByteIndex]
						lowerByte := twoLowerBytes[twoByteIndex]
						twoByteIndex++
						twoUpper = (twoUpper << 8) | uint64(upperByte)
						twoLower = (twoLower << 8) | uint64(lowerByte)
					}
					upperByte := twoUpperBytes[twoByteIndex]
					lowerByte := twoLowerBytes[twoByteIndex]
					twoByteIndex++
					twoUpper = (twoUpper << uint(lastBitsCount)) | uint64(upperByte>>uint(8-lastBitsCount))
					twoLower = (twoLower << uint(lastBitsCount)) | uint64(lowerByte>>uint(8-lastBitsCount))
					twoBitCount = twoTotalBitCount
					twoTotalBitCount = 0
					twoByteCount = 0
				}
			}
		}
		oneResultUpper := oneUpper
		oneResultLower := oneLower
		twoResultUpper := twoUpper
		twoResultLower := twoLower
		if twoBitCount == oneBitCount {
			//no adjustment required, compare the values straight up
			oneBitCount = 0
			twoBitCount = 0
		} else {
			diffBits := twoBitCount - oneBitCount
			if diffBits > 0 {
				twoResultUpper >>= uint(diffBits) //look at the high bits only (we are comparing left to right, high to low)
				twoResultLower >>= uint(diffBits)
				mask := ^(^uint64(0) << uint(diffBits))
				twoUpper &= mask
				twoLower &= mask
				twoBitCount = diffBits
				oneBitCount = 0
			} else {
				diffBits = -diffBits
				oneResultUpper >>= uint(diffBits)
				oneResultLower >>= uint(diffBits)
				mask := ^(^uint64(0) << uint(diffBits))
				oneUpper &= mask
				oneLower &= mask
				oneBitCount = diffBits
				twoBitCount = 0
			}
		}
		result := comp.compareValues(oneResultUpper, oneResultLower, twoResultUpper, twoResultLower)
		if result != 0 {
			return result
		}
	}
	return 0
}

func (countComparator) compareSegValues(oneUpper, oneLower, twoUpper, twoLower SegInt) int {
	size1 := oneUpper - oneLower
	size2 := twoUpper - twoLower
	if size1 == size2 {
		//the size of the range is the same, so just compare either upper or lower values
		if oneLower == twoLower {
			return 0
		} else if oneLower > twoLower {
			return 1
		}
	} else if size1 > size2 {
		return 1
	}
	return -1
}

func (countComparator) compareValues(oneUpper, oneLower, twoUpper, twoLower uint64) int {
	size1 := oneUpper - oneLower
	size2 := twoUpper - twoLower
	if size1 == size2 {
		//the size of the range is the same, so just compare either upper or lower values
		if oneLower == twoLower {
			return 0
		} else if oneLower > twoLower {
			return 1
		}
	} else if size1 > size2 {
		return 1
	}
	return -1
}

func (countComparator) compareLargeValues(oneUpper, oneLower, twoUpper, twoLower *big.Int) (result int) {
	oneUpper.Sub(oneUpper, oneLower)
	twoUpper.Sub(twoUpper, twoLower)
	result = oneUpper.Cmp(twoUpper)
	if result == 0 {
		//the size of the range is the same, so just compare either upper or lower values
		result = oneLower.Cmp(twoLower)
	}
	return
}

func compareDivBitCounts(oneSeries, twoSeries AddressDivisionSeries) int {
	//when this is called we know the two series have the same bit-size, we want to check that the divisions
	//also have the same bit size (which of course also implies that there are the same number of divisions)
	count := oneSeries.GetDivisionCount()
	result := count - twoSeries.GetDivisionCount()
	if result == 0 {
		for i := 0; i < count; i++ {
			result = int(oneSeries.GetGenericDivision(i).GetBitCount() - twoSeries.GetGenericDivision(i).GetBitCount())
			if result != 0 {
				break
			}
		}
	}
	return result
}

func compareSectionCount(one, two *AddressSection) int {
	return one.CompareSize(two)
}

func compareCount(one, two AddressDivisionSeries) int {
	if addrSeries1, ok := one.(AddressType); ok {
		if addrSeries2, ok := two.(AddressType); ok {
			return addrSeries1.CompareSize(addrSeries2)
		}
	} else if grouping1, ok := one.(StandardDivGroupingType); ok {
		if grouping2, ok := two.(StandardDivGroupingType); ok {
			return grouping1.CompareSize(grouping2)
		}
	}
	return one.GetCount().Cmp(two.GetCount())
}
