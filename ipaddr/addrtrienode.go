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

package ipaddr

import (
	"fmt"
	"github.com/seancfoley/bintree/tree"
	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
	"unsafe"
)

// TrieKeyConstraint is the generic type constraint used for tree keys, which are individual addresses and prefix block subnets.
type TrieKeyConstraint[T any] interface {
	comparable

	BitItem

	fmt.Stringer

	PrefixedConstraint[T]

	IsOneBit(index BitCount) bool // AddressComponent

	ToAddressBase() *Address // AddressType - used by MatchBits

	toMaxLower() T
	toMinUpper() T
	trieCompare(other *Address) int
	getTrailingBitCount(ones bool) BitCount
	toSinglePrefixBlockOrAddress() (T, addrerr.IncompatibleAddressError)
}

type trieKey[T TrieKeyConstraint[T]] struct {
	address T
}

func (a trieKey[T]) GetBitCount() tree.BitCount {
	return a.address.GetBitCount()
}

func (a trieKey[T]) String() string {
	return a.address.String()
}

func (a trieKey[T]) IsOneBit(bitIndex tree.BitCount) bool {
	return a.address.IsOneBit(bitIndex)
}

func (a trieKey[T]) GetTrailingBitCount(ones bool) tree.BitCount {
	return a.address.getTrailingBitCount(ones)
}

func (a trieKey[T]) GetPrefixLen() tree.PrefixLen {
	return tree.PrefixLen(a.address.GetPrefixLen())
}

// ToPrefixBlockLen returns the address key associated with the prefix length provided,
// the address key whose prefix of that length matches the prefix of this address key, and the remaining bits span all values.
//
// The returned address key will represent all addresses with the same prefix as this one, the prefix "block".
func (a trieKey[T]) ToPrefixBlockLen(bitCount BitCount) trieKey[T] {
	return trieKey[T]{a.address.ToPrefixBlockLen(bitCount)}
}

// Compare compares to provide the same ordering used by the trie,
// an ordering that works with prefix block subnets and individual addresses.
// The comparator is consistent with the equality of address instances
// and can be used in other contexts.  However, it only works with prefix blocks and individual addresses,
// not with addresses like 1-2.3.4.5-6 which cannot be differentiated using this comparator from 1.3.4.5
// and is thus not consistent with equality for subnets that are not CIDR prefix blocks.
//
// The comparator first compares the prefix of addresses, with the full address value considered the prefix when
// there is no prefix length, ie when it is a single address.  It takes the minimum m of the two prefix lengths and
// compares those m prefix bits in both addresses.  The ordering is determined by which of those two values is smaller or larger.
//
// If both prefix lengths match then both addresses are equal.
// Otherwise it looks at bit m in the address with larger prefix.  If 1 it is larger and if 0 it is smaller than the other.
//
// When comparing an address with a prefix p and an address without, the first p bits in both are compared, and if equal,
// the bit at index p in the non-prefixed address determines the ordering, if 1 it is larger and if 0 it is smaller than the other.
//
// When comparing an address with prefix length matching the bit count to an address with no prefix, they are considered equal if the bits match.
// For instance, 1.2.3.4/32 is equal to 1.2.3.4, and thus the trie does not allow 1.2.3.4/32 in the trie since it is indistinguishable from 1.2.3.4,
// instead 1.2.3.4/32 is converted to 1.2.3.4 when inserted into the trie.
//
// When comparing 0.0.0.0/0, which has no prefix, to other addresses, the first bit in the other address determines the ordering.
// If 1 it is larger and if 0 it is smaller than 0.0.0.0/0.
func (a trieKey[T]) Compare(other trieKey[T]) int {
	return a.address.trieCompare(other.address.ToAddressBase())
}

// MatchBits returns false if we need to keep going and try to match sub-nodes.
// MatchBits returns true if the bits do not match, or the bits match to the very end.
func (a trieKey[T]) MatchBits(key trieKey[T], bitIndex int, handleMatch tree.KeyCompareResult) bool {
	existingAddr := key.address.ToAddressBase()
	bitsPerSegment := existingAddr.GetBitsPerSegment()
	bytesPerSegment := existingAddr.GetBytesPerSegment()
	newAddr := a.address.ToAddressBase()
	segmentIndex := getHostSegmentIndex(bitIndex, bytesPerSegment, bitsPerSegment)
	segmentCount := existingAddr.GetSegmentCount()
	// the caller already checks total bits, so we only need to check either bitsPerSegment or segmentCount, but not both
	if /* newAddr.GetSegmentCount() != segmentCount || */ bitsPerSegment != newAddr.GetBitsPerSegment() {
		panic("mismatched bit length between address trie keys")
	}
	existingPref := existingAddr.GetPrefixLen()
	newPrefLen := newAddr.GetPrefixLen()

	// this block handles cases like matching ::ffff:102:304 to ::ffff:102:304/127,
	// and we found a subnode to match, but we know the final bit is a match due to the subnode being lower or upper,
	// so there is actually not more bits to match
	if segmentIndex >= segmentCount {
		// all the bits match
		handleMatch.BitsMatch()
		return true
	}

	bitsMatchedSoFar := segmentIndex * bitsPerSegment
	for {
		existingSegment := existingAddr.getSegment(segmentIndex)
		newSegment := newAddr.getSegment(segmentIndex)
		segmentPref := getSegmentPrefLen(existingAddr, existingPref, bitsPerSegment, bitsMatchedSoFar, existingSegment)
		newSegmentPref := getSegmentPrefLen(newAddr, newPrefLen, bitsPerSegment, bitsMatchedSoFar, newSegment)
		if segmentPref != nil {
			segmentPrefLen := segmentPref.Len()
			newPrefixLen := newSegmentPref.Len()
			if newSegmentPref != nil && newPrefixLen <= segmentPrefLen {
				matchingBits := getMatchingBits(existingSegment, newSegment, newPrefixLen, bitsPerSegment)
				if matchingBits >= newPrefixLen {
					handleMatch.BitsMatch()
				} else {
					// no match - the bits don't match
					// matchingBits < newPrefLen < segmentPrefLen
					handleMatch.BitsDoNotMatch(bitsMatchedSoFar + matchingBits)
				}
			} else {
				matchingBits := getMatchingBits(existingSegment, newSegment, segmentPrefLen, bitsPerSegment)
				if matchingBits >= segmentPrefLen { // match - the current subnet/address is a match so far, and we must go further to check smaller subnets
					return false
				}
				// matchingBits < segmentPrefLen - no match - the bits in current prefix do not match the prefix of the existing address
				handleMatch.BitsDoNotMatch(bitsMatchedSoFar + matchingBits)
			}
			return true
		} else if newSegmentPref != nil {
			newPrefixLen := newSegmentPref.Len()
			matchingBits := getMatchingBits(existingSegment, newSegment, newPrefixLen, bitsPerSegment)
			if matchingBits >= newPrefixLen { // the current bits match the current prefix, but the existing has no prefix
				handleMatch.BitsMatch()
			} else {
				// no match - the current subnet does not match the existing address
				handleMatch.BitsDoNotMatch(bitsMatchedSoFar + matchingBits)
			}
			return true
		} else {
			matchingBits := getMatchingBits(existingSegment, newSegment, bitsPerSegment, bitsPerSegment)
			if matchingBits < bitsPerSegment { // no match - the current subnet/address is not here
				handleMatch.BitsDoNotMatch(bitsMatchedSoFar + matchingBits)
				return true
			} else {
				segmentIndex++
				if segmentIndex == segmentCount { // match - the current subnet/address is a match
					// note that "added" is already true here, we can only be here if explicitly inserted already since it is a non-prefixed full address
					handleMatch.BitsMatch()
					return true
				}
			}
			bitsMatchedSoFar += bitsPerSegment
		}
	}
}

