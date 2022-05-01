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
	"net"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/seancfoley/ipaddress-go/ipaddr/addrerr"
	"github.com/seancfoley/ipaddress-go/ipaddr/addrstrparam"
)

const (
	PortSeparator    = ':'
	LabelSeparator   = '.'
	IPv6StartBracket = '['
	IPv6EndBracket   = ']'
)

// NewHostName constructs an HostName that will parse the given string according to the default parameters
func NewHostName(str string) *HostName {
	str = strings.TrimSpace(str)
	return &HostName{str: str, params: defaultHostParameters, hostCache: &hostCache{}}
}

// NewHostNameParams constructs an HostName that will parse the given string according to the given parameters
func NewHostNameParams(str string, params addrstrparam.HostNameParams) *HostName {
	var prms addrstrparam.HostNameParams
	if params == nil {
		prms = defaultHostParameters
	} else {
		prms = addrstrparam.CopyHostNameParams(params)
	}
	str = strings.TrimSpace(str)
	return &HostName{str: str, params: prms, hostCache: &hostCache{}}
}

func NewHostNameFromAddrPort(addr *IPAddress, port int) *HostName {
	portVal := PortInt(port)
	hostStr := toNormalizedAddrPortString(addr, portVal)
	parsedHost := parsedHost{
		originalStr:     hostStr,
		embeddedAddress: embeddedAddress{addressProvider: addr.getProvider()},
		labelsQualifier: parsedHostIdentifierStringQualifier{port: cachePorts(portVal)},
	}
	return &HostName{
		str:       hostStr,
		params:    defaultHostParameters,
		hostCache: &hostCache{normalizedString: &hostStr, hostData: &hostData{parsedHost: &parsedHost}},
	}
}

func NewHostNameFromAddr(addr *IPAddress) *HostName {
	hostStr := addr.ToNormalizedString()
	return newHostNameFromAddr(hostStr, addr)
}

func newHostNameFromAddr(hostStr string, addr *IPAddress) *HostName { // same as HostName(String hostStr, ParsedHost parsed) {
	parsedHost := parsedHost{
		originalStr:     hostStr,
		embeddedAddress: embeddedAddress{addressProvider: addr.getProvider()},
	}
	return &HostName{
		str:       hostStr,
		params:    defaultHostParameters,
		hostCache: &hostCache{normalizedString: &hostStr, hostData: &hostData{parsedHost: &parsedHost}},
	}
}

func NewHostNameFromNetTCPAddr(addr *net.TCPAddr) (*HostName, addrerr.AddressValueError) {
	return newHostNameFromSocketAddr(addr.IP, addr.Port, addr.Zone)
}

func NewHostNameFromNetUDPAddr(addr *net.UDPAddr) (*HostName, addrerr.AddressValueError) {
	return newHostNameFromSocketAddr(addr.IP, addr.Port, addr.Zone)
}

func newHostNameFromSocketAddr(ip net.IP, port int, zone string) (hostName *HostName, err addrerr.AddressValueError) {
	var ipAddr *IPAddress
	ipAddr, err = NewIPAddressFromNetIPAddr(&net.IPAddr{IP: ip, Zone: zone})
	if err != nil {
		return
	} else if ipAddr == nil {
		err = &addressValueError{addressError: addressError{key: "ipaddress.error.exceeds.size"}}
		return
	}
	portVal := PortInt(port)
	hostStr := toNormalizedAddrPortString(ipAddr, portVal)
	parsedHost := parsedHost{
		originalStr:     hostStr,
		embeddedAddress: embeddedAddress{addressProvider: ipAddr.getProvider()},
		labelsQualifier: parsedHostIdentifierStringQualifier{port: cachePorts(portVal)},
	}
	hostName = &HostName{
		str:       hostStr,
		params:    defaultHostParameters,
		hostCache: &hostCache{normalizedString: &hostStr, hostData: &hostData{parsedHost: &parsedHost}},
	}
	return
}

func NewHostNameFromNetIP(bytes net.IP) (hostName *HostName, err addrerr.AddressValueError) {
	var addr *IPAddress
	addr, err = NewIPAddressFromNetIP(bytes)
	if err != nil {
		return
	} else if addr == nil {
		err = &addressValueError{addressError: addressError{key: "ipaddress.error.exceeds.size"}}
		return
	}
	hostName = NewHostNameFromAddr(addr)
	return
}

