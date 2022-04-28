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
	"math/big"
	"net"
)

func NewIPv4SeqRange(one, two *IPv4Address) *IPv4AddressSeqRange {
	if one == nil && two == nil {
		one = zeroIPv4
	}
	return newSeqRange(one.ToIP(), two.ToIP()).ToIPv4()
}

var zeroIPv4Range = NewIPv4SeqRange(zeroIPv4, zeroIPv4)

type IPv4AddressSeqRange struct {
	ipAddressSeqRangeInternal
}

func (rng *IPv4AddressSeqRange) init() *IPv4AddressSeqRange {
	if rng.lower == nil {
		return zeroIPv4Range
	}
	return rng
}

// GetCount returns the count of addresses that this sequential range spans.
//
// Use IsMultiple if you simply want to know if the count is greater than 1.
func (rng *IPv4AddressSeqRange) GetCount() *big.Int {
	if rng == nil {
		return bigZero()
	}
	return rng.init().getCount()
}

// IsMultiple returns whether this range represents a range of multiple addresses
func (rng *IPv4AddressSeqRange) IsMultiple() bool {
	return rng != nil && rng.isMultiple()
}

// String implements the fmt.Stringer interface,
// returning the lower address canonical string, followed by the default separator " -> ",
// followed by the upper address canonical string.
// It returns "<nil>" if the receiver is a nil pointer.
func (rng *IPv4AddressSeqRange) String() string {
	if rng == nil {
		return nilString()
	}
	return rng.ToString((*IPv4Address).String, DefaultSeqRangeSeparator, (*IPv4Address).String)
}

func (rng IPv4AddressSeqRange) Format(state fmt.State, verb rune) {
	rng.init().format(state, verb)
}

func (rng *IPv4AddressSeqRange) ToString(lowerStringer func(*IPv4Address) string, separator string, upperStringer func(*IPv4Address) string) string {
	if rng == nil {
		return nilString()
	}
	return rng.init().toString(
		func(addr *IPAddress) string {
			return lowerStringer(addr.ToIPv4())
		},
		separator,
		func(addr *IPAddress) string {
			return upperStringer(addr.ToIPv4())
		},
	)
}

func (rng *IPv4AddressSeqRange) ToNormalizedString() string {
	return rng.ToString((*IPv4Address).ToNormalizedString, DefaultSeqRangeSeparator, (*IPv4Address).ToNormalizedString)
}

func (rng *IPv4AddressSeqRange) ToCanonicalString() string {
	return rng.ToString((*IPv4Address).ToCanonicalString, DefaultSeqRangeSeparator, (*IPv4Address).ToNormalizedString)
}

// GetBitCount returns the number of bits in each address in the range, which is 8
func (rng *IPv4AddressSeqRange) GetBitCount() BitCount {
	return rng.GetLower().GetBitCount()
}

// GetByteCount returns the number of bytes in each address in the range, which is 1
func (rng *IPv4AddressSeqRange) GetByteCount() int {
	return rng.GetLower().GetByteCount()
}

// GetLowerIPAddress satisfies the IPAddressRange interface, returning the lower address in the range, same as GetLower()
func (rng *IPv4AddressSeqRange) GetLowerIPAddress() *IPAddress {
	return rng.init().lower
}

// GetUpperIPAddress satisfies the IPAddressRange interface, returning the upper address in the range, same as GetUpper()
func (rng *IPv4AddressSeqRange) GetUpperIPAddress() *IPAddress {
	return rng.init().upper
}

// GetLower returns the lowest address of the sequential range, the one with the lowest numeric value
func (rng *IPv4AddressSeqRange) GetLower() *IPv4Address {
	return rng.init().lower.ToIPv4()
}

// GetUpper returns the highest address of the sequential range, the one with the highest numeric value
func (rng *IPv4AddressSeqRange) GetUpper() *IPv4Address {
	return rng.init().upper.ToIPv4()
}

func (rng *IPv4AddressSeqRange) GetNetIP() net.IP {
	return rng.GetLower().GetNetIP()
}

