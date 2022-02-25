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
	"fmt"
	"github.com/seancfoley/bintree/tree"
	"math/big"
	"sync/atomic"
	"unsafe"

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
	"github.com/seancfoley/ipaddress-go/ipaddr/addrstr"
)

const (
	HexPrefix                    = "0x"
	OctalPrefix                  = "0"
	BinaryPrefix                 = "0b"
	RangeSeparator               = '-'
	RangeSeparatorStr            = "-"
	AlternativeRangeSeparator    = '\u00bb'
	AlternativeRangeSeparatorStr = "\u00bb"
	//ExtendedDigitsRangeSeparator    = '\u00bb'
	ExtendedDigitsRangeSeparatorStr = "\u00bb"

	SegmentWildcard    = '*'
	SegmentWildcardStr = "*"
	//AlternativeSegmentWildcard  = '¿'
	SegmentSqlWildcard          = '%'
	SegmentSqlWildcardStr       = "%"
	SegmentSqlSingleWildcard    = '_'
	SegmentSqlSingleWildcardStr = "_"
)

var segmentWildcardStr = SegmentWildcardStr

func createAddress(section *AddressSection, zone Zone) *Address {
	res := &Address{
		addressInternal{
			section: section,
			zone:    zone,
			cache:   &addressCache{},
		},
	}
	return res
}

// SegmentValueProvider provides values for segments.
// Values that fall outside the segment value type range will be truncated using standard golang integer type conversions https://golang.org/ref/spec#Conversions
type SegmentValueProvider func(segmentIndex int) SegInt

type AddressValueProvider interface {
	GetSegmentCount() int

	GetValues() SegmentValueProvider

	GetUpperValues() SegmentValueProvider
}

type addrsCache struct {
	lower, upper *Address
}

type IdentifierStr struct {
	idStr HostIdentifierString // MACAddressString or IPAddressString or HostName
}

type addressCache struct {
	addrsCache *addrsCache

	stringCache *stringCache // only used by IPv6 when there is a zone

	identifierStr *IdentifierStr

	canonicalHost *HostName
}

type addressInternal struct {
	section *AddressSection
	zone    Zone
	cache   *addressCache
}

func (addr *addressInternal) GetBitCount() BitCount {
	section := addr.section
	if section == nil {
		return 0
	}
	return section.GetBitCount()
}

func (addr *addressInternal) GetByteCount() int {
	section := addr.section
	if section == nil {
		return 0
	}
	return section.GetByteCount()
}

func (addr *addressInternal) getCount() *big.Int {
	section := addr.section
	if section == nil {
		return bigOne()
	}
	return section.GetCount()
}

func (addr *addressInternal) GetPrefixCount() *big.Int {
	section := addr.section
	if section == nil {
		return bigOne()
	}
	return section.GetPrefixCount()
}

func (addr *addressInternal) GetPrefixCountLen(prefixLen BitCount) *big.Int {
	section := addr.section
	if section == nil {
		return bigOne()
	}
	return section.GetPrefixCountLen(prefixLen)
}

func (addr *addressInternal) GetBlockCount(segmentCount int) *big.Int {
	section := addr.section
	if section == nil {
		return bigOne()
	}
	return section.GetBlockCount(segmentCount)
}

// TestBit computes (this & (1 << n)) != 0), using the lower value of this segment.
func (addr *addressInternal) testBit(n BitCount) bool {
	return addr.section.TestBit(n)
}

// Returns true if the bit in the lower value of this segment at the given index is 1, where index 0 is the most significant bit.
func (addr *addressInternal) isOneBit(bitIndex BitCount) bool {
	return addr.section.IsOneBit(bitIndex)
}

func (addr *addressInternal) isMultiple() bool {
	return addr.section != nil && addr.section.isMultiple()
}

func (addr *addressInternal) isPrefixed() bool {
	return addr.section != nil && addr.section.IsPrefixed()
}

func (addr *addressInternal) GetPrefixLen() PrefixLen {
	return addr.getPrefixLen().copy()
}

func (addr *addressInternal) getPrefixLen() PrefixLen {
	if addr.section == nil {
		return nil
	}
	return addr.section.getPrefixLen()
}

func (addr *addressInternal) IsSinglePrefixBlock() bool {
	prefLen := addr.getPrefixLen()
	return prefLen != nil && addr.section.IsSinglePrefixBlock()
}

func (addr *addressInternal) IsPrefixBlock() bool {
	prefLen := addr.getPrefixLen()
	return prefLen != nil && addr.section.ContainsPrefixBlock(prefLen.bitCount())
}

func (addr *addressInternal) ContainsPrefixBlock(prefixLen BitCount) bool {
	return addr.section == nil || addr.section.ContainsPrefixBlock(prefixLen)
}

func (addr *addressInternal) ContainsSinglePrefixBlock(prefixLen BitCount) bool {
	return addr.section == nil || addr.section.ContainsSinglePrefixBlock(prefixLen)
}

func (addr *addressInternal) GetMinPrefixLenForBlock() BitCount {
	section := addr.section
	if section == nil {
		return 0
	}
	return section.GetMinPrefixLenForBlock()
}

func (addr *addressInternal) GetPrefixLenForSingleBlock() PrefixLen {
	section := addr.section
	if section == nil {
		return cacheBitCount(0)
	}
	return section.GetPrefixLenForSingleBlock()
}

func (addr *addressInternal) compareSize(other AddressType) int {
	section := addr.section
	if other == nil || other.ToAddressBase() == nil {
		// our size is 1 or greater, other 0
		return 1
	}
	return section.CompareSize(other.ToAddressBase().GetSection())
}