func NewHostNameFromPrefixedNetIP(bytes net.IP, prefixLen PrefixLen) (hostName *HostName, err addrerr.AddressValueError) {
	var addr *IPAddress
	addr, err = NewIPAddressFromPrefixedNetIP(bytes, prefixLen)
	if err != nil {
		return
	} else if addr == nil {
		err = &addressValueError{addressError: addressError{key: "ipaddress.error.exceeds.size"}}
		return
	}

	hostName = NewHostNameFromAddr(addr)
	return
}

func NewHostNameFromNetIPAddr(addr *net.IPAddr) (hostName *HostName, err addrerr.AddressValueError) {
	var ipAddr *IPAddress
	ipAddr, err = NewIPAddressFromNetIPAddr(addr)
	if err != nil {
		return
	} else if ipAddr == nil {
		err = &addressValueError{addressError: addressError{key: "ipaddress.error.exceeds.size"}}
		return
	}
	hostName = NewHostNameFromAddr(ipAddr)
	return
}

func NewHostNameFromPrefixedNetIPAddr(addr *net.IPAddr, prefixLen PrefixLen) (hostName *HostName, err addrerr.AddressValueError) {
	var ipAddr *IPAddress
	ipAddr, err = NewIPAddressFromPrefixedNetIPAddr(addr, prefixLen)
	if err != nil {
		return
	} else if ipAddr == nil {
		err = &addressValueError{addressError: addressError{key: "ipaddress.error.exceeds.size"}}
		return
	}
	hostName = NewHostNameFromAddr(ipAddr)
	return
}

var defaultHostParameters = new(addrstrparam.HostNameParamsBuilder).ToParams()

var zeroHost = NewHostName("")

type hostData struct {
	parsedHost    *parsedHost
	validateError addrerr.HostNameError
}

type resolveData struct {
	resolvedAddrs []*IPAddress
	err           error
}

type hostCache struct {
	*hostData
	*resolveData
	normalizedString,
	normalizedWildcardString,
	qualifiedString *string
}

type HostName struct {
	str    string
	params addrstrparam.HostNameParams
	*hostCache
}

func (host *HostName) init() *HostName {
	if host.params == nil { // the only way params can be nil is when str == "" as well
		return zeroHost
	}
	return host
}

func (host *HostName) GetValidationOptions() addrstrparam.HostNameParams {
	return host.init().params
}

// Validate validates that this string is a valid address, and if not, throws an exception with a descriptive message indicating why it is not.
func (host *HostName) Validate() addrerr.HostNameError {
	host = host.init()
	data := host.hostData
	if data == nil {
		parsedHost, err := validator.validateHostName(host)
		data = &hostData{parsedHost, err}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&host.hostData))
		atomic.StorePointer(dataLoc, unsafe.Pointer(data))
	}
	return data.validateError
}

// String implements the fmt.Stringer interface,
// returning the original string used to create this HostName (altered by strings.TrimSpace if a host name and not an address),
// or "<nil>" if the receiver is a nil pointer
func (host *HostName) String() string {
	if host == nil {
		return nilString()
	}
	return host.str
}

func (host *HostName) IsAddressString() bool {
	host = host.init()
	return host.IsValid() && host.parsedHost.isAddressString()
}

func (host *HostName) IsAddress() bool {
	if host.IsAddressString() {
		addr, _ := host.init().parsedHost.asAddress()
		return addr != nil
	}
	return false
}

func (host *HostName) AsAddress() *IPAddress {
	if host.IsAddress() {
		addr, _ := host.parsedHost.asAddress()
		return addr
	}
	return nil
}

func (host *HostName) IsAllAddresses() bool {
	host = host.init()
	return host.IsValid() && host.parsedHost.getAddressProvider().isProvidingAllAddresses()
}

func (host *HostName) IsEmpty() bool {
	host = host.init()
	return host.IsValid() && ((host.IsAddressString() && host.parsedHost.getAddressProvider().isProvidingEmpty()) || len(host.GetNormalizedLabels()) == 0)
}

func (host *HostName) GetAddress() *IPAddress {
	addr, _ := host.ToAddress()
	return addr
}

// ToAddress resolves to an address.
// This method can potentially return a list of resolved addresses and an error as well if some resolved addresses were invalid.
func (host *HostName) ToAddress() (addr *IPAddress, err addrerr.AddressError) {
	addresses, err := host.ToAddresses()
	if len(addresses) > 0 {
		addr = addresses[0]
	}
	return
}

