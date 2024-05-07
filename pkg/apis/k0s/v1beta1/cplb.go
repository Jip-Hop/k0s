/*
Copyright 2024 k0s authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"errors"
	"fmt"
	"net"
)

// Defaults are keepalived's defaults.
const defaultVirtualRouterID = 51
const defaultAdvertInterval = 1

// ControlPlaneLoadBalancingSpec defines the configuration options related to k0s's
// keepalived feature.
type ControlPlaneLoadBalancingSpec struct {
	// Indicates if control plane load balancing should be enabled.
	// Default: false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// type indicates the type of the node-local load balancer to deploy on
	// worker nodes. Currently, the only supported type is "Keepalived".
	// +kubebuilder:default=Keepalived
	// +optional
	Type CPLBType `json:"type,omitempty"`

	// Keepalived contains configuration options related to the "Keepalived" type
	// of load balancing.
	Keepalived *KeepalivedSpec `json:"keepalived,omitempty"`
}

// NllbType describes which type of load balancer should be deployed for the
// node-local load balancing. The default is [CPLBTypeKeepalived].
// +kubebuilder:validation:Enum=Keepalived
type CPLBType string

const (
	// CPLBTypeKeepalived selects Keepalived as the backing load balancer.
	CPLBTypeKeepalived CPLBType = "Keepalived"
)

type KeepalivedSpec struct {
	// Configuration options related to the VRRP. This is an array which allows
	// to configure multiple virtual IPs.
	VRRPInstances VRRPInstances `json:"vrrpInstances,omitempty"`
	// Configuration options related to the virtual servers. This is an array
	// which allows to configure multiple load balancers.
	VirtualServers VirtualServers `json:"virtualServers,omitempty"`
}

// VRRPInstances is a list of VRRPInstance
type VRRPInstances []VRRPInstance

// VRRPInstance defines the configuration options for a VRRP instance.
type VRRPInstance struct {
	// VirtualIP is the list virtual IP address used by the VRRP instance. VirtualIPs
	// must be a CIDR as defined in RFC 4632 and RFC 4291.
	VirtualIPs VirtualIPs `json:"virtualIPs,omitempty"`

	// Interface specifies the NIC used by the virtual router. If not specified,
	// k0s will use the interface that owns the default route.
	Interface string `json:"interface,omitempty"`

	// VirtualRouterID is the VRRP router ID. If not specified, defaults to 51.
	// VirtualRouterID must be in the range of 1-255, all the control plane
	// nodes must have the same VirtualRouterID.
	// Two clusters in the same network must not use the same VirtualRouterID.
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=255
	//+kubebuilder:default=51
	VirtualRouterID *int32 `json:"virtualRouterID,omitempty"`

	// AdvertInterval is the advertisement interval in seconds. If not specified,
	// use 1 second
	//+kubebuilder:default=1
	AdvertInterval *int32 `json:"advertInterval,omitempty"`

	// AuthPass is the password for accessing vrrpd. This is not a security
	// feature but a way to prevent accidental misconfigurations.
	// Authpass must be 8 characters or less.
	AuthPass string `json:"authPass"`
}

type VirtualIPs []string

// validateVRRPInstances validates existing configuration and sets the default
// values of undefined fields.
func (k *KeepalivedSpec) validateVRRPInstances(getDefaultNICFn func() (string, error)) []error {
	errs := []error{}
	if getDefaultNICFn == nil {
		getDefaultNICFn = getDefaultNIC
	}
	for i := range k.VRRPInstances {
		if k.VRRPInstances[i].Interface == "" {
			nic, err := getDefaultNICFn()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get default NIC: %w", err))
			}
			k.VRRPInstances[i].Interface = nic
		}

		if k.VRRPInstances[i].VirtualRouterID == nil {
			vrid := int32(defaultVirtualRouterID + i)
			k.VRRPInstances[i].VirtualRouterID = &vrid
		} else if *k.VRRPInstances[i].VirtualRouterID < 0 || *k.VRRPInstances[i].VirtualRouterID > 255 {
			errs = append(errs, errors.New("VirtualRouterID must be in the range of 1-255"))
		}

		if k.VRRPInstances[i].AdvertInterval == nil {
			advInt := int32(defaultAdvertInterval)
			k.VRRPInstances[i].AdvertInterval = &advInt
		}

		if k.VRRPInstances[i].AuthPass == "" {
			errs = append(errs, errors.New("AuthPass must be defined"))
		}
		if len(k.VRRPInstances[i].AuthPass) > 8 {
			errs = append(errs, errors.New("AuthPass must be 8 characters or less"))
		}

		if len(k.VRRPInstances[i].VirtualIPs) == 0 {
			errs = append(errs, errors.New("VirtualIPs must be defined"))
		}
		for _, vip := range k.VRRPInstances[i].VirtualIPs {
			if _, _, err := net.ParseCIDR(vip); err != nil {
				errs = append(errs, fmt.Errorf("VirtualIPs must be a CIDR. Got: %s", vip))
			}
		}
	}
	return errs
}

// VirtualServers is a list of VirtualServer
// +listType=map
// +listMapKey=ipAddress
type VirtualServers []VirtualServer

// VirtualServer defines the configuration options for a virtual server.
type VirtualServer struct {
	// IPAddress is the virtual IP address used by the virtual server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	IPAddress string `json:"ipAddress"`
	// DelayLoop is the delay timer for check polling. If not specified, defaults to 0.
	// kubebuilder:validation:Minimum=0
	DelayLoop int `json:"delayLoop,omitempty"`
	// LBAlgo is the load balancing algorithm. If not specified, defaults to rr.
	// Valid values are rr, wrr, lc, wlc, lblc, dh, sh, sed, nq. For further
	// details refer to keepalived documentation.
	// +kubebuilder:default=rr
	// +optional
	LBAlgo KeepalivedLBAlgo `json:"lbAlgo,omitempty"`
	// LBKind is the load balancing kind. If not specified, defaults to DR.
	// Valid values are NAT DR TUN. For further details refer to keepalived documentation.
	// +kubebuilder:default=DR
	// +optional
	LBKind KeepalivedLBKind `json:"lbKind,omitempty"`
	// PersistenceTimeoutSeconds specify a timeout value for persistent connections in
	// seconds. If not specified, defaults to 360 (6 minutes).
	// kubebuilder:validation:Minimum=0
	PersistenceTimeoutSeconds int `json:"persistenceTimeoutSeconds,omitempty"`
}

// KeepalivedLBAlgo describes the load balancing algorithm.
// +kubebuilder:validation:Enum=rr;wrr;lc;wlc;lblc;dh;sh;sed;nq
type KeepalivedLBAlgo string

const (
	RRAlgo   KeepalivedLBAlgo = "rr"
	WRRAlgo  KeepalivedLBAlgo = "wrr"
	LCAlgo   KeepalivedLBAlgo = "lc"
	WLCAlgo  KeepalivedLBAlgo = "wlc"
	LBLCAlgo KeepalivedLBAlgo = "lblc"
	DHAlgo   KeepalivedLBAlgo = "dh"
	SHAlgo   KeepalivedLBAlgo = "sh"
	SEDAlgo  KeepalivedLBAlgo = "sed"
	NQAlgo   KeepalivedLBAlgo = "nq"
)

// KeepalivedLBKind describes the load balancing forwarding method.
// +kubebuilder:validation:Enum=NAT;DR;TUN
type KeepalivedLBKind string

const (
	NATLBKind KeepalivedLBKind = "NAT"
	DRLBKind  KeepalivedLBKind = "DR"
	TUNLBKind KeepalivedLBKind = "TUN"
)

type RealServer struct {
	// IPAddress is the IP address of the real server.
	IPAddress string `json:"ipAddress"`
	// Weight is the weight of the real server. If not specified, defaults to 1.
	Weight int `json:"weight,omitempty"`
}

// validateVRRPInstances validates existing configuration and sets the default
// values of undefined fields.
func (k *KeepalivedSpec) validateVirtualServers() []error {
	errs := []error{}
	for i := range k.VirtualServers {
		if k.VirtualServers[i].IPAddress == "" {
			errs = append(errs, errors.New("IPAddress must be defined"))
		}
		if net.ParseIP(k.VirtualServers[i].IPAddress) == nil {
			errs = append(errs, fmt.Errorf("invalid IP address: %s", k.VirtualServers[i].IPAddress))
		}

		if k.VirtualServers[i].LBAlgo == "" {
			k.VirtualServers[i].LBAlgo = RRAlgo
		} else {
			switch k.VirtualServers[i].LBAlgo {
			case RRAlgo, WRRAlgo, LCAlgo, WLCAlgo, LBLCAlgo, DHAlgo, SHAlgo, SEDAlgo, NQAlgo:
				// valid LBAlgo
			default:
				errs = append(errs, fmt.Errorf("invalid LBAlgo: %s ", k.VirtualServers[i].LBAlgo))
			}
		}

		if k.VirtualServers[i].LBKind == "" {
			k.VirtualServers[i].LBKind = DRLBKind
		} else {
			switch k.VirtualServers[i].LBKind {
			case NATLBKind, DRLBKind, TUNLBKind:
				// valid LBKind
			default:
				errs = append(errs, fmt.Errorf("invalid LBKind: %s ", k.VirtualServers[i].LBKind))
			}
		}

		if k.VirtualServers[i].PersistenceTimeoutSeconds == 0 {
			k.VirtualServers[i].PersistenceTimeoutSeconds = 360
		} else if k.VirtualServers[i].PersistenceTimeoutSeconds < 0 {
			errs = append(errs, errors.New("PersistenceTimeout must be a positive integer"))
		}

		if k.VirtualServers[i].DelayLoop < 0 {
			errs = append(errs, errors.New("DelayLoop must be a positive integer"))
		}
	}
	return errs
}

// Validate validates the ControlPlaneLoadBalancingSpec
func (c *ControlPlaneLoadBalancingSpec) Validate(externalAddress string) []error {
	if c == nil {
		return nil
	}
	errs := []error{}

	switch c.Type {
	case CPLBTypeKeepalived:
	case "":
		c.Type = CPLBTypeKeepalived
	default:
		errs = append(errs, fmt.Errorf("unsupported CPLB type: %s. Only allowed value: %s", c.Type, CPLBTypeKeepalived))
	}

	errs = append(errs, c.Keepalived.validateVRRPInstances(nil)...)
	errs = append(errs, c.Keepalived.validateVirtualServers()...)
	// CPLB reconciler relies in watching kubernetes.default.svc endpoints
	if externalAddress != "" && len(c.Keepalived.VirtualServers) > 0 {
		errs = append(errs, errors.New(".spec.api.externalAddress and VRRPInstances cannot be used together"))
	}

	return errs
}