// ToMaxLower changes this key to a new key with a 0 at the first bit beyond the prefix, followed by all ones, and with no prefix length.
func (a trieKey[T]) ToMaxLower() trieKey[T] {
	return trieKey[T]{a.address.toMaxLower()}
}

// ToMinUpper changes this key to a new key with a 1 at the first bit beyond the prefix, followed by all zeros, and with no prefix length.
func (a trieKey[T]) ToMinUpper() trieKey[T] {
	return trieKey[T]{a.address.toMinUpper()}
}

var (
	_ tree.BinTrieNode[trieKey[*Address], any]
	_ tree.BinTrieNode[trieKey[*IPAddress], any]
	_ tree.BinTrieNode[trieKey[*IPv4Address], any]
	_ tree.BinTrieNode[trieKey[*IPv6Address], any]
	_ tree.BinTrieNode[trieKey[*MACAddress], any]
)

//
//
//
//
type trieNode[T TrieKeyConstraint[T], V any] struct {
	binNode tree.BinTrieNode[trieKey[T], V]
}

// getKey gets the key used for placing the node in the trie.
func (node *trieNode[T, V]) getKey() (t T) {
	return node.toBinTrieNode().GetKey().address
}

func (node *trieNode[T, V]) get(addr T) (V, bool) {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().Get(trieKey[T]{addr})
}

func (node *trieNode[T, V]) lowerAddedNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().LowerAddedNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) floorAddedNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().FloorAddedNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) higherAddedNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().HigherAddedNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) ceilingAddedNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().CeilingAddedNode(trieKey[T]{addr})
}

// iterator returns an iterator that iterates through the elements of the sub-trie with this node as the root.
// The iteration is in sorted element order.
func (node *trieNode[T, V]) iterator() Iterator[T] {
	return addressKeyIterator[T]{node.toBinTrieNode().Iterator()}
}

// descendingIterator returns an iterator that iterates through the elements of the subtrie with this node as the root.
// The iteration is in reverse sorted element order.
func (node *trieNode[T, V]) descendingIterator() Iterator[T] {
	return addressKeyIterator[T]{node.toBinTrieNode().DescendingIterator()}
}

// nodeIterator iterates through the added nodes of the sub-trie with this node as the root, in forward or reverse tree order.
func (node *trieNode[T, V]) nodeIterator(forward bool) tree.TrieNodeIteratorRem[trieKey[T], V] {
	return node.toBinTrieNode().NodeIterator(forward)
}

// allNodeIterator iterates through all the nodes of the sub-trie with this node as the root, in forward or reverse tree order.
func (node *trieNode[T, V]) allNodeIterator(forward bool) tree.TrieNodeIteratorRem[trieKey[T], V] {
	return node.toBinTrieNode().AllNodeIterator(forward)
}

// blockSizeNodeIterator iterates the added nodes, ordered by keys from the largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order is taken.
func (node *trieNode[T, V]) blockSizeNodeIterator(lowerSubNodeFirst bool) tree.TrieNodeIteratorRem[trieKey[T], V] {
	return node.toBinTrieNode().BlockSizeNodeIterator(lowerSubNodeFirst)
}

// blockSizeAllNodeIterator iterates all the nodes, ordered by keys from the largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order
func (node *trieNode[T, V]) blockSizeAllNodeIterator(lowerSubNodeFirst bool) tree.TrieNodeIteratorRem[trieKey[T], V] {
	return node.toBinTrieNode().BlockSizeAllNodeIterator(lowerSubNodeFirst)
}

// blockSizeCachingAllNodeIterator iterates all nodes, ordered by keys from the largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
func (node *trieNode[T, V]) blockSizeCachingAllNodeIterator() tree.CachingTrieNodeIterator[trieKey[T], V] {
	return node.toBinTrieNode().BlockSizeCachingAllNodeIterator()
}

func (node *trieNode[T, V]) containingFirstIterator(forwardSubNodeOrder bool) tree.CachingTrieNodeIterator[trieKey[T], V] {
	return node.toBinTrieNode().ContainingFirstIterator(forwardSubNodeOrder)
}

func (node *trieNode[T, V]) containingFirstAllNodeIterator(forwardSubNodeOrder bool) tree.CachingTrieNodeIterator[trieKey[T], V] {
	return node.toBinTrieNode().ContainingFirstAllNodeIterator(forwardSubNodeOrder)
}

func (node *trieNode[T, V]) containedFirstIterator(forwardSubNodeOrder bool) tree.TrieNodeIteratorRem[trieKey[T], V] {
	return node.toBinTrieNode().ContainedFirstIterator(forwardSubNodeOrder)
}

func (node *trieNode[T, V]) containedFirstAllNodeIterator(forwardSubNodeOrder bool) tree.TrieNodeIterator[trieKey[T], V] {
	return node.toBinTrieNode().ContainedFirstAllNodeIterator(forwardSubNodeOrder)
}

func (node *trieNode[T, V]) contains(addr T) bool {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().Contains(trieKey[T]{addr})
}

func (node *trieNode[T, V]) removeNode(addr T) bool {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().RemoveNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) removeElementsContainedBy(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().RemoveElementsContainedBy(trieKey[T]{addr})
}

func (node *trieNode[T, V]) elementsContainedBy(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().ElementsContainedBy(trieKey[T]{addr})
}

func (node *trieNode[T, V]) elementsContaining(addr T) *containmentPath[T, V] {
	addr = mustBeBlockOrAddress(addr)
	return toContainmentPath[T, V](node.toBinTrieNode().ElementsContaining(trieKey[T]{addr}))
}

func (node *trieNode[T, V]) longestPrefixMatch(addr T) (t T) {
	addr = mustBeBlockOrAddress(addr)
	key, _ := node.toBinTrieNode().LongestPrefixMatch(trieKey[T]{addr})
	return key.address
}

func (node *trieNode[T, V]) longestPrefixMatchNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().LongestPrefixMatchNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) elementContains(addr T) bool {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().ElementContains(trieKey[T]{addr})
}

func (node *trieNode[T, V]) getNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().GetNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) getAddedNode(addr T) *tree.BinTrieNode[trieKey[T], V] {
	addr = mustBeBlockOrAddress(addr)
	return node.toBinTrieNode().GetAddedNode(trieKey[T]{addr})
}

func (node *trieNode[T, V]) toBinTrieNode() *tree.BinTrieNode[trieKey[T], V] {
	return (*tree.BinTrieNode[trieKey[T], V])(unsafe.Pointer(node))
}

func toAddressTrieNode[T TrieKeyConstraint[T], V any](node *tree.BinTrieNode[trieKey[T], V]) *TrieNode[T] {
	return (*TrieNode[T])(unsafe.Pointer(node))
}

func toAssociativeTrieNode[T TrieKeyConstraint[T], V any](node *tree.BinTrieNode[trieKey[T], V]) *AssociativeTrieNode[T, V] {
	return (*AssociativeTrieNode[T, V])(unsafe.Pointer(node))
}

//
//
//
//
//
//
//
//
//
//
//

// using EmptyValueType alters how values are printed in strings
type emptyValue = tree.EmptyValueType

// TrieNode is a node in a compact binary prefix trie whose elements (keys) are prefix block subnets or addresses.
type TrieNode[T TrieKeyConstraint[T]] struct {
	trieNode[T, emptyValue]
}

func (node *TrieNode[T]) toBinTrieNode() *tree.BinTrieNode[trieKey[T], emptyValue] {
	return (*tree.BinTrieNode[trieKey[T], emptyValue])(unsafe.Pointer(node))
}

// tobase is used to convert the pointer rather than doing a field dereference, so that nil pointer handling can be done in *addressTrieNode
func (node *TrieNode[T]) tobase() *trieNode[T, emptyValue] {
	return (*trieNode[T, emptyValue])(unsafe.Pointer(node))
}

