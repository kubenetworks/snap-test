package core

import (
	"context"

	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/packetsocket"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/raw"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

func NewStack(ctx context.Context, tun stack.LinkEndpoint) *stack.Stack {
	nicID := tcpip.NICID(1)
	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
		},
		Clock:                    tcpip.NewStdClock(),
		AllowPacketEndpointWrite: true,
		HandleLocal:              false, // if set to true, ping local ip will fail
		// Enable raw sockets for users with sufficient
		// privileges.
		RawFactory: raw.EndpointFactory{},
	})
	// set handler for TCP UDP ICMP
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, TCPForwarder(s, ctx))
	s.SetTransportProtocolHandler(udp.ProtocolNumber, UDPForwarder(s, ctx))
	s.SetTransportProtocolHandler(header.ICMPv4ProtocolNumber, ICMPForwarder(s, ctx))
	s.SetTransportProtocolHandler(header.ICMPv6ProtocolNumber, ICMPForwarder(s, ctx))

	s.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         nicID,
		},
		{
			Destination: header.IPv6EmptySubnet,
			NIC:         nicID,
		},
	})

	s.CreateNICWithOptions(nicID, packetsocket.New(tun), stack.NICOptions{
		Disabled: false,
		Context:  ctx,
	})
	s.SetPromiscuousMode(nicID, true)
	s.SetSpoofing(nicID, true)

	// Enable SACK Recovery.
	{
		opt := tcpip.TCPSACKEnabled(true)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			log.Fatalf("SetTransportProtocolOption(%d, &%T(%t)): %v", tcp.ProtocolNumber, opt, opt, err)
		}
	}

	// Set default TTLs as required by socket/netstack.
	{
		opt := tcpip.DefaultTTLOption(64)
		if err := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
			log.Fatalf("SetNetworkProtocolOption(%d, &%T(%d)): %v", ipv4.ProtocolNumber, opt, opt, err)
		}
		if err := s.SetNetworkProtocolOption(ipv6.ProtocolNumber, &opt); err != nil {
			log.Fatalf("SetNetworkProtocolOption(%d, &%T(%d)): %v", ipv6.ProtocolNumber, opt, opt, err)
		}
	}

	// Enable Receive Buffer Auto-Tuning.
	{
		opt := tcpip.TCPModerateReceiveBufferOption(true)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			log.Fatalf("SetTransportProtocolOption(%d, &%T(%t)): %v", tcp.ProtocolNumber, opt, opt, err)
		}
	}

	{
		if err := s.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true); err != nil {
			log.Fatalf("Set IPv4 forwarding: %v", err)
		}
		if err := s.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, true); err != nil {
			log.Fatalf("Set IPv6 forwarding: %v", err)
		}
	}

	{
		option := tcpip.TCPModerateReceiveBufferOption(true)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &option); err != nil {
			log.Fatalf("Set TCP moderate receive buffer: %v", err)
		}
	}
	return s
}
