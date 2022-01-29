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

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
	"github.com/seancfoley/ipaddress-go/ipaddr/addrstr"
)

func createIPSection(segments []*AddressDivision, prefixLength PrefixLen, addrType addrType) *IPAddressSection {
	sect := &IPAddressSection{
		ipAddressSectionInternal{
			addressSectionInternal{
				addressDivisionGroupingInternal{
					addressDivisionGroupingBase: addressDivisionGroupingBase{
						divisions:    standardDivArray{segments},
						addrType:     addrType,
						cache:        &valueCache{},
						prefixLength: prefixLength,
					},
				},
			},
		},
	}
	assignStringCache(&sect.addressDivisionGroupingBase, addrType)
	return sect
}

func createIPSectionFromSegs(isIPv4 bool, orig []*IPAddressSegment, prefLen PrefixLen) (result *IPAddressSection) {
	segProvider := func(index int) *IPAddressSegment {
		return orig[index]
	}
	var divs []*AddressDivision
	var newPref PrefixLen
	var isMultiple bool
	if isIPv4 {
		divs, newPref, isMultiple = createDivisionsFromSegs(
			segProvider,
			len(orig),
			ipv4BitsToSegmentBitshift,
			IPv4BitsPerSegment,
			IPv4BytesPerSegment,
			IPv4MaxValuePerSegment,
			zeroIPv4Seg.ToIP(),
			zeroIPv4SegZeroPrefix.ToIP(),
			zeroIPv4SegPrefixBlock.ToIP(),
			prefLen)
		result = createIPv4Section(divs).ToIP()
	} else {
		divs, newPref, isMultiple = createDivisionsFromSegs(
			func(index int) *IPAddressSegment {
				return orig[index]
			},
			len(orig),
			ipv6BitsToSegmentBitshift,
			IPv6BitsPerSegment,
			IPv6BytesPerSegment,
			IPv6MaxValuePerSegment,
			zeroIPv6Seg.ToIP(),
			zeroIPv6SegZeroPrefix.ToIP(),
			zeroIPv6SegPrefixBlock.ToIP(),
			prefLen)
		result = createIPv6Section(divs).ToIP()
	}
	result.prefixLength = newPref
	result.isMult = isMultiple
	return result
}

func createInitializedIPSection(segments []*AddressDivision, prefixLength PrefixLen, addrType addrType) *IPAddressSection {
	result := createIPSection(segments, prefixLength, addrType)
	result.initMultAndPrefLen() // assigns isMult and checks prefix length
	return result
}

func deriveIPAddressSection(from *IPAddressSection, segments []*AddressDivision) (res *IPAddressSection) {
	res = createIPSection(segments, nil, from.getAddrType())
	res.initMultAndPrefLen()
	return
}

func deriveIPAddressSectionPrefLen(from *IPAddressSection, segments []*AddressDivision, prefixLength PrefixLen) (res *IPAddressSection) {
	res = createIPSection(segments, prefixLength, from.getAddrType())
	res.initMultAndPrefLen()
	return
}

//
//
//
//
type ipAddressSectionInternal struct {
	addressSectionInternal
}

func (section *ipAddressSectionInternal) GetSegment(index int) *IPAddressSegment {
	return section.getDivision(index).ToIP()
}

func (section *ipAddressSectionInternal) GetIPVersion() IPVersion {
	addrType := section.getAddrType()
	if addrType.isIPv4() {
		return IPv4
	} else if addrType.isIPv6() {
		return IPv6
	}
	return IndeterminateIPVersion
}

func (section *ipAddressSectionInternal) getNetworkPrefixLen() PrefixLen {
	return section.prefixLength
}

func (section *ipAddressSectionInternal) GetNetworkPrefixLen() PrefixLen {
	return section.getNetworkPrefixLen().copy()
}

// GetBlockMaskPrefixLen returns the prefix length if this address section is equivalent to the mask for a CIDR prefix block.
// Otherwise, it returns null.
// A CIDR network mask is an address with all 1s in the network section and then all 0s in the host section.
// A CIDR host mask is an address with all 0s in the network section and then all 1s in the host section.
// The prefix length is the length of the network section.
//
// Also, keep in mind that the prefix length returned by this method is not equivalent to the prefix length of this object,
// indicating the network and host section of this address.
// The prefix length returned here indicates the whether the value of this address can be used as a mask for the network and host
// section of any other address.  Therefore the two values can be different values, or one can be null while the other is not.
//
// This method applies only to the lower value of the range if this section represents multiple values.
func (section *ipAddressSectionInternal) GetBlockMaskPrefixLen(network bool) PrefixLen {
	cache := section.cache
	if cache == nil {
		return nil // no prefix
	}
	cachedMaskLens := cache.cachedMaskLens
	if cachedMaskLens == nil {
		networkMaskLen, hostMaskLen := section.checkForPrefixMask()
		res := &maskLenSetting{networkMaskLen, hostMaskLen}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&cache.cachedMaskLens))
		atomic.StorePointer(dataLoc, unsafe.Pointer(res))
	}
	if network {
		return cache.cachedMaskLens.networkMaskLen
	}
	return cache.cachedMaskLens.hostMaskLen
}

func (section *ipAddressSectionInternal) checkForPrefixMask() (networkMaskLen, hostMaskLen PrefixLen) {
	count := section.GetSegmentCount()
	if count == 0 {
		return
	}
	firstSeg := section.GetSegment(0)
	checkingNetworkFront, checkingHostFront := true, true
	var checkingNetworkBack, checkingHostBack bool
	var prefixedSeg int
	prefixedSegPrefixLen := BitCount(0)
	maxVal := firstSeg.GetMaxValue()
	for i := 0; i < count; i++ {
		seg := section.GetSegment(i)
		val := seg.GetSegmentValue()
		if val == 0 {
			if checkingNetworkFront {
				prefixedSeg = i
				checkingNetworkFront, checkingNetworkBack = false, true
			} else if !checkingHostFront && !checkingNetworkBack {
				return
			}
			checkingHostBack = false
		} else if val == maxVal {
			if checkingHostFront {
				prefixedSeg = i
				checkingHostFront, checkingHostBack = false, true
			} else if !checkingHostBack && !checkingNetworkFront {
				return
			}
			checkingNetworkBack = false
		} else {
			segNetworkMaskLen, segHostMaskLen := seg.checkForPrefixMask()
			if segNetworkMaskLen != nil {
				if checkingNetworkFront {
					prefixedSegPrefixLen = segNetworkMaskLen.bitCount()
					checkingNetworkBack = true
					checkingHostBack = false
					prefixedSeg = i
				} else {
					return
				}
			} else if segHostMaskLen != nil {
				if checkingHostFront {
					prefixedSegPrefixLen = segHostMaskLen.bitCount()
					checkingHostBack = true
					checkingNetworkBack = false
					prefixedSeg = i
				} else {
					return
				}
			} else {
				return
			}
			checkingNetworkFront, checkingHostFront = false, false
		}
	}
	if checkingNetworkFront {
		// all ones
		networkMaskLen = cacheBitCount(section.GetBitCount())
		hostMaskLen = cacheBitCount(0)
	} else if checkingHostFront {
		// all zeros
		hostMaskLen = cacheBitCount(section.GetBitCount())
		networkMaskLen = cacheBitCount(0)
	} else if checkingNetworkBack {
		// ending in zeros, network mask
		networkMaskLen = getNetworkPrefixLen(firstSeg.GetBitCount(), prefixedSegPrefixLen, prefixedSeg)
	} else if checkingHostBack {
		// ending in ones, host mask
		hostMaskLen = getNetworkPrefixLen(firstSeg.GetBitCount(), prefixedSegPrefixLen, prefixedSeg)
	}
	return
}

func (section *ipAddressSectionInternal) IncludesZeroHost() bool {
	networkPrefixLength := section.getPrefixLen()
	return networkPrefixLength != nil && section.IncludesZeroHostLen(networkPrefixLength.bitCount())
}

func (section *ipAddressSectionInternal) IncludesZeroHostLen(networkPrefixLength BitCount) bool {
	networkPrefixLength = checkSubnet(section.toIPAddressSection(), networkPrefixLength)
	bitsPerSegment := section.GetBitsPerSegment()
	bytesPerSegment := section.GetBytesPerSegment()
	prefixedSegmentIndex := getHostSegmentIndex(networkPrefixLength, bytesPerSegment, bitsPerSegment)
	divCount := section.GetSegmentCount()
	for i := prefixedSegmentIndex; i < divCount; i++ {
		div := section.GetSegment(i)
		segmentPrefixLength := getPrefixedSegmentPrefixLength(bitsPerSegment, networkPrefixLength, i)
		if segmentPrefixLength != nil {
			mask := div.GetSegmentHostMask(segmentPrefixLength.bitCount())
			if (mask & div.GetSegmentValue()) != 0 {
				return false
			}
			for i++; i < divCount; i++ {
				div = section.GetSegment(i)
				if !div.includesZero() {
					return false
				}
			}
		}
	}
	return true
}

func (section *ipAddressSectionInternal) IncludesMaxHost() bool {
	networkPrefixLength := section.getPrefixLen()
	return networkPrefixLength != nil && section.IncludesMaxHostLen(networkPrefixLength.bitCount())
}