func (addr *addressInternal) trieCompare(other *Address) int {
	if addr.toAddress() == other {
		return 0
	}
	segmentCount := addr.getDivisionCount()
	bitsPerSegment := addr.GetBitsPerSegment()
	o1Pref := addr.GetPrefixLen()
	o2Pref := other.GetPrefixLen()
	bitsMatchedSoFar := 0
	i := 0
	for {
		segment1 := addr.getSegment(i)
		segment2 := other.getSegment(i)
		pref1 := getSegmentPrefLen(addr.toAddress(), o1Pref, bitsPerSegment, bitsMatchedSoFar, segment1)
		pref2 := getSegmentPrefLen(other, o2Pref, bitsPerSegment, bitsMatchedSoFar, segment2)
		if pref1 != nil {
			segmentPref1 := pref1.Len()
			segmentPref2 := pref2.Len()
			if pref2 != nil && segmentPref2 <= segmentPref1 {
				matchingBits := getMatchingBits(segment1, segment2, segmentPref2, bitsPerSegment)
				if matchingBits >= segmentPref2 {
					if segmentPref2 == segmentPref1 {
						// same prefix block
						return 0
					}
					// segmentPref2 is shorter prefix, prefix bits match, so depends on bit at index segmentPref2
					if segment1.IsOneBit(segmentPref2) {
						return 1
					}
					return -1
				}
				return CompareSegInt(segment1.GetSegmentValue(), segment2.GetSegmentValue())
			} else {
				matchingBits := getMatchingBits(segment1, segment2, segmentPref1, bitsPerSegment)
				if matchingBits >= segmentPref1 {
					if segmentPref1 < bitsPerSegment {
						if segment2.IsOneBit(segmentPref1) {
							return -1
						}
						return 1
					} else {
						i++
						if i == segmentCount {
							return 1 // o1 with prefix length matching bit count is the bigger
						} // else must check the next segment
					}
				} else {
					return CompareSegInt(segment1.GetSegmentValue(), segment2.GetSegmentValue())
				}
			}
		} else if pref2 != nil {
			segmentPref2 := pref2.Len()
			matchingBits := getMatchingBits(segment1, segment2, segmentPref2, bitsPerSegment)
			if matchingBits >= segmentPref2 {
				if segmentPref2 < bitsPerSegment {
					if segment1.IsOneBit(segmentPref2) {
						return 1
					}
					return -1
				} else {
					i++
					if i == segmentCount {
						return -1 // o2 with prefix length matching bit count is the bigger
					} // else must check the next segment
				}
			} else {
				return CompareSegInt(segment1.GetSegmentValue(), segment2.GetSegmentValue())
			}
		} else {
			matchingBits := getMatchingBits(segment1, segment2, bitsPerSegment, bitsPerSegment)
			if matchingBits < bitsPerSegment { // no match - the current subnet/address is not here
				return CompareSegInt(segment1.GetSegmentValue(), segment2.GetSegmentValue())
			} else {
				i++
				if i == segmentCount {
					// same address
					return 0
				} // else must check the next segment
			}
		}
		bitsMatchedSoFar += bitsPerSegment
	}
}

// trieIncrement returns the next address according to address trie ordering
func (addr *addressInternal) trieIncrement() *Address {
	res := tree.TrieIncrement(&addressTrieKey{addr.toAddress()})
	if res == nil {
		return nil
	}
	return res.(*addressTrieKey).Address
}

// trieDecrement returns the previous key according to the trie ordering
func (addr *addressInternal) trieDecrement() *Address {
	res := tree.TrieDecrement(&addressTrieKey{addr.toAddress()})
	if res == nil {
		return nil
	}
	return res.(*addressTrieKey).Address
}

func (addr *addressInternal) toString() string { // using non-pointer receiver makes it work well with fmt
	section := addr.section
	if section == nil {
		return nilSection() // note no zone possible since a zero-address like Address{} or IPAddress{} cannot have a zone
	} else if addr.isMAC() {
		return addr.toNormalizedString()
	}
	return addr.toCanonicalString()
}

func (addr *addressInternal) IsSequential() bool {
	section := addr.section
	if section == nil {
		return true
	}
	return section.IsSequential()
}

func (addr *addressInternal) getSegment(index int) *AddressSegment {
	return addr.section.GetSegment(index)
}

func (addr *addressInternal) GetBitsPerSegment() BitCount {
	section := addr.section
	if section == nil {
		return 0
	}
	return section.GetBitsPerSegment()
}

func (addr *addressInternal) GetBytesPerSegment() int {
	section := addr.section
	if section == nil {
		return 0
	}
	return section.GetBytesPerSegment()
}

func (addr *addressInternal) getMaxSegmentValue() SegInt {
	return addr.section.GetMaxSegmentValue()
}

func (addr *addressInternal) checkIdentity(section *AddressSection) *Address {
	if section == nil {
		return nil
	} else if section == addr.section {
		return addr.toAddress()
	}
	return createAddress(section, addr.zone)
}

func (addr *addressInternal) getLower() *Address {
	lower, _ := addr.getLowestHighestAddrs()
	return lower
}

func (addr *addressInternal) getUpper() *Address {
	_, upper := addr.getLowestHighestAddrs()
	return upper
}

