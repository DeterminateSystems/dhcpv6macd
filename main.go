package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"text/template"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/server6"
	"github.com/insomniacslk/dhcp/iana"

	"github.com/mdlayher/netx/eui64"
	"github.com/pin/tftp/v3"
)

//go:embed ipxe.efi
var ipxe_efi_x86_64 []byte

// DHCPv6Handler offers DHCPv6 addresses based on the requester's MAC address.
type DHCPv6Handler struct {
	baseAddress net.IP
	serverDuid  dhcpv6.DUIDLL
}

var (
	baseAddress         = flag.String("base-address", "fec0::", "IPv6 base address to distribute MAC-based IPs through, we assume its a /72")
	networkInterface    = flag.String("interface", "eth0", "Interface to listen on")
	httpBootURLTemplate = flag.String("http-boot-url-template", "", "URL template for HTTP boot requests, like http://netboot.target/?mac={{.MAC}}")
)

var httpBootTemplate *template.Template

// Handler implements a server6.Handler.
func (s *DHCPv6Handler) Handler(conn net.PacketConn, peer net.Addr, m dhcpv6.DHCPv6) {
	err := s.handleMsg(conn, peer, m)
	if err != nil {
		log.Printf("error handling a message: %s", err.Error())
	}
}

// getMACFromPeer attempts to extract a MAC address from the peer's IPv6 address
// by parsing it as an EUI-64 link-local address (fe80::/64 with MAC in the lower 64 bits).
// This only works if the client uses EUI-64 addressing; privacy addresses (RFC 4941) will fail.
func getMACFromPeer(peer net.Addr) (net.HardwareAddr, error) {
	peerUDPAddr, ok := peer.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("peer %T is not a *net.UDPAddr", peer)
	}

	ip := peerUDPAddr.IP.To16()
	if ip == nil {
		return nil, fmt.Errorf("constructing a 16-byte IP failed: %s", peerUDPAddr)
	}

	if !ip.IsLinkLocalUnicast() {
		return nil, fmt.Errorf("peer's IP (%s) isn't link-local (with the fe80:: prefix)", ip)
	}

	_, mac, err := eui64.ParseIP(ip)
	if err != nil {
		return nil, fmt.Errorf("parsing the IP (%s) into eui64 format failed: %s", ip, err)
	}

	return mac, nil
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
	case dhcpv6.MessageTypeRequest, dhcpv6.MessageTypeConfirm,
		dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind,
		dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeInformationRequest:

		resp, err = dhcpv6.NewReplyFromMessage(msg)
		if err != nil {
			err = fmt.Errorf("DHCPv6 new reply from message error: %s", err)
			return
		}
	default:
		err = fmt.Errorf("unknown DHCPv6 message type")
		return
	}

	resp.AddOption(dhcpv6.OptServerID(&s.serverDuid))

	err = s.process(peer, msg, req, resp)
	if err != nil {
		return
	}

	log.Printf("Peer: %s", peer.String())
	log.Println(resp.Summary())

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

func wantsHttpBootFile(msg *dhcpv6.Message) bool {
	for _, vc := range msg.Options.VendorClasses() {
		for _, data := range vc.Data {
			if bytes.HasPrefix(data, []byte("HTTPClient")) {
				return true
			}
		}
	}

	return false
}

func isPxeClient(msg *dhcpv6.Message) bool {
	for _, vc := range msg.Options.VendorClasses() {
		for _, data := range vc.Data {
			if bytes.HasPrefix(data, []byte("PXEClient")) {
				return true
			}
		}
	}

	return false
}

func isUserIpxe(msg *dhcpv6.Message) bool {
	for _, uc := range msg.Options.UserClasses() {
		if bytes.HasPrefix(uc, []byte("iPXE")) {
			return true
		}
	}

	return false
}

func wantsiPxeOverTftp(msg *dhcpv6.Message) bool {
	if !isPxeClient(msg) {
		return false
	}

	if isUserIpxe(msg) {
		return false
	}

	return true
}

func wantsiPxeChainToHttp(msg *dhcpv6.Message) bool {
	return isPxeClient(msg) && isUserIpxe(msg)
}

type archPayload struct {
	Architectures []string `json:"architectures"`
}

func archsToStrings(archs iana.Archs) []string {
	out := make([]string, 0, len(archs))
	for _, a := range archs {
		out = append(out, a.String()) // e.g. "EFI_X86_64", "EFI_IA32"
	}
	return out
}