func (rng *IPv4AddressSeqRange) CopyNetIP(bytes net.IP) net.IP {
	return rng.GetLower().CopyNetIP(bytes) // this changes the arg to 4 bytes if 16 bytes and ipv4
}

func (rng *IPv4AddressSeqRange) GetUpperNetIP() net.IP {
	return rng.GetUpper().GetUpperNetIP()
}

func (rng *IPv4AddressSeqRange) CopyUpperNetIP(bytes net.IP) net.IP {
	return rng.GetUpper().CopyUpperNetIP(bytes) // this changes the arg to 4 bytes if 16 bytes and ipv4
}

func (rng *IPv4AddressSeqRange) Bytes() []byte {
	return rng.GetLower().Bytes()
}

func (rng *IPv4AddressSeqRange) CopyBytes(bytes []byte) []byte {
	return rng.GetLower().CopyBytes(bytes)
}

func (rng *IPv4AddressSeqRange) UpperBytes() []byte {
	return rng.GetUpper().UpperBytes()
}

func (rng *IPv4AddressSeqRange) CopyUpperBytes(bytes []byte) []byte {
	return rng.GetUpper().CopyUpperBytes(bytes)
}

func (rng *IPv4AddressSeqRange) GetValue() *big.Int {
	return rng.GetLower().GetValue()
}

func (rng *IPv4AddressSeqRange) GetUpperValue() *big.Int {
	return rng.GetUpper().GetValue()
}

// Contains returns whether this range contains all addresses in the given address or subnet.
func (rng *IPv4AddressSeqRange) Contains(other IPAddressType) bool {
	if rng == nil {
		return other == nil || other.ToAddressBase() == nil
	}
	return rng.init().contains(other)
}

// ContainsRange returns whether all the addresses in the given sequential range are also contained in this sequential range.
func (rng *IPv4AddressSeqRange) ContainsRange(other IPAddressSeqRangeType) bool {
	if rng == nil {
		return other == nil || other.ToIP() == nil
	}
	return rng.init().containsRange(other)
}

// Equal returns whether the given sequential address range is equal to this sequential address range.
// Two sequential address ranges are equal if their lower and upper range boundaries are equal.
func (rng *IPv4AddressSeqRange) Equal(other IPAddressSeqRangeType) bool {
	if rng == nil {
		return other == nil || other.ToIP() == nil
	}
	return rng.init().equals(other)
}

func (rng *IPv4AddressSeqRange) Compare(item AddressItem) int {
	if rng != nil {
		rng = rng.init()
	}
	return CountComparator.Compare(rng, item)
}

// CompareSize compares the counts of two address ranges, the number of individual addresses within.
//
// Rather than calculating counts with GetCount, there can be more efficient ways of comparing whether one range spans more individual addresses than another.
//
// CompareSize returns a positive integer if this range has a larger count than the one given, 0 if they are the same, or a negative integer if the other has a larger count.
func (rng *IPv4AddressSeqRange) CompareSize(other IPAddressSeqRangeType) int {
	if rng == nil {
		if other != nil && other.ToIP() != nil {
			// we have size 0, other has size >= 1
			return -1
		}
		return 0
	}
	return rng.compareSize(other)
}

// ContainsPrefixBlock returns whether the range contains the block of addresses for the given prefix length.
//
// Unlike ContainsSinglePrefixBlock, whether there are multiple prefix values for the given prefix length makes no difference.
//
// Use GetMinPrefixLenForBlock to determine whether there is a prefix length for which this method returns true.
func (rng *IPv4AddressSeqRange) ContainsPrefixBlock(prefixLen BitCount) bool {
	return rng.init().ipAddressSeqRangeInternal.ContainsPrefixBlock(prefixLen)
}

