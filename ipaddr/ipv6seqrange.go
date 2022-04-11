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

func NewIPv6SeqRange(one, two *IPv6Address) *IPv6AddressSeqRange {
	if one == nil && two == nil {
		one = zeroIPv6
	}
	one = one.WithoutZone()
	two = two.WithoutZone()
	return newSeqRange(one.ToIP(), two.ToIP()).ToIPv6()
}

var zeroIPv6Range = NewIPv6SeqRange(zeroIPv6, zeroIPv6)

type IPv6AddressSeqRange struct {
	ipAddressSeqRangeInternal
}

func (rng *IPv6AddressSeqRange) init() *IPv6AddressSeqRange {
	if rng.lower == nil {
		return zeroIPv6Range
	}
	return rng
}

// GetCount returns the count of addresses that this sequential range represents.
func (rng *IPv6AddressSeqRange) GetCount() *big.Int {
	if rng == nil {
		return bigZero()
	}
	return rng.init().getCount()
}

// IsMultiple returns whether this range represents a range of multiple addresses
func (rng *IPv6AddressSeqRange) IsMultiple() bool {
	return rng != nil && rng.isMultiple()
}

func (rng *IPv6AddressSeqRange) String() string {
	if rng == nil {
		return nilString()
	}
	return rng.ToString((*IPv6Address).String, DefaultSeqRangeSeparator, (*IPv6Address).String)
}

func (rng IPv6AddressSeqRange) Format(state fmt.State, verb rune) {
	rng.init().format(state, verb)
}

func (rng *IPv6AddressSeqRange) ToString(lowerStringer func(*IPv6Address) string, separator string, upperStringer func(*IPv6Address) string) string {
	if rng == nil {
		return nilString()
	}
	return rng.init().toString(
		func(addr *IPAddress) string {
			return lowerStringer(addr.ToIPv6())
		},
		separator,
		func(addr *IPAddress) string {
			return upperStringer(addr.ToIPv6())
		},
	)
}

func (rng *IPv6AddressSeqRange) ToNormalizedString() string {
	return rng.ToString((*IPv6Address).ToNormalizedString, DefaultSeqRangeSeparator, (*IPv6Address).ToNormalizedString)
}

func (rng *IPv6AddressSeqRange) ToCanonicalString() string {
	return rng.ToString((*IPv6Address).ToCanonicalString, DefaultSeqRangeSeparator, (*IPv6Address).ToNormalizedString)
}

// GetBitCount returns the number of bits in each address in the range, which is 16
func (rng *IPv6AddressSeqRange) GetBitCount() BitCount {
	return rng.GetLower().GetBitCount()
}

// GetByteCount returns the number of bytes in each address in the range, which is 2
func (rng *IPv6AddressSeqRange) GetByteCount() int {
	return rng.GetLower().GetByteCount()
}

func (rng *IPv6AddressSeqRange) GetLowerIPAddress() *IPAddress {
	return rng.init().lower
}

func (rng *IPv6AddressSeqRange) GetUpperIPAddress() *IPAddress {
	return rng.init().upper
}

func (rng *IPv6AddressSeqRange) GetLower() *IPv6Address {
	return rng.init().lower.ToIPv6()
}

func (rng *IPv6AddressSeqRange) GetUpper() *IPv6Address {
	return rng.init().upper.ToIPv6()
}

func (rng *IPv6AddressSeqRange) GetNetIP() net.IP {
	return rng.GetLower().GetNetIP()
}

func (rng *IPv6AddressSeqRange) CopyNetIP(bytes net.IP) net.IP {
	return rng.GetLower().CopyNetIP(bytes)
}

func (rng *IPv6AddressSeqRange) GetUpperNetIP() net.IP {
	return rng.GetUpper().GetUpperNetIP()
}

func (rng *IPv6AddressSeqRange) CopyUpperNetIP(bytes net.IP) net.IP {
	return rng.GetUpper().CopyUpperNetIP(bytes)
}

func (rng *IPv6AddressSeqRange) Bytes() []byte {
	return rng.GetLower().Bytes()
}

func (rng *IPv6AddressSeqRange) CopyBytes(bytes []byte) []byte {
	return rng.GetLower().CopyBytes(bytes)
}

func (rng *IPv6AddressSeqRange) UpperBytes() []byte {
	return rng.GetUpper().UpperBytes()
}

func (rng *IPv6AddressSeqRange) CopyUpperBytes(bytes []byte) []byte {
	return rng.GetUpper().CopyUpperBytes(bytes)
}

func (rng *IPv6AddressSeqRange) GetValue() *big.Int {
	return rng.GetLower().GetValue()
}

func (rng *IPv6AddressSeqRange) GetUpperValue() *big.Int {
	return rng.GetUpper().GetValue()
}

func (rng *IPv6AddressSeqRange) Contains(other IPAddressType) bool {
	if rng == nil {
		return other == nil || other.ToAddressBase() == nil
	}
	return rng.init().contains(other)
}

func (rng *IPv6AddressSeqRange) ContainsRange(other IPAddressSeqRangeType) bool {
	if rng == nil {
		return other == nil || other.ToIP() == nil
	}
	return rng.init().containsRange(other)
}

func (rng *IPv6AddressSeqRange) Equal(other IPAddressSeqRangeType) bool {
	if rng == nil {
		return other == nil || other.ToIP() == nil
	}
	return rng.init().equals(other)
}