func (section *ipAddressSectionInternal) IncludesMaxHostLen(networkPrefixLength BitCount) bool {
	networkPrefixLength = checkSubnet(section.toIPAddressSection(), networkPrefixLength)
	bitsPerSegment := section.GetBitsPerSegment()
	bytesPerSegment := section.GetBytesPerSegment()
	prefixedSegmentIndex := getHostSegmentIndex(networkPrefixLength, bytesPerSegment, bitsPerSegment)
	divCount := section.GetSegmentCount()
	for i := prefixedSegmentIndex; i < divCount; i++ {
		div := section.GetSegment(i)
		segmentPrefixLength := getPrefixedSegmentPrefixLength(bitsPerSegment, networkPrefixLength, i)
		if segmentPrefixLength != nil {
			mask := div.GetSegmentHostMask(segmentPrefixLength.bitCount())
			if (mask & div.getUpperSegmentValue()) != mask {
				return false
			}
			for i++; i < divCount; i++ {
				div = section.GetSegment(i)
				if !div.includesMax() {
					return false
				}
			}
		}
	}
	return true
}

func (section *ipAddressSectionInternal) toZeroHost(boundariesOnly bool) (res *IPAddressSection, err addrerr.IncompatibleAddressError) {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return section.toIPAddressSection(), nil
	}
	var prefLen BitCount
	if section.isPrefixed() {
		prefLen = section.getPrefixLen().bitCount()
	}
	if section.IsZeroHostLen(prefLen) {
		return section.toIPAddressSection(), nil
	}
	if section.IncludesZeroHost() && section.IsSingleNetwork() {
		res = section.getLower().ToIP() //cached
		return
	}
	if !section.isPrefixed() {
		mask := section.addrType.getIPNetwork().GetPrefixedNetworkMask(0)
		res = mask.GetSubSection(0, segmentCount)
		return
	}
	return section.createZeroHost(prefLen, boundariesOnly)
	//return sect.ToIP(), err
}

// boundariesOnly: whether we care if the masking works for all values in a range.
// For instance, 1.2.3.2-4/31 cannot be zero-hosted, because applyng to the boundaries results in 1.2.3.2-4/31,
// and that includes 1.2.3.3/31 which does not have host of zero.
// So in that case, we'd normally haveaddrerr.IncompatibleAddressError.  boundariesOnly as true avoids the exception,
// if we are really just interested in getting the zero-host boundaries,
// and we don't care about the remaining values in-between.
func (section *ipAddressSectionInternal) createZeroHost(prefLen BitCount, boundariesOnly bool) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	mask := section.addrType.getIPNetwork().GetNetworkMask(prefLen)
	return section.getSubnetSegments(
		getNetworkSegmentIndex(prefLen, section.GetBytesPerSegment(), section.GetBitsPerSegment()),
		cacheBitCount(prefLen),
		!boundariesOnly, //verifyMask
		section.getDivision,
		func(i int) SegInt { return mask.GetSegment(i).GetSegmentValue() })
}

func (section *ipAddressSectionInternal) toZeroHostLen(prefixLength BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	var minIndex int
	if section.isPrefixed() {
		existingPrefLen := section.getNetworkPrefixLen().bitCount()
		if prefixLength == existingPrefLen {
			return section.toZeroHost(false)
		}
		if prefixLength < existingPrefLen {
			minIndex = getNetworkSegmentIndex(prefixLength, section.GetBytesPerSegment(), section.GetBitsPerSegment())
		} else {
			minIndex = getNetworkSegmentIndex(existingPrefLen, section.GetBytesPerSegment(), section.GetBitsPerSegment())
		}
	} else {
		minIndex = getNetworkSegmentIndex(prefixLength, section.GetBytesPerSegment(), section.GetBitsPerSegment())
	}
	mask := section.addrType.getIPNetwork().GetNetworkMask(prefixLength)
	return section.getSubnetSegments(
		minIndex,
		nil,
		true,
		section.getDivision,
		func(i int) SegInt { return mask.GetSegment(i).GetSegmentValue() })
}

func (section *ipAddressSectionInternal) toZeroNetwork() *IPAddressSection {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return section.toIPAddressSection()
	}
	if !section.isPrefixed() {
		mask := section.addrType.getIPNetwork().GetHostMask(section.GetBitCount())
		return mask.GetSubSection(0, segmentCount)
	}
	return section.createZeroNetwork()
}

func (section *ipAddressSectionInternal) createZeroNetwork() *IPAddressSection {
	prefixLength := section.getNetworkPrefixLen() // we know it is prefixed here so no panic on the derefence
	mask := section.addrType.getIPNetwork().GetHostMask(prefixLength.bitCount())
	res, _ := section.getSubnetSegments(
		0,
		prefixLength,
		false,
		section.getDivision,
		func(i int) SegInt { return mask.GetSegment(i).GetSegmentValue() })
	return res
}

func (section *ipAddressSectionInternal) toMaxHost() (res *IPAddressSection, err addrerr.IncompatibleAddressError) {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return section.toIPAddressSection(), nil
	}
	if !section.isPrefixed() {
		mask := section.addrType.getIPNetwork().GetPrefixedHostMask(0)
		res = mask.GetSubSection(0, segmentCount)
		return
	}
	if section.IsMaxHostLen(section.getPrefixLen().bitCount()) {
		return section.toIPAddressSection(), nil
	}
	if section.IncludesMaxHost() && section.IsSingleNetwork() {
		return section.getUpper().ToIP(), nil // cached
	}
	return section.createMaxHost()
}

func (section *ipAddressSectionInternal) createMaxHost() (*IPAddressSection, addrerr.IncompatibleAddressError) {
	prefixLength := section.getNetworkPrefixLen() // we know it is prefixed here so no panic on the derefence
	mask := section.addrType.getIPNetwork().GetHostMask(prefixLength.bitCount())
	return section.getOredSegments(
		prefixLength,
		true,
		section.getDivision,
		func(i int) SegInt { return mask.GetSegment(i).GetSegmentValue() })
}

func (section *ipAddressSectionInternal) toMaxHostLen(prefixLength BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	if section.isPrefixed() && prefixLength == section.getNetworkPrefixLen().bitCount() {
		return section.toMaxHost()
	}
	mask := section.addrType.getIPNetwork().GetHostMask(prefixLength)
	return section.getOredSegments(
		nil,
		true,
		section.getDivision,
		func(i int) SegInt { return mask.GetSegment(i).GetSegmentValue() })
}

// IsSingleNetwork returns whether the network section of the address, the prefix, consists of a single value
func (section *ipAddressSectionInternal) IsSingleNetwork() bool {
	networkPrefixLength := section.getNetworkPrefixLen()
	if networkPrefixLength == nil {
		return !section.isMultiple()
	}
	prefLen := networkPrefixLength.bitCount()
	if prefLen >= section.GetBitCount() {
		return !section.isMultiple()
	}
	bitsPerSegment := section.GetBitsPerSegment()
	prefixedSegmentIndex := getNetworkSegmentIndex(prefLen, section.GetBytesPerSegment(), bitsPerSegment)
	if prefixedSegmentIndex < 0 {
		return true
	}
	for i := 0; i < prefixedSegmentIndex; i++ {
		if section.getDivision(i).isMultiple() {
			return false
		}
	}
	div := section.GetSegment(prefixedSegmentIndex)
	divPrefLen := getPrefixedSegmentPrefixLength(bitsPerSegment, prefLen, prefixedSegmentIndex)
	shift := bitsPerSegment - divPrefLen.bitCount()
	return (div.GetSegmentValue() >> uint(shift)) == (div.GetUpperSegmentValue() >> uint(shift))
}

// IsMaxHost returns whether this section has a prefix length and if so,
// whether the host section is the maximum value for this section or all sections in this set of address sections.
// If the host section is zero length (there are no host bits at all), returns false.
func (section *ipAddressSectionInternal) IsMaxHost() bool {
	if !section.isPrefixed() {
		return false
	}
	return section.IsMaxHostLen(section.getNetworkPrefixLen().bitCount())
}

// IsMaxHostLen returns whether the host is the max value for the given prefix length for this section.
// If this section already has a prefix length, then that prefix length is ignored.
// If the host section is zero length (there are no host bits at all), returns true.
func (section *ipAddressSectionInternal) IsMaxHostLen(prefLen BitCount) bool {
	divCount := section.GetSegmentCount()
	if divCount == 0 {
		return true
	} else if prefLen < 0 {
		prefLen = 0
	}
	bytesPerSegment := section.GetBytesPerSegment()
	bitsPerSegment := section.GetBitsPerSegment()
	// Note: 1.2.3.4/32 has a max host
	prefixedSegmentIndex := getHostSegmentIndex(prefLen, bytesPerSegment, bitsPerSegment)
	if prefixedSegmentIndex < divCount {
		segmentPrefixLength := getPrefixedSegmentPrefixLength(bitsPerSegment, prefLen, prefixedSegmentIndex)
		i := prefixedSegmentIndex
		div := section.GetSegment(i)
		mask := div.GetSegmentHostMask(segmentPrefixLength.bitCount())
		if div.isMultiple() || (mask&div.getSegmentValue()) != mask {
			return false
		}
		i++
		for ; i < divCount; i++ {
			div = section.GetSegment(i)
			if !div.IsMax() {
				return false
			}
		}
	}
	return true
}

// IsZeroHost returns whether this section has a prefix length and if so,
// whether the host section is zero for this section or all sections in this set of address sections.
func (section *ipAddressSectionInternal) IsZeroHost() bool {
	if !section.isPrefixed() {
		return false
	}
	return section.IsZeroHostLen(section.getNetworkPrefixLen().bitCount())
}