// GetKey gets the key used to place the node in the trie.
func (node *TrieNode[T]) GetKey() T {
	return node.tobase().getKey()
}

// IsRoot returns whether this node is the root of the trie.
func (node *TrieNode[T]) IsRoot() bool {
	return node.toBinTrieNode().IsRoot()
}

// IsAdded returns whether the node was "added".
// Some binary trie nodes are considered "added" and others are not.
// Those nodes created for key elements added to the trie are "added" nodes.
// Those that are not added are those nodes created to serve as junctions for the added nodes.
// Only added elements contribute to the size of a trie.
// When removing nodes, non-added nodes are removed automatically whenever they are no longer needed,
// which is when an added node has less than two added sub-nodes.
func (node *TrieNode[T]) IsAdded() bool {
	return node.toBinTrieNode().IsAdded()
}

// SetAdded makes this node an added node, which is equivalent to adding the corresponding key to the trie.
// If the node is already an added node, this method has no effect.
// You cannot set an added node to non-added, for that you should Remove the node from the trie by calling Remove.
// A non-added node will only remain in the trie if it needs to be in the trie.
func (node *TrieNode[T]) SetAdded() {
	node.toBinTrieNode().SetAdded()
}

// Clear removes this node and all sub-nodes from the trie, after which isEmpty will return true.
func (node *TrieNode[T]) Clear() {
	node.toBinTrieNode().Clear()
}

// IsLeaf returns whether this node is in the trie (a node for which IsAdded is true)
// and there are no elements in the sub-trie with this node as the root.
func (node *TrieNode[T]) IsLeaf() bool {
	return node.toBinTrieNode().IsLeaf()
}

// GetUpperSubNode gets the direct child node whose key is largest in value.
func (node *TrieNode[T]) GetUpperSubNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().GetUpperSubNode())
}

// GetLowerSubNode gets the direct child node whose key is smallest in value.
func (node *TrieNode[T]) GetLowerSubNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().GetLowerSubNode())
}

// GetParent gets the node from which this node is a direct child node, or nil if this is the root.
func (node *TrieNode[T]) GetParent() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().GetParent())
}

// PreviousAddedNode returns the previous node in the trie that is an added node, following the trie order in reverse,
// or nil if there is no such node.
func (node *TrieNode[T]) PreviousAddedNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().PreviousAddedNode())
}

// NextAddedNode returns the next node in the trie that is an added node, following the trie order,
// or nil if there is no such node.
func (node *TrieNode[T]) NextAddedNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().NextAddedNode())
}

// NextNode returns the node that follows this node following the trie order.
func (node *TrieNode[T]) NextNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().NextNode())
}

// PreviousNode eturns the node that precedes this node following the trie order.
func (node *TrieNode[T]) PreviousNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().PreviousNode())
}

// FirstNode returns the first (the lowest valued) node in the sub-trie originating from this node.
func (node *TrieNode[T]) FirstNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().FirstNode())
}

// FirstAddedNode returns the first (the lowest valued) added node in the sub-trie originating from this node,
// or nil if there are no added entries in this trie or sub-trie.
func (node *TrieNode[T]) FirstAddedNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().FirstAddedNode())
}

// LastNode returns the last (the highest valued) node in the sub-trie originating from this node.
func (node *TrieNode[T]) LastNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().LastNode())
}

// LastAddedNode returns the last (the highest valued) added node in the sub-trie originating from this node,
// or nil if there are no added entries in this trie or sub-trie.
func (node *TrieNode[T]) LastAddedNode() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().LastAddedNode())
}

// LowerAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the highest address strictly less than the given address.
func (node *TrieNode[T]) LowerAddedNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().lowerAddedNode(addr))
}

// FloorAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the highest address less than or equal to the given address.
func (node *TrieNode[T]) FloorAddedNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().floorAddedNode(addr))
}

// HigherAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the lowest address strictly greater than the given address.
func (node *TrieNode[T]) HigherAddedNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().higherAddedNode(addr))
}

// CeilingAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the lowest address greater than or equal to the given address.
func (node *TrieNode[T]) CeilingAddedNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().ceilingAddedNode(addr))
}

// Iterator returns an iterator that iterates through the elements of the sub-trie with this node as the root.
// The iteration is in sorted element order.
func (node *TrieNode[T]) Iterator() Iterator[T] {
	return node.tobase().iterator()
}

// DescendingIterator returns an iterator that iterates through the elements of the subtrie with this node as the root.
// The iteration is in reverse sorted element order.
func (node *TrieNode[T]) DescendingIterator() Iterator[T] {
	return node.tobase().descendingIterator()
}

// NodeIterator returns an iterator that iterates through the added nodes of the sub-trie with this node as the root, in forward or reverse trie order.
func (node *TrieNode[T]) NodeIterator(forward bool) IteratorWithRemove[*TrieNode[T]] {
	return addrTrieNodeIteratorRem[T, emptyValue]{node.tobase().nodeIterator(forward)}
}

// AllNodeIterator returns an iterator that iterates through all the nodes of the sub-trie with this node as the root, in forward or reverse trie order.
func (node *TrieNode[T]) AllNodeIterator(forward bool) IteratorWithRemove[*TrieNode[T]] {
	return addrTrieNodeIteratorRem[T, emptyValue]{node.tobase().allNodeIterator(forward)}
}

// BlockSizeNodeIterator returns an iterator that iterates the added nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order is taken.
func (node *TrieNode[T]) BlockSizeNodeIterator(lowerSubNodeFirst bool) IteratorWithRemove[*TrieNode[T]] {
	return addrTrieNodeIteratorRem[T, emptyValue]{node.tobase().blockSizeNodeIterator(lowerSubNodeFirst)}
}

// BlockSizeAllNodeIterator returns an iterator that iterates all the nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order.
func (node *TrieNode[T]) BlockSizeAllNodeIterator(lowerSubNodeFirst bool) IteratorWithRemove[*TrieNode[T]] {
	return addrTrieNodeIteratorRem[T, emptyValue]{node.tobase().blockSizeAllNodeIterator(lowerSubNodeFirst)}
}

// BlockSizeCachingAllNodeIterator returns an iterator that iterates all nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
func (node *TrieNode[T]) BlockSizeCachingAllNodeIterator() CachingTrieIterator[*TrieNode[T]] {
	return cachingAddressTrieNodeIterator[T, emptyValue]{node.tobase().blockSizeCachingAllNodeIterator()}
}

// ContainingFirstIterator returns an iterator that does a pre-order binary trie traversal of the added nodes
// of the sub-trie with this node as the root.
//
// All added nodes will be visited before their added sub-nodes.
// For an address trie this means added containing subnet blocks will be visited before their added contained addresses and subnet blocks.
//
// Once a given node is visited, the iterator allows you to cache an object corresponding to the
// lower or upper sub-node that can be retrieved when you later visit that sub-node.
//
// Objects are cached only with nodes to be visited.
// So for this iterator that means an object will be cached with the first added lower or upper sub-node,
// the next lower or upper sub-node to be visited,
// which is not necessarily the direct lower or upper sub-node of a given node.
//
// The caching allows you to provide iteration context from a parent to its sub-nodes when iterating.
// The caching and retrieval is done in constant-time.
func (node *TrieNode[T]) ContainingFirstIterator(forwardSubNodeOrder bool) CachingTrieIterator[*TrieNode[T]] {
	return cachingAddressTrieNodeIterator[T, emptyValue]{node.tobase().containingFirstIterator(forwardSubNodeOrder)}
}