// ContainsSinglePrefixBlock returns whether this address range contains a single prefix block for the given prefix length.
//
// This means there is only one prefix value for the given prefix length, and it also contains the full prefix block for that prefix, all addresses with that prefix.
//
// Use GetPrefixLenForSingleBlock to determine whether there is a prefix length for which this method returns true.
func (rng *IPv4AddressSeqRange) ContainsSinglePrefixBlock(prefixLen BitCount) bool {
	return rng.init().ipAddressSeqRangeInternal.ContainsSinglePrefixBlock(prefixLen)
}

// GetPrefixLenForSingleBlock returns a prefix length for which there is only one prefix in this range,
// and the range of values in this range matches the block of all values for that prefix.
//
// If the range can be described this way, then this method returns the same value as GetMinPrefixLenForBlock.
//
// If no such prefix length exists, returns nil.
//
// If this item represents a single value, this returns the bit count.
func (rng *IPv4AddressSeqRange) GetPrefixLenForSingleBlock() PrefixLen {
	return rng.init().ipAddressSeqRangeInternal.GetPrefixLenForSingleBlock()
}

// GetMinPrefixLenForBlock returns the smallest prefix length such that this includes the block of addresses for that prefix length.
//
// If the entire range can be described this way, then this method returns the same value as GetPrefixLenForSingleBlock.
//
// There may be a single prefix, or multiple possible prefix values in this item for the returned prefix length.
// Use GetPrefixLenForSingleBlock to avoid the case of multiple prefix values.
func (rng *IPv4AddressSeqRange) GetMinPrefixLenForBlock() BitCount {
	return rng.init().ipAddressSeqRangeInternal.GetMinPrefixLenForBlock()
}

// IsFullRange returns whether this address range covers the entire IPv4 address space.
//
// This is true if and only if both IncludesZero and IncludesMax return true.
func (rng *IPv4AddressSeqRange) IsFullRange() bool {
	return rng.init().ipAddressSeqRangeInternal.IsFullRange()
}

// IsMax returns whether this sequential range spans from the max address, the address whose bits are all ones, to itself.
func (rng *IPv4AddressSeqRange) IsMax() bool {
	return rng.init().ipAddressSeqRangeInternal.IsMax()
}

// IncludesMax returns whether this sequential range's upper value is the max value, the value whose bits are all ones.
func (rng *IPv4AddressSeqRange) IncludesMax() bool {
	return rng.init().ipAddressSeqRangeInternal.IncludesMax()
}

// Iterator provides an iterator to iterate through the individual addresses of this address range.
//
// Call GetCount for the count.
func (rng *IPv4AddressSeqRange) Iterator() IPv4AddressIterator {
	if rng == nil {
		return ipv4AddressIterator{nilAddrIterator()}
	}
	return ipv4AddressIterator{rng.init().iterator()}
}

// PrefixBlockIterator provides an iterator to iterate through the individual prefix blocks of the given prefix length,
// one for each prefix of that length in the address range.
func (rng *IPv4AddressSeqRange) PrefixBlockIterator(prefLength BitCount) IPv4AddressIterator {
	return &ipv4AddressIterator{rng.init().prefixBlockIterator(prefLength)}
}

// PrefixIterator provides an iterator to iterate through the individual prefixes of the given prefix length in this address range,
// each iterated element spanning the range of values for its prefix.
//
// It is similar to the prefix block iterator, except for possibly the first and last iterated elements, which might not be prefix blocks,
// instead constraining themselves to values from this range.
func (rng *IPv4AddressSeqRange) PrefixIterator(prefLength BitCount) IPv4AddressSeqRangeIterator {
	return &ipv4RangeIterator{rng.init().prefixIterator(prefLength)}
}

// ToIP converts to an IPAddressSeqRange, a polymorphic type usable with all IP address sequential ranges.
//
// ToIP can be called with a nil receiver, enabling you to chain this method with methods that might return a nil pointer.
func (rng *IPv4AddressSeqRange) ToIP() *IPAddressSeqRange {
	if rng != nil {
		rng = rng.init()
	}
	return (*IPAddressSeqRange)(rng)
}

func (rng *IPv4AddressSeqRange) Overlaps(other *IPv4AddressSeqRange) bool {
	return rng.init().overlaps(other.ToIP())
}

