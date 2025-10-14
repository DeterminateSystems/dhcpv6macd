# DHCPv6 server for difficult clients

This is a probably not RFC-compliant DHCPv6 server for a very narrow use case: assigning fixed IPv6 addresses based entirely on the client's MAC address.
Note that this DHCPv6 server does not respect or account for leases.

You should only use this if:

- You have clients you can't configure to use regular SLAAC with addresses following the usual EUI-64 scheme

- You still need these clients to have stable IP addresses, even if they don't keep any state (frequent OS wipes!)

We use this for macOS and a HITL where reprovisioning isn't 100% reliable, so we want them to get the same IP address even if _none of the scripts have run_.

Notes:

- this daemon may or may not work correctly with relay agent forwarding.
- this daemon will not try to issue HTTP Boot instructions if the template is an empty string.
  If you don't want that, feel free to PR it into an optional setting.

## Usage

```sh
go build .
sudo ./dhcpv6macd -interface enp2s0 -base-address 2001:db8:0123:4567:: -http-boot-url-template 'http://netboot.target/?mac={{.MAC}}'
```

...where the template can use these parameters:

- `MAC` -- the MAC address of the netbooting device
- `BaseAddress` -- the same value as passed in the CLI arguments
- `Payload` -- a base64 encoded JSON blob about the booting device, for example `eyJhcmNoaXRlY3R1cmVzIjpbIkVGSSB4ODYtNjQgYm9vdCBmcm9tIEhUVFAiXX0=` which is decodes to:

```json
{
  "architectures": ["EFI x86-64 boot from HTTP"]
}
```

There is also a NixOS module in the flake.nix.

See the `flake.nix` for a NixOS test involving router and a client.

### iPXE Chaining

The daemon also listens on port 69/udp for serving TFTP requests.
Currently, all requests that arrive over TFTP will be served the exact same `ipxe.efi`, which is compiled in to the binary.
In the future, it may expand to support other architectures.

The ipxe.efi binary is only served if the client requests PXE booting.
The daemon tells the client to fetch ipxe from `tftp://[baseAddr]/clientMacAddr/ipxe.efi`.
When iPXE starts, it automatically starts dhcp again, and it will chain to the templatized HTTP boot url option.

### HTTP SSE Events

The daemon also listens on port 6315/tcp for HTTP traffic.
Send a `GET /events` request and get a stream of DHCPv6 events.
The `/events` endpoint also supports filtering on a MAC address, ie: `GET /events?mac=00:00:00:00:00:00`.

The server immediately sends the events the server is aware of at the start:

```
data: {"01:01:01:01:01:01":{"Mac":"01:01:01:01:01:01","Events":[{"event":"init","timestamp":"2025-10-13T20:08:24.39495055Z"}, ...]}}
```

Or if you specified a MAC to filter on, it'll return the object directly (or `null` if the mac isn't present in the event table):

```
data: {"Mac":"01:01:01:01:01:01","Events":[{"event":"init","timestamp":"2025-10-13T20:01:16.698695404Z"},{"event":"point_pxe_to_ipxe_over_tftp","timestamp":"2025-10-13T20:01:16.699618993Z"},{"event":"served_ipxe_over_tftp","timestamp":"2025-10-13T20:01:22.946541642Z"},{"event":"point_ipxe_to_http_boot","timestamp":"2025-10-13T20:01:30.972207315Z"},{"event":"os_init","timestamp":"2025-10-13T20:01:50.065743799Z"}]}
```

Then, events will be served as they arrive:

```
data: {"mac":"01:01:01:01:01:01","event":{"event":"init","timestamp":"2025-10-13T20:01:16.698785748Z"}}

data: {"mac":"01:01:01:01:01:01","event":{"event":"point_pxe_to_ipxe_over_tftp","timestamp":"2025-10-13T20:01:16.699634041Z"}}

data: {"mac":"01:01:01:01:01:01","event":{"event":"served_ipxe_over_tftp","timestamp":"2025-10-13T20:01:22.946572526Z"}}
```

### Root certificate tweaking

The NixOS module exposes an option to set the root CA certificate for HTTPS chaining.
Setting that option rebuilds ipxe with that certificate as the root trust anchor.

The built ipxe.efi is then copied in to the build directory of dhcpv6macd for compiling in to the daemon.

## Address allocation scheme

The prefix is assumed to be at least a /80.
The MAC address is simply concatenated onto the prefix.

If the DHCPv6 Solicit request does not have a MAC address, we fall back to loading the MAC from an eui64 link-local IP.
Note that this only works if the Soliciting system encodes their MAC in their link local address via EUI-64 (privacy/stable-privacy LLAs won’t work.)

## What's not inside

This does not provide:

- Router advertisements (we use systemd-networkd for this)
- DHCPv4 (we use systemd-networkd for this)
- Options for DNS, NTP, etc

## License

This code is GPL-3, and based on Adguard's AdGuardHome DHCPv6 server: https://github.com/AdguardTeam/AdGuardHome/blob/167b1125113c86e6304471d80d983c17f0f707e3/internal/dhcpd/v6_unix.go.