// ContainingFirstAllNodeIterator returns an iterator that does a pre-order binary trie traversal of all the nodes
// of the sub-trie with this node as the root.
//
// All nodes will be visited before their sub-nodes.
// For an address trie this means containing subnet blocks will be visited before their contained addresses and subnet blocks.
//
// Once a given node is visited, the iterator allows you to cache an object corresponding to the
// lower or upper sub-node that can be retrieved when you later visit that sub-node.
// That allows you to provide iteration context from a parent to its sub-nodes when iterating.
// The caching and retrieval is done in constant-time.
func (node *TrieNode[T]) ContainingFirstAllNodeIterator(forwardSubNodeOrder bool) CachingTrieIterator[*TrieNode[T]] {
	return cachingAddressTrieNodeIterator[T, emptyValue]{node.tobase().containingFirstAllNodeIterator(forwardSubNodeOrder)}
}

// ContainedFirstIterator returns an iterator that does a post-order binary trie traversal of the added nodes
// of the sub-trie with this node as the root.
// All added sub-nodes will be visited before their parent nodes.
// For an address trie this means contained addresses and subnets will be visited before their containing subnet blocks.
func (node *TrieNode[T]) ContainedFirstIterator(forwardSubNodeOrder bool) IteratorWithRemove[*TrieNode[T]] {
	return addrTrieNodeIteratorRem[T, emptyValue]{node.tobase().containedFirstIterator(forwardSubNodeOrder)}
}

// ContainedFirstAllNodeIterator returns an iterator that does a post-order binary trie traversal of all the nodes
// of the sub-trie with this node as the root.
// All sub-nodes will be visited before their parent nodes.
// For an address trie this means contained addresses and subnets will be visited before their containing subnet blocks.
func (node *TrieNode[T]) ContainedFirstAllNodeIterator(forwardSubNodeOrder bool) Iterator[*TrieNode[T]] {
	return addrTrieNodeIterator[T, emptyValue]{node.tobase().containedFirstAllNodeIterator(forwardSubNodeOrder)}
}

// Clone clones the node.
// Keys remain the same, but the parent node and the lower and upper sub-nodes are all set to nil.
func (node *TrieNode[T]) Clone() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().Clone())
}

// CloneTree clones the sub-trie starting with this node as the root.
// The nodes are cloned, but their keys and values are not cloned.
func (node *TrieNode[T]) CloneTree() *TrieNode[T] {
	return toAddressTrieNode[T](node.toBinTrieNode().CloneTree())
}

// AsNewTrie creates a new sub-trie, copying the nodes starting with this node as the root.
// The nodes are copies of the nodes in this sub-trie, but their keys and values are not copies.
func (node *TrieNode[T]) AsNewTrie() *Trie[T] {
	return toAddressTrie[T](node.toBinTrieNode().AsNewTrie())
}

// Compare returns a negative integer, zero, or a positive integer if this node is less than, equal, or greater than the other, according to the key and the trie order.
func (node *TrieNode[T]) Compare(other *TrieNode[T]) int {
	return node.toBinTrieNode().Compare(other.toBinTrieNode())
}

// Equal returns whether the address and and mapped value match those of the given node.
func (node *TrieNode[T]) Equal(other *TrieNode[T]) bool {
	return node.toBinTrieNode().Equal(other.toBinTrieNode())
}

// TreeEqual returns whether the sub-tree represented by this node as the root node matches the given sub-trie.
func (node *TrieNode[T]) TreeEqual(other *TrieNode[T]) bool {
	return node.toBinTrieNode().TreeEqual(other.toBinTrieNode())
}

// Remove removes this node from the collection of added nodes, and also from the trie if possible.
// If it has two sub-nodes, it cannot be removed from the trie, in which case it is marked as not "added",
// nor is it counted in the trie size.
// Only added nodes can be removed from the trie.  If this node is not added, this method does nothing.
func (node *TrieNode[T]) Remove() {
	node.toBinTrieNode().Remove()
}

// Contains returns whether the given address or prefix block subnet is in the sub-trie, as an added element, with this node as the root.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the prefix block or address address exists already in the trie, false otherwise.
//
// Use GetAddedNode  to get the node for the address rather than just checking for its existence.
func (node *TrieNode[T]) Contains(addr T) bool {
	return node.tobase().contains(addr)
}

// RemoveNode removes the given single address or prefix block subnet from the trie with this node as the root.
//
// Removing an element will not remove contained elements (nodes for contained blocks and addresses).
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the prefix block or address was removed, false if not already in the trie.
//
// You can also remove by calling GetAddedNode to get the node and then calling Remove on the node.
//
// When an address is removed, the corresponding node may remain in the trie if it remains a subnet block for two sub-nodes.
// If the corresponding node can be removed from the trie, it will be removed.
func (node *TrieNode[T]) RemoveNode(addr T) bool {
	return node.tobase().removeNode(addr)
}

// RemoveElementsContainedBy removes any single address or prefix block subnet from the trie, with this node as the root, that is contained in the given individual address or prefix block subnet.
//
// Goes further than Remove, not requiring a match to an inserted node, and also removing all the sub-nodes of any removed node or sub-node.
//
// For example, after inserting 1.2.3.0 and 1.2.3.1, passing 1.2.3.0/31 to RemoveElementsContainedBy will remove them both,
// while the Remove method will remove nothing.
// After inserting 1.2.3.0/31, then Remove(Address) will remove 1.2.3.0/31, but will leave 1.2.3.0 and 1.2.3.1 in the trie.
//
// It cannot partially delete a node, such as deleting a single address from a prefix block represented by a node.
// It can only delete the whole node if the whole address or block represented by that node is contained in the given address or block.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
//Returns the root node of the subtrie that was removed from the trie, or nil if nothing was removed.
func (node *TrieNode[T]) RemoveElementsContainedBy(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().removeElementsContainedBy(addr))
}

// ElementsContainedBy checks if a part of this trie, with this node as the root, is contained by the given prefix block subnet or individual address.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns the root node of the contained subtrie, or nil if no subtrie is contained.
// The node returned need not be an "added" node, see IsAdded for more details on added nodes.
// The returned subtrie is backed by this trie, so changes in this trie are reflected in those nodes and vice-versa.
func (node *TrieNode[T]) ElementsContainedBy(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().elementsContainedBy(addr))
}

// ElementsContaining finds the trie nodes in the trie, with this sub-node as the root,
// containing the given key and returns them as a linked list.
// Only added nodes are added to the linked list
//
// If the argument is not a single address nor prefix block, this method will panic.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
func (node *TrieNode[T]) ElementsContaining(addr T) *ContainmentPath[T] {
	return &ContainmentPath[T]{*node.tobase().elementsContaining(addr)}
}

// LongestPrefixMatch returns the address or subnet with the longest prefix of all the added subnets and addresses whose prefix matches the given address.
// This is equivalent to finding the containing subnet or address with the smallest subnet size.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// The second returned argument is false if no added subnet or address contains the given argument.
//
// Use ElementContains to check for the existence of a containing address.
// To get all the containing addresses (subnets with matching prefix), use ElementsContaining.
// To get the node corresponding to the result of this method, use LongestPrefixMatchNode.
func (node *TrieNode[T]) LongestPrefixMatch(addr T) T {
	return node.tobase().longestPrefixMatch(addr)
}

// LongestPrefixMatchNode finds the containing subnet or address in the trie with the smallest subnet size,
// which is equivalent to finding the subnet or address with the longest matching prefix.
// Returns the node corresponding to that subnet.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns nil if no added subnet or address contains the given argument.
//
// Use ElementContains to check for the existence of a containing address.
// To get all the containing addresses, use ElementsContaining.
// Use LongestPrefixMatch to get only the address corresponding to the result of this method.
func (node *TrieNode[T]) LongestPrefixMatchNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().longestPrefixMatchNode(addr))
}

// ElementContains checks if a prefix block subnet or address in the trie, with this node as the root, contains the given subnet or address.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the subnet or address is contained by a trie element, false otherwise.
//
// To get all the containing addresses, use ElementsContaining.
func (node *TrieNode[T]) ElementContains(addr T) bool {
	return node.tobase().elementContains(addr)
}