// IsZeroHostLen returns whether the host is zero for the given prefix length for this section or all sections in this set of address sections.
// If this section already has a prefix length, then that prefix length is ignored.
// If the host section is zero length (there are no host bits at all), returns true.
func (section *ipAddressSectionInternal) IsZeroHostLen(prefLen BitCount) bool {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return true
	} else if prefLen < 0 {
		prefLen = 0
	}
	bitsPerSegment := section.GetBitsPerSegment()
	// Note: 1.2.3.4/32 has a zero host
	prefixedSegmentIndex := getHostSegmentIndex(prefLen, section.GetBytesPerSegment(), bitsPerSegment)
	if prefixedSegmentIndex < segmentCount {
		segmentPrefixLength := getPrefixedSegmentPrefixLength(bitsPerSegment, prefLen, prefixedSegmentIndex)
		//if segmentPrefixLength != nil {
		i := prefixedSegmentIndex
		div := section.GetSegment(i)
		if div.isMultiple() || (div.GetSegmentHostMask(segmentPrefixLength.bitCount())&div.getSegmentValue()) != 0 {
			return false
		}
		for i++; i < segmentCount; i++ {
			div := section.GetSegment(i)
			if !div.IsZero() {
				return false
			}
		}
		//}
	}
	return true
}

func (section *ipAddressSectionInternal) adjustPrefixLength(adjustment BitCount, withZeros bool) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	if adjustment == 0 && section.isPrefixed() {
		return section.toIPAddressSection(), nil
	}
	prefix := section.getAdjustedPrefix(adjustment, true, true)
	sec, err := section.setPrefixLength(prefix, withZeros)
	return sec.ToIP(), err
}

func (section *ipAddressSectionInternal) adjustPrefixLen(adjustment BitCount) *IPAddressSection {
	// no zeroing
	res, _ := section.adjustPrefixLength(adjustment, false)
	return res
}

func (section *ipAddressSectionInternal) adjustPrefixLenZeroed(adjustment BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	return section.adjustPrefixLength(adjustment, true)
}

func (section *ipAddressSectionInternal) withoutPrefixLen() *IPAddressSection {
	if !section.isPrefixed() {
		return section.toIPAddressSection()
	}
	if section.hasNoDivisions() {
		return createIPSection(section.getDivisionsInternal(), nil, section.getAddrType())
	}
	existingPrefixLength := section.getPrefixLen().bitCount()
	maxVal := section.GetMaxSegmentValue()
	var startIndex int
	if existingPrefixLength > 0 {
		bitsPerSegment := section.GetBitsPerSegment()
		bytesPerSegment := section.GetBytesPerSegment()
		startIndex = getNetworkSegmentIndex(existingPrefixLength, bytesPerSegment, bitsPerSegment)
	}
	res, _ := section.getSubnetSegments(
		startIndex,
		nil,
		false,
		func(i int) *AddressDivision {
			return section.getDivision(i)
		},
		func(i int) SegInt {
			return maxVal
		},
	)
	return res
}

func (section *ipAddressSectionInternal) checkSectionCount(other *IPAddressSection) addrerr.SizeMismatchError {
	if other.GetSegmentCount() < section.GetSegmentCount() {
		return &sizeMismatchError{incompatibleAddressError{addressError{key: "ipaddress.error.sizeMismatch"}}}
	}
	return nil
}

// error can be addrerr.IncompatibleAddressError or addrerr.SizeMismatchError
func (section *ipAddressSectionInternal) mask(msk *IPAddressSection, retainPrefix bool) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	if err := section.checkSectionCount(msk); err != nil {
		return nil, err
	}
	var prefLen PrefixLen
	if retainPrefix {
		prefLen = section.getPrefixLen()
	}
	return section.getSubnetSegments(
		0,
		prefLen,
		true,
		section.getDivision,
		func(i int) SegInt { return msk.GetSegment(i).GetSegmentValue() })
}

// error can be addrerr.IncompatibleAddressError or addrerr.SizeMismatchError
func (section *ipAddressSectionInternal) bitwiseOr(msk *IPAddressSection, retainPrefix bool) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	if err := section.checkSectionCount(msk); err != nil {
		return nil, err
	}
	var prefLen PrefixLen
	if retainPrefix {
		prefLen = section.getPrefixLen()
	}
	return section.getOredSegments(
		prefLen,
		true,
		section.getDivision,
		func(i int) SegInt { return msk.GetSegment(i).GetSegmentValue() })
}

func (section *ipAddressSectionInternal) matchesWithMask(other *IPAddressSection, mask *IPAddressSection) bool {
	if err := section.checkSectionCount(other); err != nil {
		return false
	} else if err := section.checkSectionCount(mask); err != nil {
		return false
	}
	divCount := section.GetSegmentCount()
	for i := 0; i < divCount; i++ {
		seg := section.GetSegment(i)
		maskSegment := mask.GetSegment(i)
		otherSegment := other.GetSegment(i)
		if !seg.MatchesValsWithMask(
			otherSegment.getSegmentValue(),
			otherSegment.getUpperSegmentValue(),
			maskSegment.getSegmentValue()) {
			return false
		}
	}
	return true
}

func (section *ipAddressSectionInternal) intersect(other *IPAddressSection) (res *IPAddressSection, err addrerr.SizeMismatchError) {

	//check if they are comparable section.  We only check segment count, we do not care about start index.
	err = section.checkSectionCount(other)
	if err != nil {
		return
	}

	//larger prefix length should prevail?    hmmmmm... I would say that is true, choose the larger prefix
	pref := section.getNetworkPrefixLen()
	otherPref := other.getNetworkPrefixLen()
	if pref != nil {
		if otherPref != nil {
			if otherPref.bitCount() > pref.bitCount() {
				pref = otherPref
			}
		} else {
			pref = nil
		}
	}

	if other.Contains(section.toIPAddressSection()) {
		if pref.Equal(section.getNetworkPrefixLen()) {
			res = section.toIPAddressSection()
			return
		}
	} else if !section.isMultiple() {
		return
	}
	if section.contains(other) {
		if pref.Equal(other.getNetworkPrefixLen()) {
			res = other.toIPAddressSection()
			return
		}
	} else if !other.isMultiple() {
		return
	}

	segCount := section.GetSegmentCount()
	for i := 0; i < segCount; i++ {
		seg := section.GetSegment(i)
		otherSeg := other.GetSegment(i)
		lower := seg.GetSegmentValue()
		higher := seg.getUpperSegmentValue()
		otherLower := otherSeg.GetSegmentValue()
		otherHigher := otherSeg.getUpperSegmentValue()
		if otherLower > higher || lower > otherHigher {
			//no overlap in this segment means no overlap at all
			return
		}
	}

	// all segments have overlap
	segs := createSegmentArray(segCount)
	for i := 0; i < segCount; i++ {
		seg := section.GetSegment(i)
		otherSeg := other.GetSegment(i)
		segPref := getSegmentPrefixLength(seg.getBitCount(), pref, i)
		if seg.Contains(otherSeg) {
			if segPref.Equal(otherSeg.GetSegmentPrefixLen()) {
				segs[i] = otherSeg.ToDiv()
				continue
			}
		}
		if otherSeg.Contains(seg) {
			if segPref.Equal(seg.GetSegmentPrefixLen()) {
				segs[i] = seg.ToDiv()
				continue
			}
		}
		lower := seg.GetSegmentValue()
		higher := seg.getUpperSegmentValue()
		otherLower := otherSeg.GetSegmentValue()
		otherHigher := otherSeg.getUpperSegmentValue()
		if otherLower > lower {
			lower = otherLower
		}
		if otherHigher < higher {
			higher = otherHigher
		}
		segs[i] = createAddressDivision(seg.deriveNewMultiSeg(lower, higher, segPref))
	}
	res = deriveIPAddressSectionPrefLen(section.toIPAddressSection(), segs, pref)
	return
}