func (addr *addressInternal) getLowestHighestAddrs() (lower, upper *Address) {
	if !addr.isMultiple() {
		lower = addr.toAddress()
		upper = lower
		return
	}
	cache := addr.cache
	if cache == nil {
		return addr.createLowestHighestAddrs()
	}
	cached := cache.addrsCache
	if cached == nil {
		cached = &addrsCache{}
		cached.lower, cached.upper = addr.createLowestHighestAddrs()
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&cache.addrsCache))
		atomic.StorePointer(dataLoc, unsafe.Pointer(cached))
	}
	lower, upper = cached.lower, cached.upper
	return
}

func (addr *addressInternal) createLowestHighestAddrs() (lower, upper *Address) {
	lower = addr.checkIdentity(addr.section.GetLower())
	upper = addr.checkIdentity(addr.section.GetUpper())
	return
}

func (addr *addressInternal) toMaxLower() *Address {
	section := addr.section
	if section == nil {
		return addr.toAddress()
	}
	return addr.checkIdentity(addr.section.toMaxLower())
}

func (addr *addressInternal) toMinUpper() *Address {
	section := addr.section
	if section == nil {
		return addr.toAddress()
	}
	return addr.checkIdentity(addr.section.toMinUpper())
}

func (addr *addressInternal) IsZero() bool {
	section := addr.section
	if section == nil {
		return true
	}
	return section.IsZero()
}

func (addr *addressInternal) IncludesZero() bool {
	section := addr.section
	if section == nil {
		return true
	}
	return section.IncludesZero()
}

func (addr *addressInternal) IsFullRange() bool {
	section := addr.section
	if section == nil {
		// when no bits, the only value 0 is the max value too
		return true
	}
	return section.IsFullRange()
}

func (addr *addressInternal) toAddress() *Address {
	return (*Address)(unsafe.Pointer(addr))
}

func (addr *addressInternal) hasNoDivisions() bool {
	return addr.section.hasNoDivisions()
}

func (addr *addressInternal) getDivision(index int) *AddressDivision {
	return addr.section.getDivision(index)
}

func (addr *addressInternal) getDivisionCount() int {
	if addr.section == nil {
		return 0
	}
	return addr.section.GetDivisionCount()
}

func (addr *addressInternal) getDivisionsInternal() []*AddressDivision {
	return addr.section.getDivisionsInternal()
}

func (addr *addressInternal) toPrefixBlock() *Address {
	return addr.checkIdentity(addr.section.toPrefixBlock())
}

func (addr *addressInternal) toBlock(segmentIndex int, lower, upper SegInt) *Address {
	return addr.checkIdentity(addr.section.toBlock(segmentIndex, lower, upper))
}

func (addr *addressInternal) toPrefixBlockLen(prefLen BitCount) *Address {
	return addr.checkIdentity(addr.section.toPrefixBlockLen(prefLen))
}

func (addr *addressInternal) reverseBytes() (*Address, addrerr.IncompatibleAddressError) {
	sect, err := addr.section.ReverseBytes()
	if err != nil {
		return nil, err
	}
	return addr.checkIdentity(sect), nil
}

func (addr *addressInternal) reverseBits(perByte bool) (*Address, addrerr.IncompatibleAddressError) {
	sect, err := addr.section.ReverseBits(perByte)
	if err != nil {
		return nil, err
	}
	return addr.checkIdentity(sect), nil
}

func (addr *addressInternal) reverseSegments() *Address {
	return addr.checkIdentity(addr.section.ReverseSegments())
}

// isIPv4() returns whether this matches an IPv4 address.
// we allow nil receivers to allow this to be called following a failed conversion like ToIP()
func (addr *addressInternal) isIPv4() bool {
	return addr.section != nil && addr.section.matchesIPv4AddressType()
}

// isIPv6() returns whether this matches an IPv6 address.
// we allow nil receivers to allow this to be called following a failed conversion like ToIP()
func (addr *addressInternal) isIPv6() bool {
	return addr.section != nil && addr.section.matchesIPv6AddressType()
}

// isIPv6() returns whether this matches an IPv6 address.
// we allow nil receivers to allow this to be called following a failed conversion like ToIP()
func (addr *addressInternal) isMAC() bool {
	return addr.section != nil && addr.section.matchesMACAddressType()
}

// isIP() returns whether this matches an IP address.
// It must be IPv4, IPv6, or the zero IPAddress which has no segments
// we allow nil receivers to allow this to be called following a failed conversion like ToIP()
func (addr *addressInternal) isIP() bool {
	return addr.section == nil /* zero addr */ || addr.section.matchesIPAddressType()
}

func (addr *addressInternal) prefixEquals(other AddressType) bool {
	otherAddr := other.ToAddressBase()
	if addr.toAddress() == otherAddr {
		return true
	}
	otherSection := otherAddr.GetSection()
	if addr.section == nil {
		return otherSection.GetSegmentCount() == 0
	}
	return addr.section.PrefixEqual(otherSection) &&
		// if it is IPv6 and has a zone, then it does not contain addresses from other zones
		addr.isSameZone(otherAddr)
}

func (addr *addressInternal) prefixContains(other AddressType) bool {
	otherAddr := other.ToAddressBase()
	if addr.toAddress() == otherAddr {
		return true
	}
	otherSection := otherAddr.GetSection()
	if addr.section == nil {
		return otherSection.GetSegmentCount() == 0
	}
	return addr.section.PrefixContains(otherSection) &&
		// if it is IPv6 and has a zone, then it does not contain addresses from other zones
		addr.isSameZone(otherAddr)
}