// GetNode gets the node in the trie, with this subnode as the root, corresponding to the given address,
// or returns nil if not such element exists.
//
// It returns any node, whether added or not,
// including any prefix block node that was not added.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
func (node *TrieNode[T]) GetNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().getNode(addr))
}

// GetAddedNode gets trie nodes representing added elements.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Use Contains to check for the existence of a given address in the trie,
// as well as GetNode to search for all nodes including those not-added but also auto-generated nodes for subnet blocks.
func (node *TrieNode[T]) GetAddedNode(addr T) *TrieNode[T] {
	return toAddressTrieNode[T](node.tobase().getAddedNode(addr))
}

// NodeSize returns the number of nodes in the trie with this node as the root, which is more than the number of added addresses or blocks.
func (node *TrieNode[T]) NodeSize() int {
	return node.toBinTrieNode().NodeSize()
}

// Size returns the number of elements in the sub-trie with this node as the root.
// Only nodes for which IsAdded returns true are counted.
// When zero is returned, IsEmpty returns true.
func (node *TrieNode[T]) Size() int {
	return node.toBinTrieNode().Size()
}

// IsEmpty returns whether the size is zero.
func (node *TrieNode[T]) IsEmpty() bool {
	return node.Size() == 0
}

// TreeString returns a visual representation of the sub-trie with this node as the root, with one node per line.
//
//  - withNonAddedKeys: whether to show nodes that are not added nodes.
//  - withSizes: whether to include the counts of added nodes in each sub-trie.
func (node *TrieNode[T]) TreeString(withNonAddedKeys, withSizes bool) string {
	return node.toBinTrieNode().TreeString(withNonAddedKeys, withSizes)
}

// String returns a visual representation of this node including the key, with an open circle indicating this node is not an added node,
// a closed circle indicating this node is an added node.
func (node *TrieNode[T]) String() string {
	return node.toBinTrieNode().String()
}

// For some reason Format must be here and not in addressTrieNode for nil node.
// It panics in fmt code either way, but if in here then it is handled by a recover() call in fmt properly in the debugger.

// Format implements the [fmt.Formatter] interface.
func (node TrieNode[T]) Format(state fmt.State, verb rune) {
	node.toBinTrieNode().Format(state, verb)
}

//
//
//
//
//
//
//
//
//
//
//
//

//
//
//
//
//

// AssociativeTrieNode represents a node of an associative compact binary prefix trie.
// Each key is a prefix block subnet or address.  Each node also has an associated value.
type AssociativeTrieNode[T TrieKeyConstraint[T], V any] struct {
	trieNode[T, V]
}

func (node *AssociativeTrieNode[T, V]) toBinTrieNode() *tree.BinTrieNode[trieKey[T], V] {
	return (*tree.BinTrieNode[trieKey[T], V])(unsafe.Pointer(node))
}

func (node *AssociativeTrieNode[T, V]) toBase() *trieNode[T, V] {
	return (*trieNode[T, V])(unsafe.Pointer(node))
}

// GetKey gets the key used for placing the node in the trie.
func (node *AssociativeTrieNode[T, V]) GetKey() T {
	return node.toBase().getKey()
}

// IsRoot returns whether this is the root of the backing trie.
func (node *AssociativeTrieNode[T, V]) IsRoot() bool {
	return node.toBinTrieNode().IsRoot()
}

// IsAdded returns whether the node was "added".
// Some binary trie nodes are considered "added" and others are not.
// Those nodes created for key elements added to the trie are "added" nodes.
// Those that are not added are those nodes created to serve as junctions for the added nodes.
// Only added elements contribute to the size of a trie.
// When removing nodes, non-added nodes are removed automatically whenever they are no longer needed,
// which is when an added node has less than two added sub-nodes.
func (node *AssociativeTrieNode[T, V]) IsAdded() bool {
	return node.toBinTrieNode().IsAdded()
}

// SetAdded makes this node an added node, which is equivalent to adding the corresponding key to the trie.
// If the node is already an added node, this method has no effect.
// You cannot set an added node to non-added, for that you should Remove the node from the trie by calling Remove.
// A non-added node will only remain in the trie if it needs to be in the trie.
func (node *AssociativeTrieNode[T, V]) SetAdded() {
	node.toBinTrieNode().SetAdded()
}

// Clear removes this node and all sub-nodes from the tree, after which isEmpty will return true.
func (node *AssociativeTrieNode[T, V]) Clear() {
	node.toBinTrieNode().Clear()
}

// IsLeaf returns whether this node is in the tree (a node for which IsAdded is true)
// and there are no elements in the sub-tree with this node as the root.
func (node *AssociativeTrieNode[T, V]) IsLeaf() bool {
	return node.toBinTrieNode().IsLeaf()
}

// ClearValue makes the value associated with this node the zero-value of V.
func (node *AssociativeTrieNode[T, V]) ClearValue() {
	node.toBinTrieNode().ClearValue()
}

// SetValue sets the value associated with this node.
func (node *AssociativeTrieNode[T, V]) SetValue(val V) {
	node.toBinTrieNode().SetValue(val)
}

// GetValue returns whather there is a value associated with the node, and returns that value.
func (node *AssociativeTrieNode[T, V]) GetValue() V {
	return node.toBinTrieNode().GetValue()
}

// GetUpperSubNode gets the direct child node whose key is largest in value.
func (node *AssociativeTrieNode[T, V]) GetUpperSubNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().GetUpperSubNode())
}

// GetLowerSubNode gets the direct child node whose key is smallest in value.
func (node *AssociativeTrieNode[T, V]) GetLowerSubNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().GetLowerSubNode())
}

// GetParent gets the node from which this node is a direct child node, or nil if this is the root.
func (node *AssociativeTrieNode[T, V]) GetParent() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().GetParent())
}

// PreviousAddedNode returns the first added node that precedes this node following the trie order.
func (node *AssociativeTrieNode[T, V]) PreviousAddedNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().PreviousAddedNode())
}

// NextAddedNode returns the first added node that follows this node following the trie order.
func (node *AssociativeTrieNode[T, V]) NextAddedNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().NextAddedNode())
}

// NextNode returns the node that follows this node following the trie order.
func (node *AssociativeTrieNode[T, V]) NextNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().NextNode())
}

// PreviousNode returns the node that precedes this node following the trie order.
func (node *AssociativeTrieNode[T, V]) PreviousNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().PreviousNode())
}

// FirstNode returns the first (the lowest valued) node in the sub-trie originating from this node.
func (node *AssociativeTrieNode[T, V]) FirstNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().FirstNode())
}

// FirstAddedNode returns the first (the lowest valued) added node in the sub-trie originating from this node,
// or nil if there are no added entries in this trie or sub-trie.
func (node *AssociativeTrieNode[T, V]) FirstAddedNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().FirstAddedNode())
}

// LastNode returns the last (the highest valued) node in the sub-trie originating from this node.
func (node *AssociativeTrieNode[T, V]) LastNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().LastNode())
}

// LastAddedNode returns the last (the highest valued) added node in the sub-trie originating from this node,
// or nil if there are no added entries in this trie or sub-trie.
func (node *AssociativeTrieNode[T, V]) LastAddedNode() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().LastAddedNode())
}

// LowerAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the highest address strictly less than the given address.
func (node *AssociativeTrieNode[T, V]) LowerAddedNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().lowerAddedNode(addr))
}

// FloorAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the highest address less than or equal to the given address.
func (node *AssociativeTrieNode[T, V]) FloorAddedNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().floorAddedNode(addr))
}

// HigherAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the lowest address strictly greater than the given address.
func (node *AssociativeTrieNode[T, V]) HigherAddedNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().higherAddedNode(addr))
}