func (section *ipAddressSectionInternal) subtract(other *IPAddressSection) (res []*IPAddressSection, err addrerr.SizeMismatchError) {
	//check if they are comparable section
	err = section.checkSectionCount(other)
	if err != nil {
		return
	}

	if !section.isMultiple() {
		if other.Contains(section.toIPAddressSection()) {
			return
		}
		res = []*IPAddressSection{section.toIPAddressSection()}
		return
	}
	//getDifference: same as removing the intersection
	//   section you confirm there is an intersection in each segment.
	// Then you remove each intersection, one at a time, leaving the other segments the same, since only one segment needs to differ.
	// To prevent adding the same section twice, use only the intersection (ie the relative complement of the diff)
	// of segments already handled and not the whole segment.

	// For example: 0-3.0-3.2.4 subtracting 1-4.1-3.2.4, the intersection is 1-3.1-3.2.4
	// The diff of the section segment is just 0, giving 0.0-3.2.4 (subtract the section segment, leave the others the same)
	// The diff of the second segment is also 0, but for the section segment we use the intersection since we handled the section already, giving 1-3.0.2.4
	// 	(take the intersection of the section segment, subtract the second segment, leave remaining segments the same)

	segCount := section.GetSegmentCount()
	for i := 0; i < segCount; i++ {
		seg := section.GetSegment(i)
		otherSeg := other.GetSegment(i)
		lower := seg.GetSegmentValue()
		higher := seg.getUpperSegmentValue()
		otherLower := otherSeg.GetSegmentValue()
		otherHigher := otherSeg.getUpperSegmentValue()
		if otherLower > higher || lower > otherHigher {
			//no overlap in this segment means no overlap at all
			res = []*IPAddressSection{section.toIPAddressSection()}
			return
		}
	}

	intersections := createSegmentArray(segCount)
	sections := make([]*IPAddressSection, 0, segCount<<1)
	for i := 0; i < segCount; i++ {
		seg := section.GetSegment(i)
		otherSeg := other.GetSegment(i)
		lower := seg.GetSegmentValue()
		higher := seg.getUpperSegmentValue()
		otherLower := otherSeg.GetSegmentValue()
		otherHigher := otherSeg.getUpperSegmentValue()
		if lower >= otherLower {
			if higher <= otherHigher {
				//this segment is contained in the other
				if seg.isPrefixed() {
					intersections[i] = createAddressDivision(seg.deriveNewMultiSeg(lower, higher, nil)) //addrCreator.createSegment(lower, higher, null);
				} else {
					intersections[i] = seg.ToDiv()
				}
				continue
			}
			//otherLower <= lower <= otherHigher < higher
			intersections[i] = createAddressDivision(seg.deriveNewMultiSeg(lower, otherHigher, nil))
			section := section.createDiffSection(seg, otherHigher+1, higher, i, intersections)
			sections = append(sections, section)
		} else {
			//lower < otherLower <= otherHigher
			section := section.createDiffSection(seg, lower, otherLower-1, i, intersections)
			sections = append(sections, section)
			if higher <= otherHigher {
				intersections[i] = createAddressDivision(seg.deriveNewMultiSeg(otherLower, higher, nil))
			} else {
				//lower < otherLower <= otherHigher < higher
				intersections[i] = createAddressDivision(seg.deriveNewMultiSeg(otherLower, otherHigher, nil))
				section = section.createDiffSection(seg, otherHigher+1, higher, i, intersections)
				sections = append(sections, section)
			}
		}
	}
	if len(sections) == 0 {
		return
	}

	//apply the prefix to the sections
	//for each section, we figure out what each prefix length should be
	if section.isPrefixed() {
		thisPrefix := section.getNetworkPrefixLen().bitCount()
		for i := 0; i < len(sections); i++ {
			section := sections[i]
			bitCount := section.GetBitCount()
			totalPrefix := bitCount
			for j := section.GetSegmentCount() - 1; j >= 0; j-- {
				seg := section.GetSegment(j)
				segBitCount := seg.GetBitCount()
				segPrefix := seg.GetMinPrefixLenForBlock()
				if segPrefix == segBitCount {
					break
				} else {
					totalPrefix -= segBitCount
					if segPrefix != 0 {
						totalPrefix += segPrefix
						break
					}
				}
			}
			if totalPrefix != bitCount {
				if totalPrefix < thisPrefix {
					totalPrefix = thisPrefix
				}
				section = section.SetPrefixLen(totalPrefix)
				sections[i] = section
			}
		}
	}
	res = sections
	return
}

func (section *ipAddressSectionInternal) createDiffSection(
	seg *IPAddressSegment,
	lower,
	upper SegInt,
	diffIndex int,
	intersectingValues []*AddressDivision) *IPAddressSection {
	segCount := section.GetSegmentCount()
	segments := createSegmentArray(segCount)
	for j := 0; j < diffIndex; j++ {
		segments[j] = intersectingValues[j]
	}
	diff := createAddressDivision(seg.deriveNewMultiSeg(lower, upper, nil))
	segments[diffIndex] = diff
	for j := diffIndex + 1; j < segCount; j++ {
		segments[j] = section.getDivision(j)
	}
	return deriveIPAddressSection(section.toIPAddressSection(), segments)
}

func (section *ipAddressSectionInternal) spanWithPrefixBlocks() []ExtendedIPSegmentSeries {
	wrapped := wrapIPSection(section.toIPAddressSection())
	if section.IsSequential() {
		if section.IsSinglePrefixBlock() {
			return []ExtendedIPSegmentSeries{wrapped}
		}
		return getSpanningPrefixBlocks(wrapped, wrapped)
	}
	return spanWithPrefixBlocks(wrapped)
}

func (section *ipAddressSectionInternal) spanWithSequentialBlocks() []ExtendedIPSegmentSeries {
	wrapped := wrapIPSection(section.toIPAddressSection())
	if section.IsSequential() {
		return []ExtendedIPSegmentSeries{wrapped}
	}
	return spanWithSequentialBlocks(wrapped)
}

func (section *ipAddressSectionInternal) coverSeriesWithPrefixBlock() ExtendedIPSegmentSeries {
	if section.IsSinglePrefixBlock() {
		return wrapIPSection(section.toIPAddressSection())
	}
	return coverWithPrefixBlock(
		wrapIPSection(section.getLower().ToIP()),
		wrapIPSection(section.getUpper().ToIP()))
}

func (section *ipAddressSectionInternal) coverWithPrefixBlock() *IPAddressSection {
	if section.IsSinglePrefixBlock() {
		return section.toIPAddressSection()
	}
	res := coverWithPrefixBlock(
		wrapIPSection(section.getLower().ToIP()),
		wrapIPSection(section.getUpper().ToIP()))
	return res.(WrappedIPAddressSection).IPAddressSection
}

func (section *ipAddressSectionInternal) coverWithPrefixBlockTo(other *IPAddressSection) (*IPAddressSection, addrerr.SizeMismatchError) {
	if err := section.checkSectionCount(other); err != nil {
		return nil, err
	}
	res := getCoveringPrefixBlock(
		wrapIPSection(section.toIPAddressSection()),
		wrapIPSection(other))
	return res.(WrappedIPAddressSection).IPAddressSection, nil
}

func (section *ipAddressSectionInternal) getNetworkSection() *IPAddressSection {
	var prefLen BitCount
	if section.isPrefixed() {
		prefLen = section.getPrefixLen().bitCount()
	} else {
		prefLen = section.GetBitCount()
	}
	return section.getNetworkSectionLen(prefLen)
}

func (section *ipAddressSectionInternal) getNetworkSectionLen(networkPrefixLength BitCount) *IPAddressSection {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return section.toIPAddressSection()
	}
	networkPrefixLength = checkBitCount(networkPrefixLength, section.GetBitCount())
	bitsPerSegment := section.GetBitsPerSegment()
	prefixedSegmentIndex := getNetworkSegmentIndex(networkPrefixLength, section.GetBytesPerSegment(), bitsPerSegment)
	var newSegments []*AddressDivision
	if prefixedSegmentIndex >= 0 {
		segPrefLength := getPrefixedSegmentPrefixLength(bitsPerSegment, networkPrefixLength, prefixedSegmentIndex) // prefixedSegmentIndex of -1 already handled
		lastSeg := section.GetSegment(prefixedSegmentIndex)
		prefBits := segPrefLength.bitCount()
		mask := ^SegInt(0) << uint(bitsPerSegment-prefBits)
		lower, upper := lastSeg.getSegmentValue()&mask, lastSeg.getUpperSegmentValue()|^mask
		networkSegmentCount := prefixedSegmentIndex + 1
		if networkSegmentCount == segmentCount && segsSame(segPrefLength, lastSeg.GetSegmentPrefixLen(), lower, lastSeg.getSegmentValue(), upper, lastSeg.getUpperSegmentValue()) {
			// the segment count and prefixed segment matches
			return section.toIPAddressSection()
		}
		newSegments = createSegmentArray(networkSegmentCount)
		//if networkSegmentCount > 0 {
		section.copySubSegmentsToSlice(0, prefixedSegmentIndex, newSegments)
		newSegments[prefixedSegmentIndex] = createAddressDivision(lastSeg.deriveNewMultiSeg(lower, upper, segPrefLength))
		//}
	} else {
		newSegments = createSegmentArray(0)
	}
	return deriveIPAddressSectionPrefLen(section.toIPAddressSection(), newSegments, cacheBitCount(networkPrefixLength))
}

func (section *ipAddressSectionInternal) getHostSection() *IPAddressSection {
	var prefLen BitCount
	if section.isPrefixed() {
		prefLen = section.getPrefixLen().bitCount()
	}
	return section.getHostSectionLen(prefLen)
}