// ToAddresses resolves to one or more addresses.
// The error can be addrerr.AddressStringError,addrerr.IncompatibleAddressError, or addrerr.HostNameError.
// This method can potentially return a list of resolved addresses and an error as well if some resolved addresses were invalid.
func (host *HostName) ToAddresses() (addrs []*IPAddress, err addrerr.AddressError) {
	host = host.init()
	data := host.resolveData
	if data == nil {
		//note that validation handles empty address resolution
		err = host.Validate() //addrerr.HostNameError
		if err != nil {
			return
		}
		// http://networkbit.ch/golang-dns-lookup/
		parsedHost := host.parsedHost
		if parsedHost.isAddressString() {
			addr, addrErr := parsedHost.asAddress() //addrerr.IncompatibleAddressError
			addrs, err = []*IPAddress{addr}, addrErr
			//note there is no need to apply prefix or mask here, it would have been applied to the address already
		} else {
			strHost := parsedHost.getHost()
			validationOptions := host.GetValidationOptions()
			if len(strHost) == 0 {
				addrs = []*IPAddress{}
			} else {
				var ips []net.IP
				ips, lookupErr := net.LookupIP(strHost)
				if lookupErr != nil {
					//Note we do not set resolveData, so we will attempt to resolve again
					err = &hostNameNestedError{nested: lookupErr,
						hostNameError: hostNameError{addressError{str: strHost, key: "ipaddress.host.error.host.resolve"}}}
					return
				}
				count := len(ips)
				addrs = make([]*IPAddress, 0, count)
				var errs []addrerr.AddressError
				for j := 0; j < count; j++ {
					ip := ips[j]
					if ipv4 := ip.To4(); ipv4 != nil {
						ip = ipv4
					}
					networkPrefixLength := parsedHost.getNetworkPrefixLen()
					byteLen := len(ip)
					if networkPrefixLength == nil {
						mask := parsedHost.getMask()
						if mask != nil {
							maskBytes := mask.Bytes()
							if len(maskBytes) == byteLen {
								for i := 0; i < byteLen; i++ {
									ip[i] &= maskBytes[i]
								}
								networkPrefixLength = mask.GetBlockMaskPrefixLen(true)
							}
						}
					}
					if byteLen == IPv6ByteCount {
						ipv6Addr, addrErr := NewIPv6AddressFromPrefixedBytes(ip, networkPrefixLength) // addrerr.AddressValueError
						if addrErr != nil {
							errs = append(errs, addrErr)
						} else {
							cache := ipv6Addr.cache
							if cache != nil {
								cache.identifierStr = &IdentifierStr{host}
							}
							addrs = append(addrs, ipv6Addr.ToIP())
						}
					} else if byteLen == IPv4ByteCount {
						if networkPrefixLength != nil && networkPrefixLength.bitCount() > IPv4BitCount {
							networkPrefixLength = cacheBitCount(IPv4BitCount)
						}
						ipv4Addr, addrErr := NewIPv4AddressFromPrefixedBytes(ip, networkPrefixLength) // addrerr.AddressValueError
						if addrErr != nil {
							errs = append(errs, addrErr)
						} else {
							cache := ipv4Addr.cache
							if cache != nil {
								cache.identifierStr = &IdentifierStr{host}
							}
							addrs = append(addrs, ipv4Addr.ToIP())
						}
					}
				}
				if len(errs) > 0 {
					err = &mergedError{AddressError: &hostNameError{addressError{str: strHost, key: "ipaddress.host.error.host.resolve"}}, merged: errs}
				}
				count = len(addrs)
				if count > 0 {
					// sort by preferred version
					preferredVersion := IPVersion(validationOptions.GetPreferredVersion())
					boundaryCase := 8
					if count > boundaryCase {
						c := 0
						newAddrs := make([]*IPAddress, count)
						for _, val := range addrs {
							if val.getIPVersion() == preferredVersion {
								newAddrs[c] = val
								c++
							}
						}
						for i := 0; c < count; i++ {
							val := addrs[i]
							if val.getIPVersion() != preferredVersion {
								newAddrs[c] = val
								c++
							}
						}
						addrs = newAddrs
					} else {
						preferredIndex := 0
					top:
						for i := 0; i < count; i++ {
							notPreferred := addrs[i]
							if notPreferred.getIPVersion() != preferredVersion {
								var j int
								if preferredIndex == 0 {
									j = i + 1
								} else {
									j = preferredIndex
								}
								for ; j < len(addrs); j++ {
									preferred := addrs[j]
									if preferred.getIPVersion() == preferredVersion {
										addrs[i] = preferred
										// don't swap so the non-preferred order is preserved,
										// instead shift each upwards by one spot
										k := i + 1
										for ; k < j; k++ {
											addrs[k], notPreferred = notPreferred, addrs[k]
										}
										addrs[k] = notPreferred
										preferredIndex = j + 1
										continue top
									}
								}
								// no more preferred
								break
							}
						}
					}
				}
				fmt.Printf("resolved addrs %v\n", addrs)
				fmt.Println()
			}
		}
		data = &resolveData{addrs, err}
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&host.resolveData))
		atomic.StorePointer(dataLoc, unsafe.Pointer(data))
	}
	return data.resolvedAddrs, nil
}

