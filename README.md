# DHCPv6 server for difficult clients

This is a probably not RFC-compliant DHCPv6 server for a very narrow use case: assigning fixed IPv6 addresses based entirely on the client's MAC address.
Note that this DHCPv6 server does not respect or account for leases.

You should only use this if:

- You have clients you can't configure to use regular SLAAC with addresses following the usual EUI-64 scheme

- You still need these clients to have stable IP addresses, even if they don't keep any state (frequent OS wipes!)


We use this for macs where reprovisioning isn't 100% reliable, so we want them to get the same IP address even if _none of the scripts have run_.
This is why we wrote this horrible hack.


## Usage

```
go build .
sudo ./dhcpv6macd -interface enp2s0 -base-address 2001:db8:0123:4567::
```

See the `flake.nix` for a NixOS test involving router and a client.

## Address allocation scheme

The prefix is assumed to be at least a /80.
The MAC address is simply concatenated onto the prefix.

## What's not inside

This does not provide:

- Router advertisements (we use systemd-networkd for this)
- DHCPv4 (we use systemd-networkd for this)
- Options for DNS, NTP, etc

## License

This code is GPL-3, and based on Adguard's AdGuardHome DHCPv6 server: https://github.com/AdguardTeam/AdGuardHome/blob/master/internal/dhcpd/v6_unix.go.