func (section *ipAddressSectionInternal) getHostSectionLen(networkPrefixLength BitCount) *IPAddressSection {
	segmentCount := section.GetSegmentCount()
	if segmentCount == 0 {
		return section.toIPAddressSection()
	}
	networkPrefixLength = checkBitCount(networkPrefixLength, section.GetBitCount())
	bitsPerSegment := section.GetBitsPerSegment()
	prefixedSegmentIndex := getHostSegmentIndex(networkPrefixLength, section.GetBytesPerSegment(), bitsPerSegment)
	var prefLen PrefixLen
	var newSegments []*AddressDivision
	if prefixedSegmentIndex < segmentCount {
		firstSeg := section.GetSegment(prefixedSegmentIndex)
		segPrefLength := getPrefixedSegmentPrefixLength(bitsPerSegment, networkPrefixLength, prefixedSegmentIndex)
		prefLen = segPrefLength
		prefBits := segPrefLength.bitCount()
		//mask the boundary segment
		mask := ^(^SegInt(0) << uint(bitsPerSegment-prefBits))
		divLower := uint64(firstSeg.getDivisionValue())
		divUpper := uint64(firstSeg.getUpperDivisionValue())
		divMask := uint64(mask)
		maxVal := uint64(^SegInt(0))
		masker := MaskRange(divLower, divUpper, divMask, maxVal)
		lower, upper := masker.GetMaskedLower(divLower, divMask), masker.GetMaskedUpper(divUpper, divMask)
		segLower, segUpper := SegInt(lower), SegInt(upper)
		if prefixedSegmentIndex == 0 && segsSame(segPrefLength, firstSeg.GetSegmentPrefixLen(), segLower, firstSeg.getSegmentValue(), segUpper, firstSeg.getUpperSegmentValue()) {
			// the segment count and prefixed segment matches
			return section.toIPAddressSection()
		}
		hostSegmentCount := segmentCount - prefixedSegmentIndex
		newSegments = createSegmentArray(hostSegmentCount)
		section.copySubSegmentsToSlice(prefixedSegmentIndex+1, prefixedSegmentIndex+hostSegmentCount, newSegments[1:])
		newSegments[0] = createAddressDivision(firstSeg.deriveNewMultiSeg(segLower, segUpper, segPrefLength))
	} else {
		prefLen = cacheBitCount(0)
		newSegments = createSegmentArray(0)
	}
	addrType := section.getAddrType()
	if !section.isMultiple() {
		return createIPSection(newSegments, prefLen, addrType)
	}
	return createInitializedIPSection(newSegments, prefLen, addrType)
}

func (section *ipAddressSectionInternal) getSubnetSegments( // called by methods to adjust/remove/set prefix length, masking methods, zero host and zero network methods
	startIndex int,
	networkPrefixLength PrefixLen,
	verifyMask bool,
	segProducer func(int) *AddressDivision,
	segmentMaskProducer func(int) SegInt,
) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	newSect, err := section.addressSectionInternal.getSubnetSegments(startIndex, networkPrefixLength, verifyMask, segProducer, segmentMaskProducer)
	return newSect.ToIP(), err
}

func (section *ipAddressSectionInternal) getOredSegments(
	networkPrefixLength PrefixLen,
	verifyMask bool,
	segProducer func(int) *AddressDivision,
	segmentMaskProducer func(int) SegInt) (res *IPAddressSection, err addrerr.IncompatibleAddressError) {
	networkPrefixLength = checkPrefLen(networkPrefixLength, section.GetBitCount())
	bitsPerSegment := section.GetBitsPerSegment()
	count := section.GetSegmentCount()
	for i := 0; i < count; i++ {
		segmentPrefixLength := getSegmentPrefixLength(bitsPerSegment, networkPrefixLength, i)
		seg := segProducer(i)
		//note that the mask can represent a range (for example a CIDR mask),
		//but we use the lowest value (maskSegment.value) in the range when masking (ie we discard the range)
		maskValue := segmentMaskProducer(i)
		origValue, origUpperValue := seg.getSegmentValue(), seg.getUpperSegmentValue()
		value, upperValue := origValue, origUpperValue
		if verifyMask {
			mask64 := uint64(maskValue)
			val64 := uint64(value)
			upperVal64 := uint64(upperValue)
			masker := bitwiseOrRange(val64, upperVal64, mask64, seg.GetMaxValue())
			if !masker.IsSequential() {
				err = &incompatibleAddressError{addressError{key: "ipaddress.error.maskMismatch"}}
				return
			}
			value = SegInt(masker.GetOredLower(val64, mask64))
			upperValue = SegInt(masker.GetOredUpper(upperVal64, mask64))
		} else {
			value |= maskValue
			upperValue |= maskValue
		}
		if !segsSame(segmentPrefixLength, seg.getDivisionPrefixLength(), value, origValue, upperValue, origUpperValue) {
			newSegments := createSegmentArray(count)
			section.copySubSegmentsToSlice(0, i, newSegments)
			newSegments[i] = createAddressDivision(seg.deriveNewMultiSeg(value, upperValue, segmentPrefixLength))
			for i++; i < count; i++ {
				segmentPrefixLength = getSegmentPrefixLength(bitsPerSegment, networkPrefixLength, i)
				seg = segProducer(i)
				maskValue = segmentMaskProducer(i)
				value = seg.getSegmentValue()
				upperValue = seg.getUpperSegmentValue()
				if verifyMask {
					mask64 := uint64(maskValue)
					val64 := uint64(value)
					upperVal64 := uint64(upperValue)
					masker := bitwiseOrRange(val64, upperVal64, mask64, seg.GetMaxValue())
					if !masker.IsSequential() {
						err = &incompatibleAddressError{addressError{key: "ipaddress.error.maskMismatch"}}
						return
					}
					value = SegInt(masker.GetOredLower(val64, mask64))
					upperValue = SegInt(masker.GetOredUpper(upperVal64, mask64))

				} else {
					value |= maskValue
					upperValue |= maskValue
				}
				if !segsSame(segmentPrefixLength, seg.getDivisionPrefixLength(), value, origValue, upperValue, origUpperValue) {
					newSegments[i] = createAddressDivision(seg.deriveNewMultiSeg(value, upperValue, segmentPrefixLength))
				} else {
					newSegments[i] = seg
				}
			}
			res = deriveIPAddressSectionPrefLen(section.toIPAddressSection(), newSegments, networkPrefixLength)
			return
		}
	}
	res = section.toIPAddressSection()
	return
}

func (section *ipAddressSectionInternal) getNetwork() IPAddressNetwork {
	if addrType := section.getAddrType(); addrType.isIPv4() {
		return ipv4Network
	} else if addrType.isIPv6() {
		return ipv6Network
	}
	return nil
}

func (section *ipAddressSectionInternal) getNetworkMask(network IPAddressNetwork) *IPAddressSection {
	var prefLen BitCount
	if section.isPrefixed() {
		prefLen = section.getNetworkPrefixLen().bitCount()
	} else {
		prefLen = section.GetBitCount()
	}
	return network.GetNetworkMask(prefLen).GetSubSection(0, section.GetSegmentCount())
}

func (section *ipAddressSectionInternal) getHostMask(network IPAddressNetwork) *IPAddressSection {
	var prefLen BitCount
	if section.isPrefixed() {
		prefLen = section.getNetworkPrefixLen().bitCount()
	}
	return network.GetNetworkMask(prefLen).GetSubSection(0, section.GetSegmentCount())
}

func (section *ipAddressSectionInternal) insert(index int, other *IPAddressSection, segmentToBitsShift uint) *IPAddressSection {
	return section.replaceLen(index, index, other, 0, other.GetSegmentCount(), segmentToBitsShift)
}

// Replaces segments starting from startIndex and ending before endIndex with the segments starting at replacementStartIndex and
//ending before replacementEndIndex from the replacement section
func (section *ipAddressSectionInternal) replaceLen(
	startIndex, endIndex int, replacement *IPAddressSection, replacementStartIndex, replacementEndIndex int, segmentToBitsShift uint) *IPAddressSection {

	segmentCount := section.GetSegmentCount()
	startIndex, endIndex, replacementStartIndex, replacementEndIndex =
		adjustIndices(startIndex, endIndex, segmentCount, replacementStartIndex, replacementEndIndex, replacement.GetSegmentCount())
	replacedCount := endIndex - startIndex
	replacementCount := replacementEndIndex - replacementStartIndex
	thizz := section.toAddressSection()
	if replacementCount == 0 && replacedCount == 0 { //keep in mind for ipvx, empty sections cannot have prefix lengths
		return section.toIPAddressSection()
	} else if segmentCount == replacedCount { //keep in mind for ipvx, empty sections cannot have prefix lengths
		return replacement
	}
	var newPrefixLen PrefixLen
	prefixLength := section.getPrefixLen()
	startBits := BitCount(startIndex << segmentToBitsShift)
	if prefixLength != nil && prefixLength.bitCount() <= startBits {
		newPrefixLen = prefixLength
		replacement = replacement.SetPrefixLen(0)
	} else {
		replacementEndBits := BitCount(replacementEndIndex << segmentToBitsShift)
		replacementPrefLen := replacement.getPrefixLen()
		endIndexBits := BitCount(endIndex << segmentToBitsShift)
		if replacementPrefLen != nil && replacementPrefLen.bitCount() <= replacementEndBits {
			var replacementPrefixLen BitCount
			replacementStartBits := BitCount(replacementStartIndex << segmentToBitsShift)
			replacementPrefLenIsZero := replacementPrefLen.bitCount() <= replacementStartBits
			if !replacementPrefLenIsZero {
				replacementPrefixLen = replacementPrefLen.bitCount() - replacementStartBits
			}
			newPrefixLen = cacheBitCount(startBits + replacementPrefixLen)
			if endIndex < segmentCount && (prefixLength == nil || prefixLength.bitCount() > endIndexBits) {
				if replacedCount > 0 || replacementPrefLenIsZero {
					thizz = section.setPrefixLen(endIndexBits)
				} else {
					// this covers the case of a:5:6:7:8 is getting b:c:d/47 at index 1 to 1
					// We need "a" to have no prefix, and "5" to get prefix len 0
					// But setting "5" to have prefix len 0 gives "a" the prefix len 16
					// This is not a problem if any segments are getting replaced or the replacement segments have prefix length 0
					//
					// we move the non-replaced host segments from the end of this to the end of the replacement segments
					// and we also remove the prefix length from this
					additionalSegs := segmentCount - endIndex
					thizz = section.getSubSection(0, startIndex)
					//return section.ReplaceLen(index, index, other, 0, other.GetSegmentCount())

					replacement = replacement.insert(
						replacementEndIndex, section.getSubSection(endIndex, segmentCount).ToIP(), segmentToBitsShift)
					replacementEndIndex += additionalSegs
				}
			}
		} else if prefixLength != nil {
			replacementBits := BitCount(replacementCount << segmentToBitsShift)
			var endPrefixBits BitCount
			if prefixLength.bitCount() > endIndexBits {
				endPrefixBits = prefixLength.bitCount() - endIndexBits
			}
			newPrefixLen = cacheBitCount(startBits + replacementBits + endPrefixBits)
		} // else newPrefixLen is nil
	}
	return thizz.replace(startIndex, endIndex, replacement.ToSectionBase(),
		replacementStartIndex, replacementEndIndex, newPrefixLen).ToIP()
}