func (host *HostName) IsValid() bool {
	return host.init().Validate() == nil
}

func (host *HostName) AsAddressString() *IPAddressString {
	host = host.init()
	if host.IsAddressString() {
		return host.parsedHost.asGenericAddressString()
	}
	return nil
}

func (host *HostName) GetPort() Port {
	host = host.init()
	if host.IsValid() {
		return host.parsedHost.getPort().copy()
	}
	return nil
}

func (host *HostName) GetService() string {
	host = host.init()
	if host.IsValid() {
		return host.parsedHost.getService()
	}
	return ""
}

// ToNormalizedString provides a normalized string which is lowercase for host strings, and which is the normalized string for addresses.
func (host *HostName) ToNormalizedString() string {
	host = host.init()
	str := host.normalizedString
	if str == nil {
		newStr := host.toNormalizedString(false, false)
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&host.normalizedString))
		str = &newStr
		atomic.StorePointer(dataLoc, unsafe.Pointer(str))
	}
	return *str
}

// ToNormalizedString provides a normalized string which is lowercase for host strings, and which is a normalized string for addresses.
func (host *HostName) ToNormalizedWildcardString() string {
	host = host.init()
	str := host.normalizedWildcardString
	if str == nil {
		newStr := host.toNormalizedString(false, false)
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&host.normalizedWildcardString))
		str = &newStr
		atomic.StorePointer(dataLoc, unsafe.Pointer(str))
	}
	return *str
}

// ToNormalizedString provides a normalized string which is lowercase for host strings, and which is a normalized string for addresses.
func (host *HostName) ToQualifiedString() string {
	host = host.init()
	str := host.qualifiedString
	if str == nil {
		newStr := host.toNormalizedString(false, true)
		dataLoc := (*unsafe.Pointer)(unsafe.Pointer(&host.qualifiedString))
		str = &newStr
		atomic.StorePointer(dataLoc, unsafe.Pointer(str))
	}
	return *str
}

func (host *HostName) toNormalizedString(wildcard, addTrailingDot bool) string {
	if host.IsValid() {
		var builder strings.Builder
		if host.IsAddress() {
			toNormalizedHostString(host.AsAddress(), wildcard, &builder)
		} else if host.IsAddressString() {
			builder.WriteString(host.AsAddressString().ToNormalizedString())
		} else {
			builder.WriteString(host.parsedHost.getHost())
			if addTrailingDot {
				builder.WriteByte(LabelSeparator)
			}
			/*
			 * If prefix or mask is supplied and there is an address, it is applied directly to the address provider, so
			 * we need only check for those things here
			 *
			 * Also note that ports and prefix/mask cannot appear at the same time, so this does not interfere with the port code below.
			 */
			networkPrefixLength := host.parsedHost.getEquivalentPrefixLen()
			if networkPrefixLength != nil {
				builder.WriteByte(PrefixLenSeparator)
				toUnsignedString(uint64(networkPrefixLength.bitCount()), 10, &builder)
			} else {
				mask := host.parsedHost.getMask()
				if mask != nil {
					builder.WriteByte(PrefixLenSeparator)
					builder.WriteString(mask.ToNormalizedString())
				}
			}
		}
		port := host.parsedHost.getPort()
		if port != nil {
			toNormalizedPortString(port.portNum(), &builder)
		} else {
			service := host.parsedHost.getService()
			if service != "" {
				builder.WriteByte(PortSeparator)
				builder.WriteString(string(service))
			}
		}
		return builder.String()
	}
	return host.str
}