func (addr *addressInternal) contains(other AddressType) bool {
	if other == nil {
		return true
	}
	otherAddr := other.ToAddressBase()
	if addr.toAddress() == otherAddr || otherAddr == nil {
		return true
	}
	otherSection := otherAddr.GetSection()
	if addr.section == nil {
		return otherSection.GetSegmentCount() == 0
	}
	return addr.section.Contains(otherSection) &&
		// if it is IPv6 and has a zone, then it does not contain addresses from other zones
		addr.isSameZone(otherAddr)
}

func (addr *addressInternal) equals(other AddressType) bool {
	if other == nil {
		return false
	}
	otherAddr := other.ToAddressBase()
	if addr.toAddress() == otherAddr {
		return true
	} else if otherAddr == nil {
		return false
	}
	otherSection := otherAddr.GetSection()
	if addr.section == nil {
		return otherSection.GetSegmentCount() == 0
	}
	return addr.section.Equal(otherSection) &&
		// if it it is IPv6 and has a zone, then it does not equal addresses from other zones
		addr.isSameZone(otherAddr)
}

func (addr *IPAddress) equalsSameVersion(other *IPAddress) bool {
	otherAddr := other.ToAddressBase()
	if addr.toAddress() == otherAddr {
		return true
	} else if otherAddr == nil {
		return false
	}
	otherSection := otherAddr.GetSection()
	return addr.section.sameCountTypeEquals(otherSection) &&
		// if it it is IPv6 and has a zone, then it does not equal addresses from other zones
		addr.isSameZone(otherAddr)
}

func (addr *addressInternal) withoutPrefixLen() *Address {
	return addr.checkIdentity(addr.section.withoutPrefixLen())
}

func (addr *addressInternal) adjustPrefixLen(prefixLen BitCount) *Address {
	return addr.checkIdentity(addr.section.adjustPrefixLen(prefixLen))
}

func (addr *addressInternal) adjustPrefixLenZeroed(prefixLen BitCount) (res *Address, err addrerr.IncompatibleAddressError) {
	section, err := addr.section.adjustPrefixLenZeroed(prefixLen)
	if err == nil {
		res = addr.checkIdentity(section)
	}
	return
}

func (addr *addressInternal) setPrefixLen(prefixLen BitCount) *Address {
	return addr.checkIdentity(addr.section.setPrefixLen(prefixLen))
}

func (addr *addressInternal) setPrefixLenZeroed(prefixLen BitCount) (res *Address, err addrerr.IncompatibleAddressError) {
	section, err := addr.section.setPrefixLenZeroed(prefixLen)
	if err == nil {
		res = addr.checkIdentity(section)
	}
	return
}

func (addr *addressInternal) assignPrefixForSingleBlock() *Address {
	newPrefix := addr.GetPrefixLenForSingleBlock()
	if newPrefix == nil {
		return nil
	}
	return addr.checkIdentity(addr.section.setPrefixLen(newPrefix.bitCount()))
}

// Constructs an equivalent address section with the smallest CIDR prefix possible (largest network),
// such that the range of values are a set of subnet blocks for that prefix.
func (addr *addressInternal) assignMinPrefixForBlock() *Address {
	return addr.setPrefixLen(addr.GetMinPrefixLenForBlock())
}

// toSingleBlockOrAddress converts to a single prefix block or address.
// If the given address is a single prefix block, it is returned.
// If it can be converted to a single prefix block by assigning a prefix length, the converted block is returned.
// If it is a single address, any prefix length is removed and the address is returned.
// Otherwise, nil is returned.
func (addr *addressInternal) toSinglePrefixBlockOrAddress() *Address {
	if !addr.isMultiple() {
		if !addr.isPrefixed() {
			return addr.toAddress()
		}
		return addr.withoutPrefixLen()
	} else if addr.IsSinglePrefixBlock() {
		return addr.toAddress()
	} else {
		series := addr.assignPrefixForSingleBlock()
		if series != nil {
			return series
		}
	}
	return nil
}

func (addr *addressInternal) isSameZone(other *Address) bool {
	return addr.zone == other.ToAddressBase().zone
}

func (addr *addressInternal) getAddrType() addrType {
	if addr.section == nil {
		return zeroType
	}
	return addr.section.addrType
}

// equivalent to section.sectionIterator
func (addr *addressInternal) addrIterator(excludeFunc func([]*AddressDivision) bool) AddressIterator {
	useOriginal := !addr.isMultiple()
	original := addr.toAddress()
	var iterator SegmentsIterator
	if useOriginal {
		if excludeFunc != nil && excludeFunc(addr.getDivisionsInternal()) {
			original = nil // the single-valued iterator starts out empty
		}
	} else {
		address := addr.toAddress()
		iterator = allSegmentsIterator(
			addr.getDivisionCount(),
			nil,
			func(index int) SegmentIterator { return address.getSegment(index).iterator() },
			excludeFunc)
	}
	return addrIterator(
		useOriginal,
		original,
		original.getPrefixLen(),
		false,
		iterator)
}