func archsToJSON(archs iana.Archs) (string, error) {
	p := archPayload{Architectures: archsToStrings(archs)}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func archsToEncoded(archs iana.Archs) (string, error) {
	j, err := archsToJSON(archs)
	if err != nil {
		return "", err
	}
	sEnc := base64.StdEncoding.EncodeToString([]byte(j))
	return sEnc, nil
}

func (s *DHCPv6Handler) process(peer net.Addr, msg *dhcpv6.Message,
	req, resp dhcpv6.DHCPv6) (err error) {

	log.Println("Message received", msg, msg.Options)

	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit, dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeConfirm, dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind:

		break
	case dhcpv6.MessageTypeRelease:
		resp.AddOption(&dhcpv6.OptStatusCode{
			StatusCode:    iana.StatusSuccess,
			StatusMessage: "success",
		})
		return
	default:
		err = fmt.Errorf("DHCPv6 ignore message type %s", msg.Type())
		return
	}

	var leasedIP net.IP

	mac, err := dhcpv6.ExtractMAC(msg)
	if err != nil {
		mac, err = getMACFromPeer(peer)
		if err != nil {
			return fmt.Errorf("MAC extraction failed (not in DHCPv6 options, nor is it available from the peer address %s): %w", peer, err)
		}
	}

	prefix := make([]byte, 10)
	copy(prefix, s.baseAddress[:10])
	leasedIP = append(prefix, mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])

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
		PreferredLifetime: 1200 * time.Second,
		ValidLifetime:     1200 * time.Second,
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

	if msg.IsOptionRequested(dhcpv6.OptionDNSRecursiveNameServer) {
		dhcpv6.WithDNS(net.ParseIP("2606:4700:4700::1111"), net.ParseIP("2001:4860:4860::8888"))(resp)
	}

	if httpBootTemplate != nil {
		if wantsHttpBootFile(msg) {
			payload, err := archsToEncoded(msg.Options.ArchTypes())
			if err != nil {
				log.Printf("failed to construct the arch payload: %s", err)
			}

			var buf bytes.Buffer
			if err := httpBootTemplate.Execute(&buf, map[string]string{
				"MAC":         mac.String(),
				"BaseAddress": *baseAddress,
				"Payload":     payload,
			}); err != nil {
				log.Printf("failed to render the http boot template: %v", err)
			} else {
				resp.AddOption(&dhcpv6.OptVendorClass{
					EnterpriseNumber: 0,
					Data:             [][]byte{[]byte("HTTPClient")},
				})

				// ref: https://lenovopress.lenovo.com/lp0736.pdf
				resp.AddOption(dhcpv6.OptBootFileURL(buf.String()))
			}
		} else if wantsiPxeOverTftp(msg) {
			resp.AddOption(dhcpv6.OptBootFileURL(fmt.Sprintf("tftp://[%s]/%s/ipxe.efi", *baseAddress, mac.String())))
		} else if wantsiPxeChainToHttp(msg) {

			payload, err := archsToEncoded(msg.Options.ArchTypes())
			if err != nil {
				log.Printf("failed to construct the arch payload: %s", err)
			}

			var buf bytes.Buffer
			if err := httpBootTemplate.Execute(&buf, map[string]string{
				"MAC":         mac.String(),
				"BaseAddress": *baseAddress,
				"Payload":     payload,
			}); err != nil {
				log.Printf("failed to render the http boot template: %v", err)
			} else {
				resp.AddOption(dhcpv6.OptBootFileURL(buf.String()))
			}
		}
	}

	resp.AddOption(&dhcpv6.OptStatusCode{
		StatusCode:    iana.StatusSuccess,
		StatusMessage: "success",
	})

	return
}

func tftpReadHandler(filename string, rf io.ReaderFrom) error {
	log.Println("Serving ", filename)
	r := bytes.NewReader(ipxe_efi_x86_64)
	n, err := rf.ReadFrom(r)
	if err != nil {
		log.Printf("Serving failure: %v", err)
		return err
	}
	log.Printf("%d bytes sent for %s", n, filename)
	return nil

}

func main() {
	flag.Parse()

	var err error

	if *httpBootURLTemplate != "" {
		httpBootTemplate, err = template.New("httpBootURL").Parse(*httpBootURLTemplate)
		if err != nil {
			log.Fatalf("failed to parse template: %v", err)
		}
	}

	iface, err := net.InterfaceByName(*networkInterface)
	if err != nil {
		log.Fatalf("finding interface %s by name: %s", *networkInterface, err)
		return
	}

	parsedBaseIP := net.ParseIP(*baseAddress)
	if parsedBaseIP == nil && parsedBaseIP.To4() != nil {
		log.Fatalf("invalid IPv6 base-address: %s", *baseAddress)
	}

	dhcpv6Handler := DHCPv6Handler{
		baseAddress: parsedBaseIP,
		serverDuid: dhcpv6.DUIDLL{
			HWType:        iana.HWTypeEthernet,
			LinkLayerAddr: iface.HardwareAddr,
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

	go func() {
		log.Printf("Starting the TFTP server on port 69")
		tftpServer := tftp.NewServer(tftpReadHandler, nil)
		tftpServer.SetTimeout(5 * time.Second) // optional
		err = tftpServer.ListenAndServe(":69") // blocks until s.Shutdown() is called
		if err != nil {
			log.Fatalf("starting TFTP server: %v", err)
		}
	}()

	server, err := server6.NewServer(*networkInterface, laddr, dhcpv6Handler.Handler)
	if err != nil {
		log.Fatalf("starting DHCPv6 server: %s", err)
	}

	log.Printf("listening via UDP on %s", listenAddr)
	if err = server.Serve(); err != nil {
		log.Fatalf("DHCPv6 server exited: %v", err)
	}

}