func toNormalizedPortString(port PortInt, builder *strings.Builder) {
	builder.WriteByte(PortSeparator)
	toUnsignedString(uint64(port), 10, builder)
}

func toNormalizedHostString(addr *IPAddress, wildcard bool, builder *strings.Builder) {
	if addr.isIPv6() {
		if !wildcard && addr.IsPrefixed() { // prefix needs to be outside the brackets
			normalized := addr.ToNormalizedString()
			index := strings.IndexByte(normalized, PrefixLenSeparator)
			builder.WriteByte(IPv6StartBracket)
			translateReserved(addr.ToIPv6(), normalized[:index], builder)
			builder.WriteByte(IPv6EndBracket)
			builder.WriteString(normalized[index:])
		} else {
			normalized := addr.ToNormalizedWildcardString()
			builder.WriteByte(IPv6StartBracket)
			translateReserved(addr.ToIPv6(), normalized, builder)
			builder.WriteByte(IPv6EndBracket)
		}
	} else {
		if wildcard {
			builder.WriteString(addr.ToNormalizedWildcardString())
		} else {
			builder.WriteString(addr.ToNormalizedString())
		}
	}
}

func toNormalizedAddrPortString(addr *IPAddress, port PortInt) string {
	builder := strings.Builder{}
	toNormalizedHostString(addr, false, &builder)
	toNormalizedPortString(port, &builder)
	return builder.String()
}

// Equal returns true if the given host name matches this one.
// For hosts to match, they must represent the same addresses or have the same host names.
// Hosts are not resolved when matching.  Also, hosts must have the same port or service.
// They must have the same masks if they are host names.
// Even if two hosts are invalid, they match if they have the same invalid string.
func (host *HostName) Equal(other *HostName) bool {
	if host == nil {
		return other == nil
	} else if other == nil {
		return false
	}
	host = host.init()
	other = other.init()
	if host == other {
		return true
	}
	if host.IsValid() {
		if other.IsValid() {
			parsedHost := host.parsedHost
			otherParsedHost := other.parsedHost
			if parsedHost.isAddressString() {
				return otherParsedHost.isAddressString() &&
					parsedHost.asGenericAddressString().Equal(otherParsedHost.asGenericAddressString()) &&
					parsedHost.getPort().Equal(otherParsedHost.getPort()) &&
					parsedHost.getService() == otherParsedHost.getService()
			}
			if otherParsedHost.isAddressString() {
				return false
			}
			thisHost := parsedHost.getHost()
			otherHost := otherParsedHost.getHost()
			if thisHost != otherHost {
				return false
			}
			return parsedHost.getEquivalentPrefixLen().Equal(otherParsedHost.getEquivalentPrefixLen()) &&
				parsedHost.getMask().Equal(otherParsedHost.getMask()) &&
				parsedHost.getPort().Equal(otherParsedHost.getPort()) &&
				parsedHost.getService() == otherParsedHost.getService()
		}
		return false
	}
	return !other.IsValid() && host.String() == other.String()
}

// GetNormalizedLabels returns an array of normalized strings for this host name instance.
//
// If this represents an IP address, the address segments are separated into the returned array.
// If this represents a host name string, the domain name segments are separated into the returned array,
// with the top-level domain name (right-most segment) as the last array element.
//
// The individual segment strings are normalized in the same way as ToNormalizedString.
//
// Ports, service name strings, prefix lengths, and masks are all omitted from the returned array.
func (host *HostName) GetNormalizedLabels() []string {
	host = host.init()
	if host.IsValid() {
		return host.parsedHost.getNormalizedLabels()
	} else {
		str := host.str
		if len(str) == 0 {
			return []string{}
		}
		return []string{str}
	}
}

// GetHost returns the host string normalized but without port, service, prefix or mask.
//
// If an address, returns the address string normalized, but without port, service, prefix, mask, or brackets for IPv6.
//
// To get a normalized string encompassing all details, use toNormalizedString()
//
// If not a valid host, returns the zero string
func (host *HostName) GetHost() string {
	host = host.init()
	if host.IsValid() {
		return host.parsedHost.getHost()
	}
	return ""
}