// CeilingAddedNode returns the added node, in this sub-trie with this node as the root, whose address is the lowest address greater than or equal to the given address.
func (node *AssociativeTrieNode[T, V]) CeilingAddedNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().ceilingAddedNode(addr))
}

// Iterator returns an iterator that iterates through the elements of the sub-trie with this node as the root.
// The iteration is in sorted element order.
func (node *AssociativeTrieNode[T, V]) Iterator() Iterator[T] {
	return node.toBase().iterator()
}

// DescendingIterator returns an iterator that iterates through the elements of the subtrie with this node as the root.
// The iteration is in reverse sorted element order.
func (node *AssociativeTrieNode[T, V]) DescendingIterator() Iterator[T] {
	return node.toBase().descendingIterator()
}

// NodeIterator returns an iterator that iterates through the added nodes of the sub-trie with this node as the root, in forward or reverse trie order.
func (node *AssociativeTrieNode[T, V]) NodeIterator(forward bool) IteratorWithRemove[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIteratorRem[T, V]{node.toBase().nodeIterator(forward)}
}

// AllNodeIterator returns an iterator that iterates through all the nodes of the sub-trie with this node as the root, in forward or reverse trie order.
func (node *AssociativeTrieNode[T, V]) AllNodeIterator(forward bool) IteratorWithRemove[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIteratorRem[T, V]{node.toBase().allNodeIterator(forward)}
}

// BlockSizeNodeIterator returns an iterator that iterates the added nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order is taken.
func (node *AssociativeTrieNode[T, V]) BlockSizeNodeIterator(lowerSubNodeFirst bool) IteratorWithRemove[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIteratorRem[T, V]{node.toBase().blockSizeNodeIterator(lowerSubNodeFirst)}
}

// BlockSizeAllNodeIterator returns an iterator that iterates all the nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
//
// If lowerSubNodeFirst is true, for blocks of equal size the lower is first, otherwise the reverse order.
func (node *AssociativeTrieNode[T, V]) BlockSizeAllNodeIterator(lowerSubNodeFirst bool) IteratorWithRemove[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIteratorRem[T, V]{node.toBase().blockSizeAllNodeIterator(lowerSubNodeFirst)}
}

// BlockSizeCachingAllNodeIterator returns an iterator that iterates all nodes, ordered by keys from largest prefix blocks to smallest and then to individual addresses,
// in the sub-trie with this node as the root.
func (node *AssociativeTrieNode[T, V]) BlockSizeCachingAllNodeIterator() CachingTrieIterator[*AssociativeTrieNode[T, V]] {
	return cachingAssociativeAddressTrieNodeIteratorX[T, V]{node.toBase().blockSizeCachingAllNodeIterator()}
}

// ContainingFirstIterator returns an iterator that does a pre-order binary trie traversal of the added nodes
// of the sub-trie with this node as the root.
//
// All added nodes will be visited before their added sub-nodes.
// For an address trie this means added containing subnet blocks will be visited before their added contained addresses and subnet blocks.
//
// Once a given node is visited, the iterator allows you to cache an object corresponding to the
// lower or upper sub-node that can be retrieved when you later visit that sub-node.
//
// Objects are cached only with nodes to be visited.
// So for this iterator that means an object will be cached with the first added lower or upper sub-node,
// the next lower or upper sub-node to be visited,
// which is not necessarily the direct lower or upper sub-node of a given node.
//
// The caching allows you to provide iteration context from a parent to its sub-nodes when iterating.
// The caching and retrieval is done in constant-time.
func (node *AssociativeTrieNode[T, V]) ContainingFirstIterator(forwardSubNodeOrder bool) CachingTrieIterator[*AssociativeTrieNode[T, V]] {
	return cachingAssociativeAddressTrieNodeIteratorX[T, V]{node.toBase().containingFirstIterator(forwardSubNodeOrder)}
}

// ContainingFirstAllNodeIterator returns an iterator that does a pre-order binary trie traversal of all the nodes
// of the sub-trie with this node as the root.
//
// All nodes will be visited before their sub-nodes.
// For an address trie this means containing subnet blocks will be visited before their contained addresses and subnet blocks.
//
// Once a given node is visited, the iterator allows you to cache an object corresponding to the
// lower or upper sub-node that can be retrieved when you later visit that sub-node.
// That allows you to provide iteration context from a parent to its sub-nodes when iterating.
// The caching and retrieval is done in constant-time.
func (node *AssociativeTrieNode[T, V]) ContainingFirstAllNodeIterator(forwardSubNodeOrder bool) CachingTrieIterator[*AssociativeTrieNode[T, V]] {
	return cachingAssociativeAddressTrieNodeIteratorX[T, V]{node.toBase().containingFirstAllNodeIterator(forwardSubNodeOrder)}
}

// ContainedFirstIterator returns an iterator that does a post-order binary trie traversal of the added nodes
// of the sub-trie with this node as the root.
// All added sub-nodes will be visited before their parent nodes.
// For an address trie this means contained addresses and subnets will be visited before their containing subnet blocks.
func (node *AssociativeTrieNode[T, V]) ContainedFirstIterator(forwardSubNodeOrder bool) IteratorWithRemove[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIteratorRem[T, V]{node.toBase().containedFirstIterator(forwardSubNodeOrder)}
}

// ContainedFirstAllNodeIterator returns an iterator that does a post-order binary trie traversal of all the nodes
// of the sub-trie with this node as the root.
// All sub-nodes will be visited before their parent nodes.
// For an address trie this means contained addresses and subnets will be visited before their containing subnet blocks.
func (node *AssociativeTrieNode[T, V]) ContainedFirstAllNodeIterator(forwardSubNodeOrder bool) Iterator[*AssociativeTrieNode[T, V]] {
	return associativeAddressTrieNodeIterator[T, V]{node.toBase().containedFirstAllNodeIterator(forwardSubNodeOrder)}
}

// Clone clones the node.
// Keys remain the same, but the parent node and the lower and upper sub-nodes are all set to nil.
func (node *AssociativeTrieNode[T, V]) Clone() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().Clone())
}

// CloneTree clones the sub-trie starting with this node as the root.
// The nodes are cloned, but their keys and values are not cloned.
func (node *AssociativeTrieNode[T, V]) CloneTree() *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBinTrieNode().CloneTree())
}

// AsNewTrie creates a new sub-trie, copying the nodes starting with this node as the root.
// The nodes are copies of the nodes in this sub-trie, but their keys and values are not copies.
func (node *AssociativeTrieNode[T, V]) AsNewTrie() *AssociativeTrie[T, V] {
	return toAssociativeTrie[T, V](node.toBinTrieNode().AsNewTrie())
}

// Compare returns a negative integer, zero, or a positive integer if this node is less than, equal, or greater than the other, according to the key and the trie order.
func (node *AssociativeTrieNode[T, V]) Compare(other *AssociativeTrieNode[T, V]) int {
	return node.toBinTrieNode().Compare(other.toBinTrieNode())
}

// Equal returns whether the key and mapped value match those of the given node.
func (node *AssociativeTrieNode[T, V]) Equal(other *AssociativeTrieNode[T, V]) bool {
	return node.toBinTrieNode().Equal(other.toBinTrieNode())
}

// TreeEqual returns whether the sub-trie represented by this node as the root node matches the given sub-trie.
func (node *AssociativeTrieNode[T, V]) TreeEqual(other *AssociativeTrieNode[T, V]) bool {
	return node.toBinTrieNode().TreeEqual(other.toBinTrieNode())
}

// DeepEqual returns whether the key is equal to that of the given node and the value is deep equal to that of the given node.
func (node *AssociativeTrieNode[T, V]) DeepEqual(other *AssociativeTrieNode[T, V]) bool {
	return node.toBinTrieNode().DeepEqual(other.toBinTrieNode())
}

