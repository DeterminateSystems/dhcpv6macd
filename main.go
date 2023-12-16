package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/server6"
	"github.com/insomniacslk/dhcp/iana"
)

type DHCPv6Handler struct {
	baseAddress net.IP
	serverDuid  dhcpv6.DUIDLLT
}

var (
	baseAddress      = flag.String("base-address", "fec0::", "IPv6 base address to distribute MAC-based IPs through, we assume its a /72")
	networkInterface = flag.String("interface", "eth0", "Interface to listen on")
)

// Handler implements a server6.Handler.
func (h *DHCPv6Handler) Handler(conn net.PacketConn, peer net.Addr, m dhcpv6.DHCPv6) {
	err := h.handleMsg(conn, peer, m)
	if err != nil {
		log.Printf(err.Error())
	}
}

func (s *DHCPv6Handler) handleMsg(conn net.PacketConn, peer net.Addr,
	req dhcpv6.DHCPv6) (err error) {

	msg, err := req.GetInnerMessage()
	if err != nil {
		err = fmt.Errorf("DHCPv6 get inner message error: %s", err)
		return
	}

	err = s.checkClientID(msg)
	if err != nil {
		log.Printf("missing client ID")
		return
	}

	err = s.checkServerID(msg)
	if err != nil {
		log.Printf("error checking serverID: %s", err)
		return
	}

	var resp dhcpv6.DHCPv6
	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit:
		rapidCommit := msg.GetOneOption(dhcpv6.OptionRapidCommit)
		if rapidCommit != nil {
			resp, err = dhcpv6.NewReplyFromMessage(msg)
			if err != nil {
				err = fmt.Errorf("DHCPv6 new reply from message error: %s", err)
				return
			}
		} else {
			resp, err = dhcpv6.NewAdvertiseFromSolicit(msg)
			if err != nil {
				err = fmt.Errorf("DHCPv6 new advertise from solicit error: %s", err)
				return
			}
		}
		break
	case dhcpv6.MessageTypeRequest, dhcpv6.MessageTypeConfirm,
		dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind,
		dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeInformationRequest:

		resp, err = dhcpv6.NewReplyFromMessage(msg)
		if err != nil {
			err = fmt.Errorf("DHCPv6 new reply from message error: %s", err)
			return
		}
		break
	default:
		err = fmt.Errorf("Unknown DHCPv6 message type")
		return
	}

	resp.AddOption(dhcpv6.OptServerID(&s.serverDuid))

	err = s.process(msg, req, resp)
	if err != nil {
		return
	}

	fmt.Printf("Peer: %s\n", peer.String())
	fmt.Println(resp.Summary())

	_, err = conn.WriteTo(resp.ToBytes(), peer)
	if err != nil {
		err = fmt.Errorf("DHCPv6 reply write error: %s", err)
		return
	}

	return
}

// Check Client ID
func (s *DHCPv6Handler) checkClientID(msg *dhcpv6.Message) error {
	if msg.Options.ClientID() == nil {
		return fmt.Errorf("dhcpv6: no ClientID option in request")
	}

	return nil
}

// Check the message has a matching server ID
func (s *DHCPv6Handler) checkServerID(msg *dhcpv6.Message) error {
	sid := msg.Options.ServerID()

	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit,
		dhcpv6.MessageTypeConfirm,
		dhcpv6.MessageTypeRebind:

		if sid != nil {
			return fmt.Errorf("dhcpv6: drop packet: ServerID option in message %s", msg.Type().String())
		}
	case dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRelease,
		dhcpv6.MessageTypeDecline:
		if sid == nil {
			return fmt.Errorf("dhcpv6: drop packet: no ServerID option in message %s", msg.Type().String())
		}

		if !sid.Equal(&s.serverDuid) {
			return fmt.Errorf("dhcpv6: drop packet: mismatched ServerID option in message %s: %s",
				msg.Type().String(), sid.String())
		}
	}

	return nil
}

func (s *DHCPv6Handler) checkIA(msg *dhcpv6.Message, expectedIP net.IP) error {
	switch msg.Type() {
	case dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeConfirm,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind:

		oia := msg.Options.OneIANA()
		if oia == nil {
			return fmt.Errorf("no IANA option in %s", msg.Type().String())
		}

		oiaAddr := oia.Options.OneAddress()
		if oiaAddr == nil {
			return fmt.Errorf("no IANA.Addr option in %s", msg.Type().String())
		}

		if !oiaAddr.IPv6Addr.Equal(expectedIP) {
			return fmt.Errorf("invalid IANA.Addr option in %s", msg.Type().String())
		}
	}
	return nil
}

func (s *DHCPv6Handler) process(msg *dhcpv6.Message,
	req, resp dhcpv6.DHCPv6) (err error) {

	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit, dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeConfirm, dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind:

		break
	default:
		err = fmt.Errorf("DHCPv6 ignore message type %s", msg.Type())
		return
	}

	var leasedIP net.IP

	mac, err := dhcpv6.ExtractMAC(msg)
	if err != nil {
		log.Printf("No MAC address in request: %v", err)
		return fmt.Errorf("No MAC address")
	}
	leasedIP = append(s.baseAddress[:10], mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	log.Printf("Assigning %v to %v", leasedIP, mac)

	err = s.checkIA(msg, leasedIP)
	if err != nil {
		return fmt.Errorf("error checking the IA: %s", err)
	}

	oia := &dhcpv6.OptIANA{
		T1: 600 * time.Second,
		T2: 1050 * time.Second,
	}

	roia := msg.Options.OneIANA()
	if roia != nil {
		copy(oia.IaId[:], roia.IaId[:])
	} else {
		copy(oia.IaId[:], []byte("DSYS"))
	}

	oiaAddr := &dhcpv6.OptIAAddress{
		IPv6Addr:          leasedIP,
		PreferredLifetime: 600 * time.Second,
		ValidLifetime:     600 * time.Second,
	}

	oia.Options = dhcpv6.IdentityOptions{
		Options: []dhcpv6.Option{
			oiaAddr,
		},
	}

	resp.AddOption(oia)

	fqdn := msg.GetOneOption(dhcpv6.OptionFQDN)
	if fqdn != nil {
		resp.AddOption(fqdn)
	}

	resp.AddOption(&dhcpv6.OptStatusCode{
		StatusCode:    iana.StatusSuccess,
		StatusMessage: "success",
	})

	return
}

func main() {
	flag.Parse()

	iface, err := net.InterfaceByName(*networkInterface)
	if err != nil {
		log.Fatalf("finding interface %s by name: %s", *networkInterface, err)
		return
	}

	dhcpv6Handler := DHCPv6Handler{
		baseAddress: net.ParseIP(*baseAddress),
		serverDuid: dhcpv6.DUIDLLT{
			HWType:        iana.HWTypeEthernet,
			LinkLayerAddr: iface.HardwareAddr,
			Time:          dhcpv6.GetTime(),
		},
	}

	listenAddr := &net.UDPAddr{
		IP:   dhcpv6.AllDHCPRelayAgentsAndServers,
		Port: dhcpv6.DefaultServerPort,
		Zone: *baseAddress,
	}

	laddr := &net.UDPAddr{
		IP:   net.IPv6unspecified,
		Port: dhcpv6.DefaultServerPort,
	}

	server, err := server6.NewServer(*networkInterface, laddr, dhcpv6Handler.Handler)
	if err != nil {
		fmt.Printf("starting DHCPv6 server: %s", err)
	}

	log.Printf("listening via UDP on %s", listenAddr)

	server.Serve()
}