func (addr *addressInternal) prefixIterator(isBlockIterator bool) AddressIterator {
	prefLen := addr.getPrefixLen()
	if prefLen == nil {
		return addr.addrIterator(nil)
	}
	var useOriginal bool
	if isBlockIterator {
		useOriginal = addr.IsSinglePrefixBlock()
	} else {
		useOriginal = addr.GetPrefixCount().Cmp(bigOneConst()) == 0
	}
	prefLength := prefLen.bitCount()
	bitsPerSeg := addr.GetBitsPerSegment()
	bytesPerSeg := addr.GetBytesPerSegment()
	networkSegIndex := getNetworkSegmentIndex(prefLength, bytesPerSeg, bitsPerSeg)
	hostSegIndex := getHostSegmentIndex(prefLength, bytesPerSeg, bitsPerSeg)
	segCount := addr.getDivisionCount()
	var iterator SegmentsIterator
	address := addr.toAddress()
	if !useOriginal {
		var hostSegIteratorProducer func(index int) SegmentIterator
		if isBlockIterator {
			hostSegIteratorProducer = func(index int) SegmentIterator {
				seg := address.GetSegment(index)
				if seg.isPrefixed() { // IP address segments know their own prefix, MAC segments do not
					return seg.prefixBlockIterator()
				}
				segPref := getPrefixedSegmentPrefixLength(bitsPerSeg, prefLength, index)
				return seg.prefixedBlockIterator(segPref.bitCount())
			}
		} else {
			hostSegIteratorProducer = func(index int) SegmentIterator {
				seg := address.GetSegment(index)
				if seg.isPrefixed() { // IP address segments know their own prefix, MACS segments do not
					return seg.prefixIterator()
				}
				segPref := getPrefixedSegmentPrefixLength(bitsPerSeg, prefLength, index)
				return seg.prefixedIterator(segPref.bitCount())
			}
		}
		iterator = segmentsIterator(
			segCount,
			nil, //when no prefix we defer to other iterator, when there is one we use the whole original section in the encompassing iterator and not just the original segments
			func(index int) SegmentIterator { return address.GetSegment(index).iterator() },
			nil,
			networkSegIndex,
			hostSegIndex,
			hostSegIteratorProducer)
	}
	if isBlockIterator {
		return addrIterator(
			useOriginal,
			address,
			address.getPrefixLen(),
			prefLength < addr.GetBitCount(),
			iterator)
	}
	return prefixAddrIterator(
		useOriginal,
		address,
		address.getPrefixLen(),
		iterator)
}

func (addr *addressInternal) blockIterator(segmentCount int) AddressIterator {
	if segmentCount < 0 {
		segmentCount = 0
	}
	allSegsCount := addr.getDivisionCount()
	if segmentCount >= allSegsCount {
		return addr.addrIterator(nil)
	}
	useOriginal := !addr.section.isMultipleTo(segmentCount)
	address := addr.toAddress()
	var iterator SegmentsIterator
	if !useOriginal {
		var hostSegIteratorProducer func(index int) SegmentIterator
		hostSegIteratorProducer = func(index int) SegmentIterator {
			return address.GetSegment(index).identityIterator()
		}
		segIteratorProducer := func(index int) SegmentIterator {
			return address.GetSegment(index).iterator()
		}
		iterator = segmentsIterator(
			allSegsCount,
			nil, //when no prefix we defer to other iterator, when there is one we use the whole original section in the encompassing iterator and not just the original segments
			segIteratorProducer,
			nil,
			segmentCount-1,
			segmentCount,
			hostSegIteratorProducer)
	}
	return addrIterator(
		useOriginal,
		address,
		address.getPrefixLen(),
		addr.section.isMultipleFrom(segmentCount),
		iterator)
}

func (addr *addressInternal) sequentialBlockIterator() AddressIterator {
	return addr.blockIterator(addr.getSequentialBlockIndex())
}

func (addr *addressInternal) getSequentialBlockIndex() int {
	if addr.section == nil {
		return 0
	}
	return addr.section.GetSequentialBlockIndex()
}

func (addr *addressInternal) getSequentialBlockCount() *big.Int {
	if addr.section == nil {
		return bigOne()
	}
	return addr.section.GetSequentialBlockCount()
}

func (addr *addressInternal) hasZone() bool {
	return addr.zone != NoZone
}

func (addr *addressInternal) increment(increment int64) *Address {
	return addr.checkIdentity(addr.section.increment(increment))
}

func (addr *addressInternal) incrementBoundary(increment int64) *Address {
	return addr.checkIdentity(addr.section.incrementBoundary(increment))
}

func (addr *addressInternal) getStringCache() *stringCache {
	cache := addr.cache
	if cache == nil {
		return nil
	}
	return addr.cache.stringCache
}

func (addr *addressInternal) getSegmentStrings() []string {
	return addr.section.getSegmentStrings()
}

func (addr *addressInternal) toCanonicalString() string {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.ToIPv6().toCanonicalString(addr.zone)
		}
		return cacheStr(&cache.canonicalString,
			func() string { return addr.section.ToIPv6().toCanonicalString(addr.zone) })
	}
	return addr.section.ToCanonicalString()
}

func (addr *addressInternal) toNormalizedString() string {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.ToIPv6().toNormalizedString(addr.zone)
		}
		return cacheStr(&cache.normalizedIPv6String,
			func() string { return addr.section.ToIPv6().toNormalizedString(addr.zone) })
	}
	return addr.section.ToNormalizedString()
}

func (addr *addressInternal) toCompressedString() string {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.ToIPv6().toCompressedString(addr.zone)
		}
		return cacheStr(&cache.compressedIPv6String,
			func() string { return addr.section.ToIPv6().toCompressedString(addr.zone) })
	}
	return addr.section.ToCompressedString()
}

func (addr *addressInternal) toOctalString(with0Prefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.toOctalStringZoned(with0Prefix, addr.zone)
		}
		var cacheField **string
		if with0Prefix {
			cacheField = &cache.octalStringPrefixed
		} else {
			cacheField = &cache.octalString
		}
		return cacheStrErr(cacheField,
			func() (string, addrerr.IncompatibleAddressError) {
				return addr.section.toOctalStringZoned(with0Prefix, addr.zone)
			})
	}
	return addr.section.ToOctalString(with0Prefix)
}