// TreeDeepEqual returns whether the sub-trie represented by this node as the root node matches the given sub-trie, matching with Compare on the keys and reflect.DeepEqual on the values.
func (node *AssociativeTrieNode[T, V]) TreeDeepEqual(other *AssociativeTrieNode[T, V]) bool {
	return node.toBinTrieNode().TreeDeepEqual(other.toBinTrieNode())
}

/////////////////////////////////////////////////////////////////////////////

// Remove removes this node from the collection of added nodes, and also from the trie if possible.
// If it has two sub-nodes, it cannot be removed from the trie, in which case it is marked as not "added",
// nor is it counted in the trie size.
// Only added nodes can be removed from the trie.  If this node is not added, this method does nothing.
func (node *AssociativeTrieNode[T, V]) Remove() {
	node.toBinTrieNode().Remove()
}

// Contains returns whether the given address or prefix block subnet is in the sub-trie, as an added element, with this node as the root.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the prefix block or address address exists already in the trie, false otherwise.
//
// Use GetAddedNode  to get the node for the address rather than just checking for its existence.
func (node *AssociativeTrieNode[T, V]) Contains(addr T) bool {
	return node.toBase().contains(addr)
}

// RemoveNode removes the given single address or prefix block subnet from the trie with this node as the root.
//
// Removing an element will not remove contained elements (nodes for contained blocks and addresses).
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the prefix block or address was removed, false if not already in the trie.
//
// You can also remove by calling GetAddedNode to get the node and then calling Remove on the node.
//
// When an address is removed, the corresponding node may remain in the trie if it remains a subnet block for two sub-nodes.
// If the corresponding node can be removed from the trie, it will be removed.
func (node *AssociativeTrieNode[T, V]) RemoveNode(addr T) bool {
	return node.toBase().removeNode(addr)
}

// RemoveElementsContainedBy removes any single address or prefix block subnet from the trie, with this node as the root, that is contained in the given individual address or prefix block subnet.
//
// Goes further than Remove, not requiring a match to an inserted node, and also removing all the sub-nodes of any removed node or sub-node.
//
// For example, after inserting 1.2.3.0 and 1.2.3.1, passing 1.2.3.0/31 to RemoveElementsContainedBy will remove them both,
// while the Remove method will remove nothing.
// After inserting 1.2.3.0/31, then Remove(Address) will remove 1.2.3.0/31, but will leave 1.2.3.0 and 1.2.3.1 in the trie.
//
// It cannot partially delete a node, such as deleting a single address from a prefix block represented by a node.
// It can only delete the whole node if the whole address or block represented by that node is contained in the given address or block.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
//Returns the root node of the subtrie that was removed from the trie, or nil if nothing was removed.
func (node *AssociativeTrieNode[T, V]) RemoveElementsContainedBy(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().removeElementsContainedBy(addr))
}

// ElementsContainedBy checks if a part of this trie, with this node as the root, is contained by the given prefix block subnet or individual address.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns the root node of the contained subtrie, or nil if no subtrie is contained.
// The node returned need not be an "added" node, see IsAdded for more details on added nodes.
// The returned subtrie is backed by this trie, so changes in this trie are reflected in those nodes and vice-versa.
func (node *AssociativeTrieNode[T, V]) ElementsContainedBy(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().elementsContainedBy(addr))
}

// ElementsContaining finds the trie nodes in the trie, with this sub-node as the root,
// containing the given key and returns them as a linked list.
// Only added nodes are added to the linked list.
//
// If the argument is not a single address nor prefix block, this method will panic.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
func (node *AssociativeTrieNode[T, V]) ElementsContaining(addr T) *ContainmentValuesPath[T, V] {
	return &ContainmentValuesPath[T, V]{*node.toBase().elementsContaining(addr)}
}

// LongestPrefixMatch returns the address or subnet with the longest prefix of all the added subnets or the address whose prefix matches the given address.
// This is equivalent to finding the containing subnet or address with the smallest subnet size.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns nil if no added subnet or address contains the given argument.
//
// Use ElementContains to check for the existence of a containing address.
// To get all the containing addresses (subnets with matching prefix), use ElementsContaining.
// To get the node corresponding to the result of this method, use LongestPrefixMatchNode.
func (node *AssociativeTrieNode[T, V]) LongestPrefixMatch(addr T) T {
	return node.toBase().longestPrefixMatch(addr)
}

// LongestPrefixMatchNode finds the containing subnet or address in the trie with the smallest subnet size,
// which is equivalent to finding the subnet or address with the longest matching prefix.
// Returns the node corresponding to that subnet.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns nil if no added subnet or address contains the given argument.
//
// Use ElementContains to check for the existence of a containing address.
// To get all the containing addresses, use ElementsContaining.
// Use LongestPrefixMatch to get only the address corresponding to the result of this method.
func (node *AssociativeTrieNode[T, V]) LongestPrefixMatchNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().longestPrefixMatchNode(addr))
}

// ElementContains checks if a prefix block subnet or address in the trie, with this node as the root, contains the given subnet or address.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns true if the subnet or address is contained by a trie element, false otherwise.
//
// To get all the containing addresses, use ElementsContaining.
func (node *AssociativeTrieNode[T, V]) ElementContains(addr T) bool {
	return node.toBase().elementContains(addr)
}

// GetNode gets the node in the trie, with this subnode as the root, corresponding to the given address,
// or returns nil if not such element exists.
//
// It returns any node, whether added or not,
// including any prefix block node that was not added.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
func (node *AssociativeTrieNode[T, V]) GetNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().getNode(addr))
}

// GetAddedNode gets trie nodes representing added elements.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Use Contains to check for the existence of a given address in the trie,
// as well as GetNode to search for all nodes including those not-added but also auto-generated nodes for subnet blocks.
func (node *AssociativeTrieNode[T, V]) GetAddedNode(addr T) *AssociativeTrieNode[T, V] {
	return toAssociativeTrieNode[T, V](node.toBase().getAddedNode(addr))
}

// Get gets the value for the specified key in this mapped trie or subtrie.
//
// If the argument is not a single address nor prefix block, this method will panic.
// The [Partition] type can be used to convert the argument to single addresses and prefix blocks before calling this method.
//
// Returns the value for the given key.
// Returns nil if the contains no mapping for that key or if the mapped value is nil.
func (node *AssociativeTrieNode[T, V]) Get(addr T) (V, bool) {
	return node.toBase().get(addr)
}

// NodeSize returns the number of nodes in the trie with this node as the root, which is more than the number of added addresses or blocks.
func (node *AssociativeTrieNode[T, V]) NodeSize() int {
	return node.toBinTrieNode().NodeSize()
}

// Size returns the number of elements in the trie.
// Only nodes for which IsAdded returns true are counted.
// When zero is returned, IsEmpty returns true.
func (node *AssociativeTrieNode[T, V]) Size() int {
	return node.toBinTrieNode().Size()
}

// IsEmpty returns whether the size is zero.
func (node *AssociativeTrieNode[T, V]) IsEmpty() bool {
	return node.Size() == 0
}

// TreeString returns a visual representation of the sub-trie with this node as the root, with one node per line.
//
//  - withNonAddedKeys: whether to show nodes that are not added nodes
//  - withSizes: whether to include the counts of added nodes in each sub-trie
func (node *AssociativeTrieNode[T, V]) TreeString(withNonAddedKeys, withSizes bool) string {
	return node.toBinTrieNode().TreeString(withNonAddedKeys, withSizes)
}

// String returns a visual representation of this node including the key, with an open circle indicating this node is not an added node,
// a closed circle indicating this node is an added node.
func (node *AssociativeTrieNode[T, V]) String() string {
	return node.toBinTrieNode().String()
}

// For some reason Format must be here and not in addressTrieNode for nil node.
// It panics in fmt code either way, but if in here then it is handled by a recover() call in fmt properly in the debugger.