func (section *ipAddressSectionInternal) toNormalizedWildcardString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToNormalizedWildcardString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToNormalizedWildcardString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toCanonicalWildcardString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToCanonicalWildcardString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToCanonicalWildcardString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toSegmentedBinaryString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToSegmentedBinaryString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToSegmentedBinaryString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toSQLWildcardString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToSQLWildcardString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToSQLWildcardString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toFullString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToFullString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToFullString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toReverseDNSString() (string, addrerr.IncompatibleAddressError) {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToReverseDNSString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToReverseDNSString()
	}
	return nilSection(), nil
}

func (section *ipAddressSectionInternal) toPrefixLenString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToPrefixLenString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToPrefixLenString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toSubnetString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToNormalizedWildcardString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToPrefixLenString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toCompressedWildcardString() string {
	if sect := section.toIPv4AddressSection(); sect != nil {
		return sect.ToCompressedWildcardString()
	} else if sect := section.toIPv6AddressSection(); sect != nil {
		return sect.ToCompressedWildcardString()
	}
	return nilSection()
}

func (section *ipAddressSectionInternal) toCustomString(stringOptions addrstr.IPStringOptions) string {
	return toNormalizedIPZonedString(stringOptions, section.toIPAddressSection(), NoZone)
}

func (section *ipAddressSectionInternal) toCustomZonedString(stringOptions addrstr.IPStringOptions, zone Zone) string {
	return toNormalizedIPZonedString(stringOptions, section.toIPAddressSection(), zone)
}

func (section *ipAddressSectionInternal) Wrap() WrappedIPAddressSection {
	return wrapIPSection(section.toIPAddressSection())
}

func (section *ipAddressSectionInternal) toIPAddressSection() *IPAddressSection {
	return (*IPAddressSection)(unsafe.Pointer(section))
}

//// only needed for godoc / pkgsite

func (section *ipAddressSectionInternal) GetBitCount() BitCount {
	return section.addressSectionInternal.GetBitCount()
}

func (section *ipAddressSectionInternal) GetByteCount() int {
	return section.addressSectionInternal.GetByteCount()
}

//IPv6v4, Div,  Not needed Addr because of GetGenericSegment
//func (grouping *addressDivisionGroupingBase) GetGenericDivision(index int) DivisionType {
//
//IPv6v4, Div, Not needed Addr
//func (grouping *addressDivisionGroupingBase) GetDivisionCount() int {

func (section *ipAddressSectionInternal) IsZero() bool {
	return section.addressSectionInternal.IsZero()
}

func (section *ipAddressSectionInternal) IncludesZero() bool {
	return section.addressSectionInternal.IncludesZero()
}

func (section *ipAddressSectionInternal) IsMax() bool {
	return section.addressSectionInternal.IsMax()
}

func (section *ipAddressSectionInternal) IncludesMax() bool {
	return section.addressSectionInternal.IncludesMax()
}

func (section *ipAddressSectionInternal) IsFullRange() bool {
	return section.addressSectionInternal.IsFullRange()
}

func (section *ipAddressSectionInternal) GetSequentialBlockIndex() int {
	return section.addressSectionInternal.GetSequentialBlockIndex()
}

func (section *ipAddressSectionInternal) GetSequentialBlockCount() *big.Int {
	return section.addressSectionInternal.GetSequentialBlockCount()
}

func (section *ipAddressSectionInternal) ContainsPrefixBlock(prefixLen BitCount) bool {
	return section.addressSectionInternal.ContainsPrefixBlock(prefixLen)
}

func (section *ipAddressSectionInternal) ContainsSinglePrefixBlock(prefixLen BitCount) bool {
	return section.addressSectionInternal.ContainsSinglePrefixBlock(prefixLen)
}

func (section *ipAddressSectionInternal) IsPrefixBlock() bool {
	return section.addressSectionInternal.IsPrefixBlock()
}

func (section *ipAddressSectionInternal) IsSinglePrefixBlock() bool {
	return section.addressSectionInternal.IsSinglePrefixBlock()
}

func (section *ipAddressSectionInternal) GetMinPrefixLenForBlock() BitCount {
	return section.addressSectionInternal.GetMinPrefixLenForBlock()
}

func (section *ipAddressSectionInternal) GetPrefixLenForSingleBlock() PrefixLen {
	return section.addressSectionInternal.GetPrefixLenForSingleBlock()
}

func (section *ipAddressSectionInternal) GetValue() *big.Int {
	return section.addressSectionInternal.GetValue()
}

func (section *ipAddressSectionInternal) GetUpperValue() *big.Int {
	return section.addressSectionInternal.GetUpperValue()
}

func (section *ipAddressSectionInternal) Bytes() []byte {
	return section.addressSectionInternal.Bytes()
}

func (section *ipAddressSectionInternal) UpperBytes() []byte {
	return section.addressSectionInternal.UpperBytes()
}

func (section *ipAddressSectionInternal) CopyBytes(bytes []byte) []byte {
	return section.addressSectionInternal.CopyBytes(bytes)
}

func (section *ipAddressSectionInternal) CopyUpperBytes(bytes []byte) []byte {
	return section.addressSectionInternal.CopyUpperBytes(bytes)
}

func (section *ipAddressSectionInternal) IsSequential() bool {
	return section.addressSectionInternal.IsSequential()
}

func (section *ipAddressSectionInternal) GetBitsPerSegment() BitCount {
	return section.addressSectionInternal.GetBitsPerSegment()
}

func (section *ipAddressSectionInternal) GetBytesPerSegment() int {
	return section.addressSectionInternal.GetBytesPerSegment()
}

func (section *ipAddressSectionInternal) GetGenericSegment(index int) AddressSegmentType {
	return section.addressSectionInternal.GetGenericSegment(index)
}

func (section *ipAddressSectionInternal) GetSegmentCount() int {
	return section.addressSectionInternal.GetSegmentCount()
}

func (section *ipAddressSectionInternal) GetMaxSegmentValue() SegInt {
	return section.addressSectionInternal.GetMaxSegmentValue()
}

func (section *ipAddressSectionInternal) TestBit(n BitCount) bool {
	return section.addressSectionInternal.TestBit(n)
}

func (section *ipAddressSectionInternal) IsOneBit(prefixBitIndex BitCount) bool {
	return section.addressSectionInternal.IsOneBit(prefixBitIndex)
}

func (section *ipAddressSectionInternal) PrefixEqual(other AddressSectionType) bool {
	return section.addressSectionInternal.PrefixEqual(other)
}

func (section *ipAddressSectionInternal) PrefixContains(other AddressSectionType) bool {
	return section.addressSectionInternal.PrefixContains(other)
}

//// end needed for godoc / pkgsite

//
//
//
// An IPAddress section has segments, which are divisions of equal length and size
type IPAddressSection struct {
	ipAddressSectionInternal
}

func (section *IPAddressSection) Contains(other AddressSectionType) bool {
	if section == nil {
		return other == nil || other.ToSectionBase() == nil
	}
	return section.contains(other)
}

func (section *IPAddressSection) Equal(other AddressSectionType) bool {
	if section == nil {
		return other == nil || other.ToSectionBase() == nil
	}
	return section.equal(other)
}

func (section *IPAddressSection) Compare(item AddressItem) int {
	return CountComparator.Compare(section, item)
}

func (section *IPAddressSection) CompareSize(other StandardDivGroupingType) int {
	if section == nil {
		if other != nil && other.ToDivGrouping() != nil {
			// we have size 0, other has size >= 1
			return -1
		}
		return 0
	}
	return section.compareSize(other)
}

func (section *IPAddressSection) GetCount() *big.Int {
	if section == nil {
		return bigZero()
	} else if sect := section.ToIPv4(); sect != nil {
		return sect.GetCount()
	} else if sect := section.ToIPv6(); sect != nil {
		return sect.GetCount()
	}
	return section.addressDivisionGroupingBase.getCount()
}

func (section *IPAddressSection) IsMultiple() bool {
	return section != nil && section.isMultiple()
}

func (section *IPAddressSection) IsPrefixed() bool {
	return section != nil && section.isPrefixed()
}

func (section *IPAddressSection) GetPrefixCount() *big.Int {
	if sect := section.ToIPv4(); sect != nil {
		return sect.GetPrefixCount()
	} else if sect := section.ToIPv6(); sect != nil {
		return sect.GetPrefixCount()
	}
	return section.addressDivisionGroupingBase.GetPrefixCount()
}