func (addr *addressInternal) toBinaryString(with0bPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.toBinaryStringZoned(with0bPrefix, addr.zone)
		}
		var cacheField **string
		if with0bPrefix {
			cacheField = &cache.binaryStringPrefixed
		} else {
			cacheField = &cache.binaryString
		}
		return cacheStrErr(cacheField,
			func() (string, addrerr.IncompatibleAddressError) {
				return addr.section.toBinaryStringZoned(with0bPrefix, addr.zone)
			})
	}
	return addr.section.ToBinaryString(with0bPrefix)
}

func (addr *addressInternal) toHexString(with0xPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr.hasZone() {
		cache := addr.getStringCache()
		if cache == nil {
			return addr.section.toHexStringZoned(with0xPrefix, addr.zone)
		}
		var cacheField **string
		if with0xPrefix {
			cacheField = &cache.hexStringPrefixed
		} else {
			cacheField = &cache.hexString
		}
		return cacheStrErr(cacheField,
			func() (string, addrerr.IncompatibleAddressError) {
				return addr.section.toHexStringZoned(with0xPrefix, addr.zone)
			})
	}
	return addr.section.ToHexString(with0xPrefix)
}

func (addr *addressInternal) format(state fmt.State, verb rune) {
	section := addr.section
	section.format(state, verb, addr.zone, addr.isIP())
}

var zeroAddr = createAddress(zeroSection, NoZone)

type Address struct {
	addressInternal
}

func (addr *Address) init() *Address {
	if addr.section == nil {
		return zeroAddr // this has a zero section rather that a nil section
	}
	return addr
}

func (addr *Address) GetCount() *big.Int {
	if addr == nil {
		return bigZero()
	}
	return addr.getCount()
}

func (addr *Address) IsMultiple() bool {
	return addr != nil && addr.isMultiple()
}

func (addr *Address) IsPrefixed() bool {
	return addr != nil && addr.isPrefixed()
}

func (addr *Address) PrefixEqual(other AddressType) bool {
	return addr.init().prefixEquals(other)
}

func (addr *Address) PrefixContains(other AddressType) bool {
	return addr.init().prefixContains(other)
}

func (addr *Address) Contains(other AddressType) bool {
	if addr == nil {
		return other == nil || other.ToAddressBase() == nil
	}
	return addr.init().contains(other)
}

func (addr *Address) Compare(item AddressItem) int {
	return CountComparator.Compare(addr, item)
}

func (addr *Address) Equal(other AddressType) bool {
	if addr == nil {
		return other == nil || other.ToAddressBase() == nil
	} else if other.ToAddressBase() == nil {
		return false
	}
	return addr.init().equals(other)
}

func (addr *Address) CompareSize(other AddressType) int {
	if addr == nil {
		if other != nil && other.ToAddressBase() != nil {
			// we have size 0, other has size >= 1
			return -1
		}
		return 0
	}
	return addr.init().compareSize(other)
}

// TrieCompare compares two addresses according to the trie order.  It returns a number less than zero, zero, or a number greater than zero if the first address argument is less than, equal to, or greater than the second.
func (addr *Address) TrieCompare(other *Address) (int, addrerr.IncompatibleAddressError) {
	if thisAddr := addr.ToIPv4(); thisAddr != nil {
		if oth := other.ToIPv4(); oth != nil {
			return thisAddr.TrieCompare(oth), nil
		}
	} else if thisAddr := addr.ToIPv6(); thisAddr != nil {
		if oth := other.ToIPv6(); oth != nil {
			return thisAddr.TrieCompare(oth), nil
		}
	} else if thisAddr := addr.ToMAC(); thisAddr != nil {
		if oth := other.ToMAC(); oth != nil {
			return thisAddr.TrieCompare(oth)
		}
	}
	return 0, &incompatibleAddressError{addressError{key: "ipaddress.error.mismatched.bit.size"}}
}

// TrieIncrement returns the next address according to address trie ordering
func (addr *Address) TrieIncrement() *Address {
	return addr.trieIncrement()
}

// TrieDecrement returns the previous key according to the trie ordering
func (addr *Address) TrieDecrement() *Address {
	return addr.trieDecrement()
}

func (addr *Address) GetSection() *AddressSection {
	return addr.init().section
}

// GetTrailingSection gets the subsection from the series starting from the given index
// The first segment is at index 0.
func (addr *Address) GetTrailingSection(index int) *AddressSection {
	return addr.GetSection().GetTrailingSection(index)
}

// GetSubSection gets the subsection from the series starting from the given index and ending just before the give endIndex
// The first segment is at index 0.
func (addr *Address) GetSubSection(index, endIndex int) *AddressSection {
	return addr.GetSection().GetSubSection(index, endIndex)
}

// CopySubSegments copies the existing segments from the given start index until but not including the segment at the given end index,
// into the given slice, as much as can be fit into the slice, returning the number of segments copied
func (addr *Address) CopySubSegments(start, end int, segs []*AddressSegment) (count int) {
	return addr.GetSection().CopySubSegments(start, end, segs)
}

// CopySegments copies the existing segments into the given slice,
// as much as can be fit into the slice, returning the number of segments copied
func (addr *Address) CopySegments(segs []*AddressSegment) (count int) {
	return addr.GetSection().CopySegments(segs)
}

// GetSegments returns a slice with the address segments.  The returned slice is not backed by the same array as this section.
func (addr *Address) GetSegments() []*AddressSegment {
	return addr.GetSection().GetSegments()
}