/*
TODO LATER isUNCIPv6Literal and isReverseDNS
*/
///**
// * Returns whether this host name is an Uniform Naming Convention IPv6 literal host name.
// *
// * @return
// */
//public boolean isUNCIPv6Literal() {
//	return isValid() && parsedHost.isUNCIPv6Literal();
//}
//
///**
// * Returns whether this host name is a reverse DNS string host name.
// *
// * @return
// */
//public boolean isReverseDNS() {
//	return isValid() && parsedHost.isReverseDNS();
//}

// GetNetworkPrefixLen returns the prefix length, if a prefix length was supplied,
// either as part of an address or as part of a domain (in which case the prefix applies to any resolved address).
// Otherwise, GetNetworkPrefixLen returns nil.
func (host *HostName) GetNetworkPrefixLen() PrefixLen {
	if host.IsAddress() {
		addr, err := host.parsedHost.asAddress()
		if err == nil {
			return addr.getNetworkPrefixLen().copy()
		}
	} else if host.IsAddressString() {
		return host.parsedHost.asGenericAddressString().getNetworkPrefixLen().copy()
	} else if host.IsValid() {
		return host.parsedHost.getEquivalentPrefixLen().copy()
	}
	return nil
}

// GetMask returns the resulting mask value if a mask was provided with this host name.
func (host *HostName) GetMask() *IPAddress {
	if host.IsValid() {
		if host.parsedHost.isAddressString() {
			return host.parsedHost.getAddressProvider().getProviderMask()
		}
		return host.parsedHost.getMask()
	}
	return nil
}

// ResolvesToSelf returns whether this represents, or resolves to,
// a host or address representing the same host.
func (host *HostName) ResolvesToSelf() bool {
	if host.IsSelf() {
		return true
	} else if host.GetAddress() != nil {
		host.resolvedAddrs[0].IsLoopback()
	}
	return false
}

// IsSelf returns whether this represents a host or address representing the same host.
// Also see isLocalHost and IsLoopback
func (host *HostName) IsSelf() bool {
	return host.IsLocalHost() || host.IsLoopback()
}

// IsLocalHost returns whether this host is "localhost"
func (host *HostName) IsLocalHost() bool {
	return host.IsValid() && strings.EqualFold(host.str, "localhost")
}

// IsLoopback returns whether this host has the loopback address, such as
// [::1] (aka [0:0:0:0:0:0:0:1]) or 127.0.0.1
//
// Also see isSelf()
func (host *HostName) IsLoopback() bool {
	return host.IsAddress() && host.AsAddress().IsLoopback()
}

// ToTCPAddrService returns the TCPAddr if this HostName both resolves to an address and has an associated service or port, otherwise returns nil
func (host *HostName) ToNetTCPAddrService(serviceMapper func(string) Port) *net.TCPAddr {
	if host.IsValid() {
		port := host.GetPort()
		if port == nil && serviceMapper != nil {
			service := host.GetService()
			if service != "" {
				port = serviceMapper(service)
			}
		}
		if port != nil {
			if addr := host.GetAddress(); addr != nil {
				return &net.TCPAddr{
					IP:   addr.GetNetIP(),
					Port: port.portNum(),
					Zone: string(addr.zone),
				}
			}
		}
	}
	return nil
}

// ToTCPAddr returns the TCPAddr if this HostName both resolves to an address and has an associated port.
// Otherwise, it returns nil.
func (host *HostName) ToNetTCPAddr() *net.TCPAddr {
	return host.ToNetTCPAddrService(nil)
}

// ToUDPAddrService returns the UDPAddr if this HostName both resolves to an address and has an associated service or port
func (host *HostName) ToNetUDPAddrService(serviceMapper func(string) Port) *net.UDPAddr {
	tcpAddr := host.ToNetTCPAddrService(serviceMapper)
	if tcpAddr != nil {
		return &net.UDPAddr{
			IP:   tcpAddr.IP,
			Port: tcpAddr.Port,
			Zone: tcpAddr.Zone,
		}
	}
	return nil
}

// ToUDPAddr returns the UDPAddr if this HostName both resolves to an address and has an associated port
func (host *HostName) ToNetUDPAddr(serviceMapper func(string) Port) *net.UDPAddr {
	return host.ToNetUDPAddrService(serviceMapper)
}