func (section *IPAddressSection) GetPrefixCountLen(prefixLen BitCount) *big.Int {
	if sect := section.ToIPv4(); sect != nil {
		return sect.GetPrefixCountLen(prefixLen)
	} else if sect := section.ToIPv6(); sect != nil {
		return sect.GetPrefixCountLen(prefixLen)
	}
	return section.addressDivisionGroupingBase.GetPrefixCountLen(prefixLen)
}

// GetBlockCount returns the count of values in the initial (higher) count of divisions.
func (section *IPAddressSection) GetBlockCount(segmentCount int) *big.Int {
	if sect := section.ToIPv4(); sect != nil {
		return sect.GetBlockCount(segmentCount)
	} else if sect := section.ToIPv6(); sect != nil {
		return sect.GetBlockCount(segmentCount)
	}
	return section.addressDivisionGroupingBase.GetBlockCount(segmentCount)
}

func (section *IPAddressSection) IsAdaptiveZero() bool {
	return section != nil && section.matchesZeroGrouping()
}

func (section *IPAddressSection) ToDivGrouping() *AddressDivisionGrouping {
	return section.ToSectionBase().ToDivGrouping()
}

func (section *IPAddressSection) ToSectionBase() *AddressSection {
	return (*AddressSection)(unsafe.Pointer(section))
}

func (section *IPAddressSection) ToIPv6() *IPv6AddressSection {
	if section.IsIPv6() {
		return (*IPv6AddressSection)(section)
	}
	return nil
}

func (section *IPAddressSection) ToIPv4() *IPv4AddressSection {
	if section.IsIPv4() {
		return (*IPv4AddressSection)(section)
	}
	return nil
}

func (section *IPAddressSection) IsIPv4() bool { // we allow nil receivers to allow this to be called following a failed converion like ToIP()
	return section != nil && section.matchesIPv4SectionType()
}

func (section *IPAddressSection) IsIPv6() bool {
	return section != nil && section.matchesIPv6SectionType()
}

// Gets the subsection from the series starting from the given index
// The first segment is at index 0.
func (section *IPAddressSection) GetTrailingSection(index int) *IPAddressSection {
	return section.GetSubSection(index, section.GetSegmentCount())
}

// GetSubSection gets the subsection from the series starting from the given index and ending just before the give endIndex
// The first segment is at index 0.
func (section *IPAddressSection) GetSubSection(index, endIndex int) *IPAddressSection {
	return section.getSubSection(index, endIndex).ToIP()
}

func (section *IPAddressSection) GetNetworkSection() *IPAddressSection {
	return section.getNetworkSection()
}

func (section *IPAddressSection) GetNetworkSectionLen(prefLen BitCount) *IPAddressSection {
	return section.getNetworkSectionLen(prefLen)
}

func (section *IPAddressSection) GetHostSection() *IPAddressSection {
	return section.getHostSection()
}

func (section *IPAddressSection) GetHostSectionLen(prefLen BitCount) *IPAddressSection {
	return section.getHostSectionLen(prefLen)
}

func (section *IPAddressSection) GetNetworkMask() *IPAddressSection {
	return section.getNetworkMask(section.getNetwork())
}

func (section *IPAddressSection) GetHostMask() *IPAddressSection {
	return section.getHostMask(section.getNetwork())
}

// CopySubSegments copies the existing segments from the given start index until but not including the segment at the given end index,
// into the given slice, as much as can be fit into the slice, returning the number of segments copied
func (section *IPAddressSection) CopySubSegments(start, end int, segs []*IPAddressSegment) (count int) {
	return section.visitSubDivisions(start, end, func(index int, div *AddressDivision) bool { segs[index] = div.ToIP(); return false }, len(segs))
}

// CopySubSegments copies the existing segments from the given start index until but not including the segment at the given end index,
// into the given slice, as much as can be fit into the slice, returning the number of segments copied
func (section *IPAddressSection) CopySegments(segs []*IPAddressSegment) (count int) {
	return section.visitDivisions(func(index int, div *AddressDivision) bool { segs[index] = div.ToIP(); return false }, len(segs))
}

// GetSegments returns a slice with the address segments.  The returned slice is not backed by the same array as this section.
func (section *IPAddressSection) GetSegments() (res []*IPAddressSegment) {
	res = make([]*IPAddressSegment, section.GetSegmentCount())
	section.CopySegments(res)
	return
}

func (section *IPAddressSection) GetLower() *IPAddressSection {
	return section.getLower().ToIP()
}

func (section *IPAddressSection) GetUpper() *IPAddressSection {
	return section.getUpper().ToIP()
}

func (section *IPAddressSection) ToZeroHost() (res *IPAddressSection, err addrerr.IncompatibleAddressError) {
	return section.toZeroHost(false)
}

func (section *IPAddressSection) ToZeroHostLen(prefixLength BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	return section.ToZeroHostLen(prefixLength)
}

func (section *IPAddressSection) ToZeroNetwork() *IPAddressSection {
	return section.toZeroNetwork()
}

func (section *IPAddressSection) ToMaxHost() (res *IPAddressSection, err addrerr.IncompatibleAddressError) {
	return section.toMaxHost()
}

func (section *IPAddressSection) ToMaxHostLen(prefixLength BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	return section.toMaxHostLen(prefixLength)
}

func (section *IPAddressSection) WithoutPrefixLen() *IPAddressSection {
	if !section.IsPrefixed() {
		return section
	}
	return section.withoutPrefixLen()
}

func (section *IPAddressSection) SetPrefixLen(prefixLen BitCount) *IPAddressSection {
	return section.setPrefixLen(prefixLen).ToIP()
}

func (section *IPAddressSection) SetPrefixLenZeroed(prefixLen BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.setPrefixLenZeroed(prefixLen)
	return res.ToIP(), err
}

func (section *IPAddressSection) AdjustPrefixLen(prefixLen BitCount) *IPAddressSection {
	return section.adjustPrefixLen(prefixLen)
}

func (section *IPAddressSection) AdjustPrefixLenZeroed(prefixLen BitCount) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	return section.adjustPrefixLenZeroed(prefixLen)
}

func (section *IPAddressSection) ToPrefixBlock() *IPAddressSection {
	return section.toPrefixBlock().ToIP()
}

func (section *IPAddressSection) ToPrefixBlockLen(prefLen BitCount) *IPAddressSection {
	return section.toPrefixBlockLen(prefLen).ToIP()
}

func (section *IPAddressSection) AssignPrefixForSingleBlock() *IPAddressSection {
	return section.assignPrefixForSingleBlock().ToIP()
}

func (section *IPAddressSection) AssignMinPrefixForBlock() *IPAddressSection {
	return section.assignMinPrefixForBlock().ToIP()
}

func (section *IPAddressSection) ToBlock(segmentIndex int, lower, upper SegInt) *IPAddressSection {
	return section.toBlock(segmentIndex, lower, upper).ToIP()
}

func (section *IPAddressSection) Iterator() IPSectionIterator {
	if section == nil {
		return ipSectionIterator{nilSectIterator()}
	}
	return ipSectionIterator{section.sectionIterator(nil)}
}

func (section *IPAddressSection) PrefixIterator() IPSectionIterator {
	return ipSectionIterator{section.prefixIterator(false)}
}

func (section *IPAddressSection) PrefixBlockIterator() IPSectionIterator {
	return ipSectionIterator{section.prefixIterator(true)}
}

func (section *IPAddressSection) BlockIterator(segmentCount int) IPSectionIterator {
	return ipSectionIterator{section.blockIterator(segmentCount)}
}

func (section *IPAddressSection) SequentialBlockIterator() IPSectionIterator {
	return ipSectionIterator{section.sequentialBlockIterator()}
}

func (section *IPAddressSection) IncrementBoundary(increment int64) *IPAddressSection {
	return section.incrementBoundary(increment).ToIP()
}

func (section *IPAddressSection) Increment(increment int64) *IPAddressSection {
	return section.increment(increment).ToIP()
}

func (section *IPAddressSection) SpanWithPrefixBlocks() []*IPAddressSection {
	if section.IsSequential() {
		if section.IsSinglePrefixBlock() {
			return []*IPAddressSection{section}
		}
		wrapped := wrapIPSection(section)
		spanning := getSpanningPrefixBlocks(wrapped, wrapped)
		return cloneToIPSections(spanning)
	}
	wrapped := wrapIPSection(section)
	return cloneToIPSections(spanWithPrefixBlocks(wrapped))
}

func (section *IPAddressSection) SpanWithSequentialBlocks() []*IPAddressSection {
	if section.IsSequential() {
		return []*IPAddressSection{section}
	}
	wrapped := wrapIPSection(section)
	return cloneToIPSections(spanWithSequentialBlocks(wrapped))
}

func (section *IPAddressSection) CoverWithPrefixBlock() *IPAddressSection {
	return section.coverWithPrefixBlock()
}

func (section *IPAddressSection) ReverseBits(perByte bool) (*IPAddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.reverseBits(perByte)
	return res.ToIP(), err
}

func (section *IPAddressSection) ReverseBytes() (*IPAddressSection, addrerr.IncompatibleAddressError) {
	res, err := section.reverseBytes(false)
	return res.ToIP(), err
}