// GetSegment returns the segment at the given index
func (addr *Address) GetSegment(index int) *AddressSegment {
	return addr.getSegment(index)
}

// GetSegmentCount returns the segment count
func (addr *Address) GetSegmentCount() int {
	return addr.getDivisionCount()
}

// GetGenericDivision returns the segment at the given index as an DivisionType
func (addr *Address) GetGenericDivision(index int) DivisionType {
	return addr.getDivision(index)
}

// GetGenericSegment returns the segment at the given index as an AddressSegmentType
func (addr *Address) GetGenericSegment(index int) AddressSegmentType {
	return addr.getSegment(index)
}

// GetDivisionCount returns the division count
func (addr *Address) GetDivisionCount() int {
	return addr.getDivisionCount()
}

// TestBit computes (this & (1 << n)) != 0), using the lower value of this segment.
func (addr *Address) TestBit(n BitCount) bool {
	return addr.init().testBit(n)
}

// IsOneBit returns true if the bit in the lower value of this address at the given index is 1, where index 0 is the most significant bit.
func (addr *Address) IsOneBit(bitIndex BitCount) bool {
	return addr.init().isOneBit(bitIndex)
}

func (addr *Address) GetLower() *Address {
	return addr.init().getLower()
}

func (addr *Address) GetUpper() *Address {
	return addr.init().getUpper()
}

func (addr *Address) GetValue() *big.Int {
	return addr.init().section.GetValue()
}

func (addr *Address) GetUpperValue() *big.Int {
	return addr.init().section.GetUpperValue()
}

func (addr *Address) Bytes() []byte {
	return addr.init().section.Bytes()
}

func (addr *Address) UpperBytes() []byte {
	return addr.init().section.UpperBytes()
}

func (addr *Address) CopyBytes(bytes []byte) []byte {
	return addr.init().section.CopyBytes(bytes)
}

func (addr *Address) CopyUpperBytes(bytes []byte) []byte {
	return addr.init().section.CopyUpperBytes(bytes)
}

func (addr *Address) IsMax() bool {
	return addr.init().section.IsMax()
}

func (addr *Address) IncludesMax() bool {
	return addr.init().section.IncludesMax()
}

func (addr *Address) ToPrefixBlock() *Address {
	return addr.init().toPrefixBlock()
}

func (addr *Address) ToPrefixBlockLen(prefLen BitCount) *Address {
	return addr.init().toPrefixBlockLen(prefLen)
}

func (addr *Address) ToBlock(segmentIndex int, lower, upper SegInt) *Address {
	return addr.init().toBlock(segmentIndex, lower, upper)
}

func (addr *Address) WithoutPrefixLen() *Address {
	if !addr.IsPrefixed() {
		return addr
	}
	return addr.init().withoutPrefixLen()
}

func (addr *Address) SetPrefixLen(prefixLen BitCount) *Address {
	return addr.init().setPrefixLen(prefixLen)
}

func (addr *Address) SetPrefixLenZeroed(prefixLen BitCount) (*Address, addrerr.IncompatibleAddressError) {
	return addr.init().setPrefixLenZeroed(prefixLen)
}

func (addr *Address) AdjustPrefixLen(prefixLen BitCount) *Address {
	return addr.adjustPrefixLen(prefixLen).ToAddressBase()
}

func (addr *Address) AdjustPrefixLenZeroed(prefixLen BitCount) (*Address, addrerr.IncompatibleAddressError) {
	res, err := addr.adjustPrefixLenZeroed(prefixLen)
	return res.ToAddressBase(), err
}

func (addr *Address) AssignPrefixForSingleBlock() *Address {
	return addr.init().assignPrefixForSingleBlock()
}

// AssignMinPrefixForBlock return an equivalent address with the smallest CIDR prefix possible (largest network),
// such that the range of values are the prefix block for that prefix.
func (addr *Address) AssignMinPrefixForBlock() *Address {
	return addr.init().assignMinPrefixForBlock()
}

// ToSinglePrefixBlockOrAddress converts to a single prefix block or address.
// If the given address is a single prefix block, it is returned.
// If it can be converted to a single prefix block by assigning a prefix length, the converted block is returned.
// If it is a single address, any prefix length is removed and the address is returned.
// Otherwise, nil is returned.
// This method provides the address formats used by tries.
func (addr *Address) ToSinglePrefixBlockOrAddress() *Address {
	return addr.init().toSinglePrefixBlockOrAddress()
}

func (addr *Address) GetMaxSegmentValue() SegInt {
	return addr.init().getMaxSegmentValue()
}

func (addr *Address) Iterator() AddressIterator {
	if addr == nil {
		return nilAddrIterator()
	}
	return addr.addrIterator(nil)
}

func (addr *Address) PrefixIterator() AddressIterator {
	return addr.prefixIterator(false)
}

func (addr *Address) PrefixBlockIterator() AddressIterator {
	return addr.prefixIterator(true)
}

func (addr *Address) BlockIterator(segmentCount int) AddressIterator {
	return addr.init().blockIterator(segmentCount)
}

func (addr *Address) SequentialBlockIterator() AddressIterator {
	return addr.init().sequentialBlockIterator()
}

func (addr *Address) GetSequentialBlockIndex() int {
	return addr.getSequentialBlockIndex()
}

func (addr *Address) GetSequentialBlockCount() *big.Int {
	return addr.getSequentialBlockCount()
}

func (addr *Address) IncrementBoundary(increment int64) *Address {
	return addr.init().IncrementBoundary(increment)
}

