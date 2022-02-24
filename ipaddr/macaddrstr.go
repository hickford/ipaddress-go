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
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
	"github.com/seancfoley/ipaddress-go/ipaddr/addrstrparam"
)

var defaultMACAddrParameters = new(addrstrparam.MACAddressStringParamsBuilder).ToParams()

// NewMACAddressStringParams constructs a MACAddressString that will parse the given string according to the given parameters
func NewMACAddressStringParams(str string, params addrstrparam.MACAddressStringParams) *MACAddressString {
	var p addrstrparam.MACAddressStringParams
	if params == nil {
		p = defaultMACAddrParameters
	} else {
		p = addrstrparam.CopyMACAddressStringParams(params)
	}
	return &MACAddressString{str: strings.TrimSpace(str), params: p, macAddrStringCache: new(macAddrStringCache)}
}

// NewMACAddressString constructs a MACAddressString that will parse the given string according to the default parameters
func NewMACAddressString(str string) *MACAddressString {
	return &MACAddressString{str: strings.TrimSpace(str), params: defaultMACAddrParameters, macAddrStringCache: new(macAddrStringCache)}
}

func newMACAddressStringFromAddr(str string, addr *MACAddress) *MACAddressString {
	return &MACAddressString{
		str:    str,
		params: defaultMACAddrParameters,
		macAddrStringCache: &macAddrStringCache{
			&macAddrData{
				addressProvider: wrappedMACAddressProvider{addr},
			},
		},
	}
}

var zeroMACAddressString = NewMACAddressString("")

type macAddrData struct {
	addressProvider   macAddressProvider
	validateException addrerr.AddressStringError
}

type macAddrStringCache struct {
	*macAddrData
}

type MACAddressString struct {
	str    string
	params addrstrparam.MACAddressStringParams // when nil, defaultParameters is used
	*macAddrStringCache
}

func (addrStr *MACAddressString) init() *MACAddressString {
	if addrStr.macAddrStringCache == nil {
		return zeroMACAddressString
	}
	return addrStr
}

func (addrStr *MACAddressString) GetValidationOptions() addrstrparam.MACAddressStringParams {
	return addrStr.init().params
}

func (addrStr *MACAddressString) String() string {
	if addrStr == nil {
		return nilString()
	}
	return addrStr.str
}

func (addrStr *MACAddressString) ToNormalizedString() string {
	addr := addrStr.GetAddress()
	if addr != nil {
		return addr.toNormalizedString()
	}
	return addrStr.String()
}

func (addrStr *MACAddressString) GetAddress() *MACAddress {
	provider, err := addrStr.getAddressProvider()
	if err != nil {
		return nil
	}
	addr, _ := provider.getAddress()
	return addr
}

func (addrStr *MACAddressString) ToAddress() (*MACAddress, addrerr.AddressError) {
	provider, err := addrStr.getAddressProvider()
	if err != nil {
		return nil, err
	}
	return provider.getAddress()
}

// IsPrefixed returns whether this address represents the set of all addresses with the same prefix
func (addrStr *MACAddressString) IsPrefixed() bool {
	return addrStr.getPrefixLen() != nil
}

// GetPrefixLen returns the prefix length if this address is a valid prefixed address, otherwise returns nil
func (addrStr *MACAddressString) GetPrefixLen() PrefixLen {
	return addrStr.getPrefixLen().copy()
}

func (addrStr *MACAddressString) getPrefixLen() PrefixLen {
	addr := addrStr.GetAddress()
	if addr != nil {
		return addr.getPrefixLen()
	}
	return nil
}

// IsFullRange returns whether the address represents the set all all valid MAC48Len addresses for its address length
func (addrStr *MACAddressString) IsFullRange() bool {
	addr := addrStr.GetAddress()
	return addr != nil && addr.IsFullRange()
}

//IsEmpty returns true if the address is empty (zero-length).
func (addrStr *MACAddressString) IsEmpty() bool {
	addr, err := addrStr.ToAddress()
	return err == nil && addr == nil
}

func (addrStr *MACAddressString) IsZero() bool {
	addr := addrStr.GetAddress()
	return addr != nil && addr.IsZero()
}

func (addrStr *MACAddressString) IsValid() bool {
	return addrStr.Validate() == nil
}

func (addrStr *MACAddressString) getAddressProvider() (macAddressProvider, addrerr.AddressStringError) {
	addrStr = addrStr.init()
	err := addrStr.Validate()
	return addrStr.addressProvider, err
}

// Validate validates that this string is a valid address, and if not, throws an exception with a descriptive message indicating why it is not.
func (addrStr *MACAddressString) Validate() addrerr.AddressStringError {
	addrStr = addrStr.init()
	data := addrStr.macAddrData
	if data == nil {
		addressProvider, err := validator.validateMACAddressStr(addrStr)
		data = &macAddrData{addressProvider, err}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&addrStr.macAddrData))
		atomic.StorePointer(dataLoc, unsafe.Pointer(data))
	}
	return data.validateException
}

func (addrStr *MACAddressString) Compare(other *MACAddressString) int {
	if addrStr == other {
		return 0
	} else if addrStr == nil {
		return -1
	} else if other == nil {
		return 1
	}
	addrStr = addrStr.init()
	other = other.init()
	if addrStr == other {
		return 0
	}
	if addrStr.IsValid() {
		if other.IsValid() {
			addr := addrStr.GetAddress()
			if addr != nil {
				otherAddr := other.GetAddress()
				if otherAddr != nil {
					return addr.Compare(otherAddr)
				}
			}
			// one or the other is null, either empty or IncompatibleAddressException
			return strings.Compare(addrStr.String(), other.String())
		}
		return 1
	} else if other.IsValid() {
		return -1
	}
	return strings.Compare(addrStr.String(), other.String())
}

// Two MACAddressString objects are equal if they represent the same set of addresses.
//
// If a MACAddressString is invalid, it is equal to another address only if the other address was constructed from the same string.
func (addrStr *MACAddressString) Equal(other *MACAddressString) bool {
	if addrStr == nil {
		return other == nil
	} else if other == nil {
		return false
	}
	addrStr = addrStr.init()
	other = other.init()
	if addrStr == other {
		return true
	}

	//if they have the same string, they must be the same,
	//but the converse is not true, if they have different strings, they can still be the same

	// Also note that we do not call equals() on the validation options, this is intended as an optimization,
	// and probably better to avoid going through all the validation objects here
	stringsMatch := addrStr.String() == other.String()
	if stringsMatch && addrStr.params == other.params {
		return true
	}
	if addrStr.IsValid() {
		if other.IsValid() {
			value := addrStr.GetAddress()
			if value != nil {
				otherValue := other.GetAddress()
				if otherValue != nil {
					return value.equals(otherValue)
				} else {
					return false
				}
			} else if other.GetAddress() != nil {
				return false
			}
			// both are null, either empty or addrerr.IncompatibleAddressError
			return stringsMatch
		}
	} else if !other.IsValid() { // both are invalid
		return stringsMatch // Two invalid addresses are not equal unless strings match, regardless of validation options
	}
	return false
}

func (addrStr *MACAddressString) Wrap() ExtendedIdentifierString {
	return WrappedMACAddressString{addrStr}
}
