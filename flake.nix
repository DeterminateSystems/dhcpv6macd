{
  description = "dhcpv6macd";

  # Nixpkgs / NixOS version to use.
  inputs.nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1";
  inputs.nix-filter.url = "github:numtide/nix-filter";

  outputs = { self, nixpkgs, nix-filter }:
    let

      # to work with older version of flakes
      lastModifiedDate = self.lastModifiedDate or self.lastModified or "19700101";

      # Generate a user-friendly version number.
      version = builtins.substring 0 8 lastModifiedDate;

      # System types to support.
      supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];

      # Helper function to generate an attrset '{ x86_64-linux = f "x86_64-linux"; ... }'.
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Nixpkgs instantiated for supported system types.
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });

    in
    {
      devShells = forAllSystems
        (system:
          let
            pkgs = nixpkgsFor.${system};
          in
          {
            default = pkgs.mkShell
              {
                buildInputs = with pkgs; [
                  codespell
                  eclint
                  go
                  nixpkgs-fmt
                ];
              };
          });

      # Provide some binary packages for selected system types.
      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "dhcpv6macd";
            inherit version;

            src = nix-filter.lib {
              root = ./.;
              include = [
                "go.mod"
                "go.sum"
                (nix-filter.lib.matchExt "go")
              ];
            };

            # This hash locks the dependencies of this package. It is
            # necessary because of how Go requires network access to resolve
            # VCS.  See https://www.tweag.io/blog/2021-03-04-gomod2nix/ for
            # details. Normally one can build with a fake sha256 and rely on native Go
            # mechanisms to tell you what the hash should be or determine what
            # it should be "out-of-band" with other tooling (eg. gomod2nix).
            # To begin with it is recommended to set this, but one must
            # remember to bump this hash when your dependencies change.
            #vendorSha256 = pkgs.lib.fakeSha256;

            vendorHash = "sha256-KzJxL5K7WZIeaBdhV3OKBYSSjox3G+4hF5PxKeXCRKY=";

            goSum = ./go.sum;
          };
        });

      nixosModules = {
        dhcpv6macd = { pkgs, config, lib, ... }: {
          options = {
            services.detsys.dhcpv6macd = {
              enable = lib.mkEnableOption "DHCPv6MACd";
              interface = lib.mkOption {
                type = lib.types.nullOr lib.types.str;
                description = lib.mdDoc ''
                  The name of the network interface to listen on.
                '';
              };
              baseAddress = lib.mkOption {
                type = lib.types.str;
                description = lib.mdDoc ''
                  The IPv6 address to start with when issuing IP addresses.
                  The prefix is assumed to be at least a /80.
                  The MAC address is simply concatenated onto the prefix.
                '';
              };
            };
          };
          config = let cfg = config.services.detsys.dhcpv6macd; in lib.mkIf cfg.enable {
            networking.firewall.interfaces."${cfg.interface}" = {
              allowedUDPPorts = [ 547 ];
              allowedTCPPorts = [ 547 ];
            };

            systemd.services.dhcpv6macd = {
              wantedBy = [ "multi-user.target" ];
              serviceConfig = {
                DynamicUser = true;
                AmbientCapabilities = "CAP_NET_BIND_SERVICE";
                ProtectSystem = "strict";
                ExecStart = "${self.packages."${pkgs.stdenv.system}".default}/bin/dhcpv6macd "
                  + (lib.escapeShellArgs [
                  "-interface"
                  cfg.interface
                  "-base-address"
                  cfg.baseAddress
                ]);
              };
            };
          };
        };
      };

      checks.x86_64-linux.package = self.packages.x86_64-linux.default;
      checks.x86_64-linux.nixostest-basic = (import (nixpkgs + "/nixos/lib/testing-python.nix") { system = "x86_64-linux"; }).simpleTest
        {
          name = "basic";
          nodes.router = {
            imports = [
              self.nixosModules.dhcpv6macd
            ];

            services.detsys.dhcpv6macd = {
              enable = true;
              interface = "eth1";
              baseAddress = "fd19:287e:c5a0:4931::";
            };

            networking.useNetworkd = true;
            systemd.network.networks = {
              "10-ds-macnet" = {
                matchConfig.Name = "eth1";

                address = [
                  "fd19:287e:c5a0:4931::/64"
                  "fe80::1/64"
                ];

                networkConfig = {
                  DHCPServer = true;
                  IPv6SendRA = true;
                };
                ipv6SendRAConfig.Managed = true;
                linkConfig.RequiredForOnline = false;
              };
            };
          };

          nodes.client = {
            systemd.network.networks."40-eth1" = {
              dhcpV6Config.DUIDType = "link-layer";
            };
            networking = {
              useNetworkd = true;
              useDHCP = false;
              interfaces.eth1 = {
                useDHCP = true;
                macAddress = "02:de:ad:be:ef:01";
              };
            };
          };

          testScript = ''
            router.wait_for_unit("network.target")
            router.wait_for_unit("dhcpv6macd.service")
            router.succeed("systemd-cat ip addr")


            client.wait_for_unit("network.target")
            client.wait_for_unit("systemd-networkd-wait-online.service", timeout=30)
            client.succeed("systemd-cat networkctl status eth1")

            eth1_addrs = client.succeed("ip -6 addr show eth1")
            assert "fd19:287e:c5a0:4931:0:2de:adbe:ef01" in eth1_addrs, "Did not find expected client IPv6 addr"
          '';
        };
    };
}