func (addr *Address) Increment(increment int64) *Address {
	return addr.init().increment(increment)
}

func (addr *Address) ReverseBytes() (*Address, addrerr.IncompatibleAddressError) {
	return addr.init().reverseBytes()
}

func (addr *Address) ReverseBits(perByte bool) (*Address, addrerr.IncompatibleAddressError) {
	return addr.init().reverseBits(perByte)
}

func (addr *Address) ReverseSegments() *Address {
	return addr.init().reverseSegments()
}

// IsMulticast returns whether this address is multicast
func (addr *Address) IsMulticast() bool {
	if thisAddr := addr.ToIPv4(); thisAddr != nil {
		return thisAddr.IsMulticast()
	} else if thisAddr := addr.ToIPv6(); thisAddr != nil {
		return thisAddr.IsMulticast()
	} else if thisAddr := addr.ToMAC(); thisAddr != nil {
		return thisAddr.IsMulticast()
	}
	return false
}

// IsLocal returns whether the address can be considered a local address (as opposed to a global one)
func (addr *Address) IsLocal() bool {
	if thisAddr := addr.ToIPv4(); thisAddr != nil {
		return thisAddr.IsLocal()
	} else if thisAddr := addr.ToIPv6(); thisAddr != nil {
		return thisAddr.IsLocal()
	} else if thisAddr := addr.ToMAC(); thisAddr != nil {
		return thisAddr.IsLocal()
	}
	return false
}

func (addr *Address) GetLeadingBitCount(ones bool) BitCount {
	return addr.GetSection().GetLeadingBitCount(ones)
}

func (addr *Address) GetTrailingBitCount(ones bool) BitCount {
	return addr.GetSection().GetTrailingBitCount(ones)
}

func (addr Address) Format(state fmt.State, verb rune) {
	addr.init().format(state, verb)
}

func (addr *Address) String() string {
	if addr == nil {
		return nilString()
	}
	return addr.init().toString()
}

func (addr *Address) GetSegmentStrings() []string {
	if addr == nil {
		return nil
	}
	return addr.init().getSegmentStrings()
}

func (addr *Address) ToCanonicalString() string {
	if addr == nil {
		return nilString()
	}
	return addr.init().toCanonicalString()
}

func (addr *Address) ToNormalizedString() string {
	if addr == nil {
		return nilString()
	}
	return addr.init().toNormalizedString()
}

func (addr *Address) ToCompressedString() string {
	if addr == nil {
		return nilString()
	}
	return addr.init().toCompressedString()
}

func (addr *Address) ToHexString(with0xPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr == nil {
		return nilString(), nil
	}
	return addr.init().toHexString(with0xPrefix)
}

func (addr *Address) ToOctalString(with0Prefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr == nil {
		return nilString(), nil
	}
	return addr.init().toOctalString(with0Prefix)
}

func (addr *Address) ToBinaryString(with0bPrefix bool) (string, addrerr.IncompatibleAddressError) {
	if addr == nil {
		return nilString(), nil
	}
	return addr.init().toBinaryString(with0bPrefix)
}

func (addr *Address) ToCustomString(stringOptions addrstr.StringOptions) string {
	if addr == nil {
		return nilString()
	}
	return addr.GetSection().toCustomStringZoned(stringOptions, addr.zone)
}

func (addr *Address) ToAddressString() HostIdentifierString {
	if addr.isIP() {
		return addr.ToIP().ToAddressString()
	} else if addr.isMAC() {
		return addr.ToMAC().ToAddressString()
	}
	return nil
}

func (addr *Address) IsIPv4() bool {
	return addr != nil && addr.isIPv4()
}

func (addr *Address) IsIPv6() bool {
	return addr != nil && addr.isIPv6()
}

func (addr *Address) IsIP() bool {
	return addr != nil && addr.isIP()
}

func (addr *Address) IsMAC() bool {
	return addr != nil && addr.isMAC()
}

func (addr *Address) ToAddressBase() *Address {
	return addr
}

func (addr *Address) ToIP() *IPAddress {
	if addr.IsIP() {
		return (*IPAddress)(unsafe.Pointer(addr))
	}
	return nil
}

func (addr *Address) ToIPv6() *IPv6Address {
	if addr.IsIPv6() {
		return (*IPv6Address)(unsafe.Pointer(addr))
	}
	return nil
}

func (addr *Address) ToIPv4() *IPv4Address {
	if addr.IsIPv4() {
		return (*IPv4Address)(unsafe.Pointer(addr))
	}
	return nil
}

func (addr *Address) ToMAC() *MACAddress {
	if addr.IsMAC() {
		return (*MACAddress)(addr)
	}
	return nil
}

func (addr *Address) Wrap() WrappedAddress {
	return WrapAddress(addr.init())
}

// ToKey creates the associated address key.
// While addresses can be compare with the Compare, TrieCompare or Equal methods as well as various provided instances of AddressComparator,
// they are not comparable with go operators.
// However, AddressKey instances are comparable with go operators, and thus can be used as map keys.
func (addr *Address) ToKey() *AddressKey {
	if thisAddr := addr.ToIPv4(); thisAddr != nil {
		return thisAddr.ToKey().ToBaseKey()
	} else if thisAddr := addr.ToIPv6(); thisAddr != nil {
		return thisAddr.ToKey().ToBaseKey()
	} else if thisAddr := addr.ToMAC(); thisAddr != nil {
		return thisAddr.ToKey().ToBaseKey()
	}
	return nil
}