// Format implements the [fmt.Formatter] interface.
func (node AssociativeTrieNode[T, V]) Format(state fmt.State, verb rune) {
	node.toBase().binNode.Format(state, verb)
}

//
//
//
//
//
//
//

// ContainmentPath represents a path through the trie of containing subnets,
// each node in the path contained by the previous node,
// the first node corresponding to the shortest prefix match, the last element corresponding to the longest prefix match.
type containmentPath[T TrieKeyConstraint[T], V any] struct {
	path tree.Path[trieKey[T], V]
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (path *containmentPath[T, V]) count() int {
	if path == nil {
		return 0
	}
	return path.path.Size()
}

// String returns a visual representation of the Path with one node per line.
func (path *containmentPath[T, V]) string() string {
	if path == nil {
		return nilString()
	}
	return path.path.String()
}

func toContainmentPath[T TrieKeyConstraint[T], V any](path *tree.Path[trieKey[T], V]) *containmentPath[T, V] {
	return (*containmentPath[T, V])(unsafe.Pointer(path))
}

//
//
//
//
//

// ContainmentPath represents a path through the trie of containing subnets,
// each node in the path contained by the previous node,
// the first node corresponding to the shortest prefix match, the last element corresponding to the longest prefix match.
type ContainmentPath[T TrieKeyConstraint[T]] struct {
	containmentPath[T, emptyValue]
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (path *ContainmentPath[T]) Count() int {
	return path.count()
}

// String returns a visual representation of the Path with one node per line.
func (path *ContainmentPath[T]) String() string {
	return path.string()
}

// ShortestPrefixMatch returns the beginning of the Path of containing subnets, which may or may not match the tree root of the originating tree.
// If there are no containing elements (prefix matches) this returns nil.
func (path *ContainmentPath[T]) ShortestPrefixMatch() *ContainmentPathNode[T] {
	return toContainmentPathNode[T](path.path.GetRoot())
}

// LongestPrefixMatch returns the end of the Path of containing subnets, which may or may not match a leaf in the originating tree.
// If there are no containing elements (prefix matches) this returns nil.
func (path *ContainmentPath[T]) LongestPrefixMatch() *ContainmentPathNode[T] {
	return toContainmentPathNode[T](path.path.GetLeaf())
}

//
//
//
//
//
//
//
//
//

// ContainmentValuesPath represents a path through the associative trie of containing subnets,
// each node in the path contained by the previous node,
// the first node corresponding to the shortest prefix match, the last element corresponding to the longest prefix match.
type ContainmentValuesPath[T TrieKeyConstraint[T], V any] struct {
	containmentPath[T, V]
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (path *ContainmentValuesPath[T, V]) Count() int {
	return path.count()
}

// String returns a visual representation of the Path with one node per line.
func (path *ContainmentValuesPath[T, V]) String() string {
	return path.string()
}

// ShortestPrefixMatch returns the beginning of the Path of containing subnets, which may or may not match the tree root of the originating tree.
// If there are no containing elements (prefix matches) this returns nil.
func (path *ContainmentValuesPath[T, V]) ShortestPrefixMatch() *ContainmentValuesPathNode[T, V] {
	return toContainmentValuesPathNode[T, V](path.path.GetRoot())
}

// LongestPrefixMatch returns the end of the Path of containing subnets, which may or may not match a leaf in the originating tree.
// If there are no containing elements (prefix matches) this returns nil.
func (path *ContainmentValuesPath[T, V]) LongestPrefixMatch() *ContainmentValuesPathNode[T, V] {
	return toContainmentValuesPathNode[T, V](path.path.GetLeaf())
}

//
//
//
//
//
//
//
//
//

// ContainmentPathNode is a node in a ContainmentPath
type containmentPathNode[T TrieKeyConstraint[T], V any] struct {
	pathNode tree.PathNode[trieKey[T], V]
}

// getKey gets the containing block or matching address corresponding to this node
func (node *containmentPathNode[T, V]) getKey() T {
	return node.pathNode.GetKey().address
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (node *containmentPathNode[T, V]) count() int {
	if node == nil {
		return 0
	}
	return node.pathNode.Size()
}

// String returns a visual representation of this node including the address key
func (node *containmentPathNode[T, V]) string() string {
	if node == nil {
		return nilString()
	}
	return node.pathNode.String()
}

// ListString returns a visual representation of the containing subnets starting from this node and moving downwards to sub-nodes.
func (node *containmentPathNode[T, V]) listString() string {
	return node.pathNode.ListString(true, true)
}

//
//
//
//
//
//

// ContainmentPathNode is a node in a ContainmentPath
type ContainmentPathNode[T TrieKeyConstraint[T]] struct {
	containmentPathNode[T, emptyValue]
}

// GetKey gets the containing block or matching address corresponding to this node
func (node *ContainmentPathNode[T]) GetKey() T {
	return node.getKey()
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (node *ContainmentPathNode[T]) Count() int {
	return node.count()
}

// String returns a visual representation of this node including the address key
func (node *ContainmentPathNode[T]) String() string {
	return node.string()
}

// ListString returns a visual representation of the containing subnets starting from this node and moving downwards to sub-nodes.
func (node *ContainmentPathNode[T]) ListString() string {
	return node.listString()
}

// Next gets the node contained by this node
func (node *ContainmentPathNode[T]) Next() *ContainmentPathNode[T] {
	return toContainmentPathNode[T](node.pathNode.Next())
}

// Previous gets the node containing this node
func (node *ContainmentPathNode[T]) Previous() *ContainmentPathNode[T] {
	return toContainmentPathNode[T](node.pathNode.Previous())
}

func toContainmentPathNode[T TrieKeyConstraint[T]](node *tree.PathNode[trieKey[T], emptyValue]) *ContainmentPathNode[T] {
	return (*ContainmentPathNode[T])(unsafe.Pointer(node))
}

//
//
//
//
//
//

// ContainmentValuesPathNode is a node in a ContainmentPath
type ContainmentValuesPathNode[T TrieKeyConstraint[T], V any] struct {
	containmentPathNode[T, V]
}

// GetKey gets the containing block or matching address corresponding to this node
func (node *ContainmentValuesPathNode[T, V]) GetKey() T {
	return node.getKey()
}

// Count returns the count of containing subnets in the path of containing subnets, starting from this node and moving downwards to sub-nodes.
// This is a constant-time operation since the size is maintained in each node and adjusted with each add and Remove operation in the sub-tree.
func (node *ContainmentValuesPathNode[T, V]) Count() int {
	return node.count()
}

// String returns a visual representation of this node including the address key
func (node *ContainmentValuesPathNode[T, V]) String() string {
	return node.string()
}

// ListString returns a visual representation of the containing subnets starting from this node and moving downwards to sub-nodes.
func (node *ContainmentValuesPathNode[T, V]) ListString() string {
	return node.listString()
}

// Next gets the node contained by this node
func (node *ContainmentValuesPathNode[T, V]) Next() *ContainmentValuesPathNode[T, V] {
	return toContainmentValuesPathNode[T, V](node.pathNode.Next())
}

// Previous gets the node containing this node
func (node *ContainmentValuesPathNode[T, V]) Previous() *ContainmentValuesPathNode[T, V] {
	return toContainmentValuesPathNode[T, V](node.pathNode.Previous())
}

// GetValue returns the value assigned to the block or address, if the node was an associative node from an associative trie.
// Otherwise, it returns the zero value.
func (node *ContainmentValuesPathNode[T, V]) GetValue() V {
	return node.pathNode.GetValue()
}

func toContainmentValuesPathNode[T TrieKeyConstraint[T], V any](node *tree.PathNode[trieKey[T], V]) *ContainmentValuesPathNode[T, V] {
	return (*ContainmentValuesPathNode[T, V])(unsafe.Pointer(node))
}