func (rng *IPv6AddressSeqRange) Compare(item AddressItem) int {
	if rng != nil {
		rng = rng.init()
	}
	return CountComparator.Compare(rng, item)
}

func (rng *IPv6AddressSeqRange) CompareSize(other IPAddressSeqRangeType) int {
	if rng == nil {
		if other != nil && other.ToIP() != nil {
			// we have size 0, other has size >= 1
			return -1
		}
		return 0
	}
	return rng.compareSize(other)
}

func (rng *IPv6AddressSeqRange) ContainsPrefixBlock(prefixLen BitCount) bool {
	return rng.init().ipAddressSeqRangeInternal.ContainsPrefixBlock(prefixLen)
}

func (rng *IPv6AddressSeqRange) ContainsSinglePrefixBlock(prefixLen BitCount) bool {
	return rng.init().ipAddressSeqRangeInternal.ContainsSinglePrefixBlock(prefixLen)
}

func (rng *IPv6AddressSeqRange) GetPrefixLenForSingleBlock() PrefixLen {
	return rng.init().ipAddressSeqRangeInternal.GetPrefixLenForSingleBlock()
}

func (rng *IPv6AddressSeqRange) GetMinPrefixLenForBlock() BitCount {
	return rng.init().ipAddressSeqRangeInternal.GetMinPrefixLenForBlock()
}

func (rng *IPv6AddressSeqRange) Iterator() IPv6AddressIterator {
	if rng == nil {
		return ipv6AddressIterator{nilAddrIterator()}
	}
	return ipv6AddressIterator{rng.init().iterator()}
}

func (rng *IPv6AddressSeqRange) PrefixBlockIterator(prefLength BitCount) IPv6AddressIterator {
	return &ipv6AddressIterator{rng.init().prefixBlockIterator(prefLength)}
}

func (rng *IPv6AddressSeqRange) PrefixIterator(prefLength BitCount) IPv6AddressSeqRangeIterator {
	return &ipv6RangeIterator{rng.init().prefixIterator(prefLength)}
}

func (rng *IPv6AddressSeqRange) ToIP() *IPAddressSeqRange {
	if rng != nil {
		rng = rng.init()
	}
	return (*IPAddressSeqRange)(rng)
}

func (rng *IPv6AddressSeqRange) Overlaps(other *IPv6AddressSeqRange) bool {
	return rng.init().overlaps(other.ToIP())
}

func (rng *IPv6AddressSeqRange) Intersect(other *IPv6AddressSeqRange) *IPAddressSeqRange {
	return rng.init().intersect(other.toIPSequentialRange())
}

func (rng *IPv6AddressSeqRange) CoverWithPrefixBlock() *IPv6Address {
	return rng.GetLower().CoverWithPrefixBlockTo(rng.GetUpper())
}

func (rng *IPv6AddressSeqRange) SpanWithPrefixBlocks() []*IPv6Address {
	return rng.GetLower().SpanWithPrefixBlocksTo(rng.GetUpper())
}

func (rng *IPv6AddressSeqRange) SpanWithSequentialBlocks() []*IPv6Address {
	return rng.GetLower().SpanWithSequentialBlocksTo(rng.GetUpper())
}

// Join joins the given ranges into the fewest number of ranges.
// The returned array will be sorted by ascending lowest range value.
func (rng *IPv6AddressSeqRange) Join(ranges ...*IPv6AddressSeqRange) []*IPv6AddressSeqRange {
	origLen := len(ranges)
	ranges2 := make([]*IPAddressSeqRange, 0, origLen+1)
	for _, rng := range ranges {
		ranges2 = append(ranges2, rng.ToIP())
	}
	ranges2 = append(ranges2, rng.ToIP())
	return cloneToIPv6SeqRange(join(ranges2))
}

// JoinTo joins this range to the other.  If this range overlaps with the given range,
// or if the highest value of the lower range is one below the lowest value of the higher range,
// then the two are joined into a new larger range that is returned.
// Otherwise nil is returned.
func (rng *IPv6AddressSeqRange) JoinTo(other *IPv6AddressSeqRange) *IPv6AddressSeqRange {
	return rng.init().joinTo(other.init().ToIP()).ToIPv6()
}

// Extend extends this sequential range to include all address in the given range.
// If the argument has a different IP version than this, nil is returned.
// Otherwise, this method returns the range that includes this range, the given range, and all addresses in-between.
func (rng *IPv6AddressSeqRange) Extend(other *IPv6AddressSeqRange) *IPv6AddressSeqRange {
	return rng.init().extend(other.init().ToIP()).ToIPv6()
}

// Subtract Subtracts the given range from this range, to produce either zero, one, or two address ranges that contain the addresses in this range and not in the given range.
// If the result has length 2, the two ranges are ordered by ascending lowest range value.
func (rng *IPv6AddressSeqRange) Subtract(other *IPv6AddressSeqRange) []*IPv6AddressSeqRange {
	return cloneToIPv6SeqRange(rng.init().subtract(other.init().ToIP()))
}

func (rng *IPv6AddressSeqRange) ToKey() *IPv6AddressSeqRangeKey {
	return &IPv6AddressSeqRangeKey{
		lower: *rng.GetLower().ToKey(),
		upper: *rng.GetUpper().ToKey(),
	}
}