func (host *HostName) ToNetIP() net.IP {
	if addr, err := host.ToAddress(); addr != nil && err == nil {
		return addr.GetNetIP()
	}
	return nil
}

func (host *HostName) ToNetIPAddr() *net.IPAddr {
	if addr, err := host.ToAddress(); addr != nil && err == nil {
		return &net.IPAddr{
			IP:   addr.GetNetIP(),
			Zone: string(addr.zone),
		}
	}
	return nil
}

// Compare returns a negative integer, zero, or a positive integer if this host name is less than, equal, or greater than the given host name.
// Any address item is comparable to any other.
func (host *HostName) Compare(other *HostName) int {
	if host == other {
		return 0
	} else if host == nil {
		return -1
	} else if other == nil {
		return 1
	}
	if host.IsValid() {
		if other.IsValid() {
			parsedHost := host.parsedHost
			otherParsedHost := other.parsedHost
			if parsedHost.isAddressString() {
				if otherParsedHost.isAddressString() {
					result := parsedHost.asGenericAddressString().Compare(otherParsedHost.asGenericAddressString())
					if result != 0 {
						return result
					}
					//fall through to compare ports
				} else {
					return -1
				}
			} else if otherParsedHost.isAddressString() {
				return 1
			} else {
				//both are non-address hosts
				normalizedLabels := parsedHost.getNormalizedLabels()
				otherNormalizedLabels := otherParsedHost.getNormalizedLabels()
				oneLen := len(normalizedLabels)
				twoLen := len(otherNormalizedLabels)
				var minLen int
				if oneLen < twoLen {
					minLen = oneLen
				} else {
					minLen = twoLen
				}
				for i := 1; i <= minLen; i++ {
					one := normalizedLabels[oneLen-i]
					two := otherNormalizedLabels[twoLen-i]
					result := strings.Compare(one, two)
					if result != 0 {
						return result
					}
				}
				if oneLen != twoLen {
					return oneLen - twoLen
				}

				//keep in mind that hosts can has masks/prefixes or ports, but not both
				networkPrefixLength := parsedHost.getEquivalentPrefixLen()
				otherPrefixLength := otherParsedHost.getEquivalentPrefixLen()
				if networkPrefixLength != nil {
					if otherPrefixLength != nil {
						if *networkPrefixLength != *otherPrefixLength {
							return int(otherPrefixLength.bitCount() - networkPrefixLength.bitCount())
						}
						//fall through to compare ports
					} else {
						return 1
					}
				} else {
					if otherPrefixLength != nil {
						return -1
					}
					mask := parsedHost.getMask()
					otherMask := otherParsedHost.getMask()
					if mask != nil {
						if otherMask != nil {
							ret := mask.Compare(otherMask)
							if ret != 0 {
								return ret
							}
							//fall through to compare ports
						} else {
							return 1
						}
					} else {
						if otherMask != nil {
							return -1
						}
						//fall through to compare ports
					}
				} //end non-address host compare
			}

			//two equivalent address strings or two equivalent hosts, now check port and service names
			portOne := parsedHost.getPort()
			portTwo := otherParsedHost.getPort()
			portRet := portOne.Compare(portTwo)
			if portRet != 0 {
				return portRet
			}
			serviceOne := parsedHost.getService()
			serviceTwo := otherParsedHost.getService()
			if serviceOne != "" {
				if serviceTwo != "" {
					ret := strings.Compare(serviceOne, serviceTwo)
					if ret != 0 {
						return ret
					}
				} else {
					return 1
				}
			} else if serviceTwo != "" {
				return -1
			}
			return 0
		} else {
			return 1
		}
	} else if other.IsValid() {
		return -1
	}
	return strings.Compare(host.String(), other.String())
}

func (host *HostName) Wrap() ExtendedIdentifierString {
	return WrappedHostName{host}
}

func translateReserved(addr *IPv6Address, str string, builder *strings.Builder) {
	//This is particularly targeted towards the zone
	if !addr.HasZone() {
		builder.WriteString(str)
		return
	}
	index := strings.IndexByte(str, IPv6ZoneSeparator)
	var translated = builder
	translated.WriteString(str[0:index])
	translated.WriteString("%25")
	for i := index + 1; i < len(str); i++ {
		c := str[i]
		if isReserved(c) {
			translated.WriteByte('%')
			toUnsignedString(uint64(c), 16, translated)
		} else {
			translated.WriteByte(c)
		}
	}
}