func (rng *IPv4AddressSeqRange) Intersect(other *IPv4AddressSeqRange) *IPAddressSeqRange {
	return rng.init().intersect(other.toIPSequentialRange())
}

func (rng *IPv4AddressSeqRange) CoverWithPrefixBlock() *IPv4Address {
	return rng.GetLower().CoverWithPrefixBlockTo(rng.GetUpper())
}

func (rng *IPv4AddressSeqRange) SpanWithPrefixBlocks() []*IPv4Address {
	return rng.GetLower().SpanWithPrefixBlocksTo(rng.GetUpper())
}

func (rng *IPv4AddressSeqRange) SpanWithSequentialBlocks() []*IPv4Address {
	return rng.GetLower().SpanWithSequentialBlocksTo(rng.GetUpper())
}

// Joins the given ranges into the fewest number of ranges.
// The returned array will be sorted by ascending lowest range value.
func (rng *IPv4AddressSeqRange) Join(ranges ...*IPv4AddressSeqRange) []*IPv4AddressSeqRange {
	origLen := len(ranges)
	ranges2 := make([]*IPAddressSeqRange, 0, origLen+1)
	for _, rng := range ranges {
		ranges2 = append(ranges2, rng.ToIP())
	}
	ranges2 = append(ranges2, rng.ToIP())
	return cloneToIPv4SeqRange(join(ranges2))
}

// JoinTo joins this range to the other.  If this range overlaps with the given range,
// or if the highest value of the lower range is one below the lowest value of the higher range,
// then the two are joined into a new larger range that is returned.
// Otherwise nil is returned.
func (rng *IPv4AddressSeqRange) JoinTo(other *IPv4AddressSeqRange) *IPv4AddressSeqRange {
	return rng.init().joinTo(other.init().ToIP()).ToIPv4()
}

// Extend extends this sequential range to include all address in the given range.
// If the argument has a different IP version than this, nil is returned.
// Otherwise, this method returns the range that includes this range, the given range, and all addresses in-between.
func (rng *IPv4AddressSeqRange) Extend(other *IPv4AddressSeqRange) *IPv4AddressSeqRange {
	return rng.init().extend(other.init().ToIP()).ToIPv4()
}

// Subtract Subtracts the given range from this range, to produce either zero, one, or two address ranges that contain the addresses in this range and not in the given range.
// If the result has length 2, the two ranges are ordered by ascending lowest range value.
func (rng *IPv4AddressSeqRange) Subtract(other *IPv4AddressSeqRange) []*IPv4AddressSeqRange {
	return cloneToIPv4SeqRange(rng.init().subtract(other.init().ToIP()))
}

// GetIPv4Count is equivalent to GetCount() but returns a uint64
func (rng *IPv4AddressSeqRange) GetIPv4Count() uint64 {
	return uint64(rng.GetUpper().Uint32Value()-rng.GetLower().Uint32Value()) + 1
}

// GetIPv4PrefixCount is equivalent to GetPrefixCountLen(int) but returns a uint64
func (rng *IPv4AddressSeqRange) GetIPv4PrefixCount(prefixLength BitCount) uint64 {
	prefixLength = checkBitCount(prefixLength, IPv4BitCount)
	bitCount := IPv4BitCount
	if bitCount <= prefixLength {
		return rng.GetIPv4Count()
	}
	shiftAdjustment := bitCount - prefixLength
	upperAdjusted := rng.GetUpper().Uint32Value() >> uint(shiftAdjustment)
	lowerAdjusted := rng.GetLower().Uint32Value() >> uint(shiftAdjustment)
	return uint64(upperAdjusted-lowerAdjusted) + 1
}

func (rng *IPv4AddressSeqRange) ToKey() *IPv4AddressSeqRangeKey {
	return &IPv4AddressSeqRangeKey{
		lower: *rng.GetLower().ToKey(),
		upper: *rng.GetUpper().ToKey(),
	}
}