func (section *IPAddressSection) ReverseSegments() *IPAddressSection {
	if section.GetSegmentCount() <= 1 {
		if section.IsPrefixed() {
			return section.WithoutPrefixLen()
		}
		return section
	}
	res, _ := section.reverseSegments(
		func(i int) (*AddressSegment, addrerr.IncompatibleAddressError) {
			return section.GetSegment(i).withoutPrefixLen().ToSegmentBase(), nil
		},
	)
	return res.ToIP()
}

func (section *IPAddressSection) String() string {
	if section == nil {
		return nilString()
	}
	return section.toString()
}

func (section *IPAddressSection) ToCanonicalString() string {
	if section == nil {
		return nilString()
	}
	return section.toCanonicalString()
}

func (section *IPAddressSection) ToNormalizedString() string {
	if section == nil {
		return nilString()
	}
	return section.toNormalizedString()
}

func (section *IPAddressSection) ToCompressedString() string {
	if section == nil {
		return nilString()
	}
	return section.toCompressedString()
}

func (section *IPAddressSection) ToHexString(with0xPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toHexString(with0xPrefix)
}

func (section *IPAddressSection) ToOctalString(with0Prefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toOctalString(with0Prefix)
}

func (section *IPAddressSection) ToBinaryString(with0bPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toBinaryString(with0bPrefix)
}

func (section *IPAddressSection) ToNormalizedWildcardString() string {
	if section == nil {
		return nilString()
	}
	return section.toNormalizedWildcardString()
}

func (section *IPAddressSection) ToCanonicalWildcardString() string {
	if section == nil {
		return nilString()
	}
	return section.toCanonicalWildcardString()
}

func (section *IPAddressSection) ToSegmentedBinaryString() string {
	if section == nil {
		return nilString()
	}
	return section.toSegmentedBinaryString()
}

func (section *IPAddressSection) ToSQLWildcardString() string {
	if section == nil {
		return nilString()
	}
	return section.toSQLWildcardString()
}

func (section *IPAddressSection) ToFullString() string {
	if section == nil {
		return nilString()
	}
	return section.toFullString()
}

func (section *IPAddressSection) ToReverseDNSString() (string, addrerr.IncompatibleAddressError) {
	if section == nil {
		return nilString(), nil
	}
	return section.toReverseDNSString()
}

func (section *IPAddressSection) ToPrefixLenString() string {
	if section == nil {
		return nilString()
	}
	return section.toPrefixLenString()
}

func (section *IPAddressSection) ToSubnetString() string {
	if section == nil {
		return nilString()
	}
	return section.toSubnetString()
}

func (section *IPAddressSection) ToCompressedWildcardString() string {
	if section == nil {
		return nilString()
	}
	return section.toCompressedWildcardString()
}

func (section *IPAddressSection) ToCustomString(stringOptions addrstr.IPStringOptions) string {
	if section == nil {
		return nilString()
	}
	return section.toCustomString(stringOptions)
}

func (section *IPAddressSection) GetSegmentStrings() []string {
	if section == nil {
		return nil
	}
	return section.getSegmentStrings()
}

var (
	rangeWildcard                 = new(addrstr.WildcardsBuilder).ToWildcards()
	allWildcards                  = new(addrstr.WildcardOptionsBuilder).SetWildcardOptions(addrstr.WildcardsAll).ToOptions()
	wildcardsRangeOnlyNetworkOnly = new(addrstr.WildcardOptionsBuilder).SetWildcards(rangeWildcard).ToOptions()
	allSQLWildcards               = new(addrstr.WildcardOptionsBuilder).SetWildcardOptions(addrstr.WildcardsAll).SetWildcards(
		new(addrstr.WildcardsBuilder).SetWildcard(SegmentSqlWildcardStr).SetSingleWildcard(SegmentSqlSingleWildcardStr).ToWildcards()).ToOptions()
)

func BitsPerSegment(version IPVersion) BitCount {
	if version == IPv4 {
		return IPv4BitsPerSegment
	}
	return IPv6BitsPerSegment
}

// handles prefix block subnets, and ensures segment prefixes match the section prefix
func assignPrefix(prefixLength PrefixLen, segments []*AddressDivision, res *IPAddressSection, singleOnly, checkPrefixes bool, boundaryBits BitCount) {
	prefLen := prefixLength.bitCount()
	if prefLen < 0 {
		prefLen = 0
	} else if prefLen > boundaryBits {
		prefLen = boundaryBits
		prefixLength = cacheBitCount(boundaryBits)
	} else {
		prefixLength = cachePrefixLen(prefixLength) // use our own cache of prefix lengths so callers cannot overwrite a section's prefix length
	}
	segLen := len(segments)
	if segLen > 0 {
		var segProducer func(*AddressDivision, PrefixLen) *AddressDivision
		applyPrefixSubnet := !singleOnly && isPrefixSubnetDivs(segments, prefLen)
		if applyPrefixSubnet || checkPrefixes {
			if applyPrefixSubnet {
				segProducer = (*AddressDivision).toPrefixedNetworkDivision
			} else {
				segProducer = (*AddressDivision).toPrefixedDivision
			}
			applyPrefixToSegments(
				prefLen,
				segments,
				res.GetBitsPerSegment(),
				res.GetBytesPerSegment(),
				segProducer)
			if applyPrefixSubnet {
				res.isMult = res.isMult || res.GetSegment(segLen-1).isMultiple()
			}
		}
	}
	res.prefixLength = prefixLength
	return
}

// Starting from the first host bit according to the prefix, if the section is a sequence of zeros in both low and high values,
// followed by a sequence where low values are zero and high values are 1, then the section is a subnet prefix.
//
// Note that this includes sections where hosts are all zeros, or sections where hosts are full range of values,
// so the sequence of zeros can be empty and the sequence of where low values are zero and high values are 1 can be empty as well.
// However, if they are both empty, then this returns false, there must be at least one bit in the sequence.
func isPrefixSubnetDivs(sectionSegments []*AddressDivision, networkPrefixLength BitCount) bool {
	segmentCount := len(sectionSegments)
	if segmentCount == 0 {
		return false
	}
	seg := sectionSegments[0]
	return isPrefixSubnet(
		func(segmentIndex int) SegInt {
			return sectionSegments[segmentIndex].ToSegmentBase().GetSegmentValue()
		},
		func(segmentIndex int) SegInt {
			return sectionSegments[segmentIndex].ToSegmentBase().GetUpperSegmentValue()
		},
		segmentCount,
		seg.GetByteCount(),
		seg.GetBitCount(),
		seg.ToSegmentBase().GetMaxValue(),
		networkPrefixLength,
		zerosOnly)
}

func applyPrefixToSegments(
	sectionPrefixBits BitCount,
	segments []*AddressDivision,
	segmentBitCount BitCount,
	segmentByteCount int,
	segProducer func(*AddressDivision, PrefixLen) *AddressDivision) {
	var i int
	if sectionPrefixBits != 0 {
		i = getNetworkSegmentIndex(sectionPrefixBits, segmentByteCount, segmentBitCount)
	}
	for ; i < len(segments); i++ {
		pref := getPrefixedSegmentPrefixLength(segmentBitCount, sectionPrefixBits, i)
		if pref != nil {
			segments[i] = segProducer(segments[i], pref)
		}
	}
}

func createSegmentsUint64(
	segLen int,
	highBytes,
	lowBytes uint64,
	bytesPerSegment int,
	bitsPerSegment BitCount,
	creator addressSegmentCreator,
	assignedPrefixLength PrefixLen) []*AddressDivision {
	segmentMask := ^(^SegInt(0) << uint(bitsPerSegment))
	lowSegCount := getHostSegmentIndex(64, bytesPerSegment, bitsPerSegment)
	newSegs := make([]*AddressDivision, segLen)
	lowIndex := segLen - lowSegCount
	if lowIndex < 0 {
		lowIndex = 0
	}
	segmentIndex := segLen - 1
	bytes := lowBytes
	for {
		for {
			segmentPrefixLength := getSegmentPrefixLength(bitsPerSegment, assignedPrefixLength, segmentIndex)
			value := segmentMask & SegInt(bytes)
			seg := creator.createSegment(value, value, segmentPrefixLength)
			newSegs[segmentIndex] = seg
			segmentIndex--
			if segmentIndex < lowIndex {
				break
			}
			bytes >>= uint(bitsPerSegment)
		}
		if lowIndex == 0 {
			break
		}
		lowIndex = 0
		bytes = highBytes
	}
	return newSegs
}

func createSegments(
	lowerValueProvider,
	upperValueProvider SegmentValueProvider,
	segmentCount int,
	bitsPerSegment BitCount,
	creator addressSegmentCreator,
	prefixLength PrefixLen) (segments []*AddressDivision, isMultiple bool) {
	segments = createSegmentArray(segmentCount)
	for segmentIndex := 0; segmentIndex < segmentCount; segmentIndex++ {
		segmentPrefixLength := getSegmentPrefixLength(bitsPerSegment, prefixLength, segmentIndex)
		var value, value2 SegInt = 0, 0
		if lowerValueProvider == nil {
			value = upperValueProvider(segmentIndex)
			value2 = value
		} else {
			value = lowerValueProvider(segmentIndex)
			if upperValueProvider != nil {
				value2 = upperValueProvider(segmentIndex)
				if !isMultiple && value2 != value {
					isMultiple = true

				}
			} else {
				value2 = value
			}
		}
		seg := creator.createSegment(value, value2, segmentPrefixLength)
		segments[segmentIndex] = seg
	}
	return
}
