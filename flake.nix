{
  description = "dhcpv6macd";

  # Nixpkgs / NixOS version to use.
  inputs.nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1";
  inputs.nix-filter.url = "github:numtide/nix-filter";

  outputs =
    {
      self,
      nixpkgs,
      nix-filter,
    }:
    let

      # to work with older version of flakes
      lastModifiedDate = self.lastModifiedDate or self.lastModified or "19700101";

      # Generate a user-friendly version number.
      version = builtins.substring 0 8 lastModifiedDate;

      # System types to support.
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];

      # Helper function to generate an attrset '{ x86_64-linux = f "x86_64-linux"; ... }'.
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Nixpkgs instantiated for supported system types.
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });

      iPxePatches = [
        # Note: these patches come from the integration branch of https://github.com/DeterminateSystems/ipxe
        ./ipxe/0001-ipxe-cmdline.patch
      ];
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              codespell
              eclint
              go
              go-tools
              nixfmt
              just
            ];
          };
        }
      );

      lib = {
        mkiPXE =
          {
            system,
            certBundle ? null,
          }:
          (nixpkgsFor.${system}.ipxe.override {
            embedScript = ./ipxe/dhcpv6-httpboot.ipxe;
          }).overrideAttrs
            (oldAttrs: {
              patches = (if oldAttrs ? patches then oldAttrs.patches else [ ]) ++ iPxePatches;

              makeFlags = oldAttrs.makeFlags ++ (nixpkgs.lib.optional (certBundle != null) "TRUST=${certBundle}");
            });
      };

      # Provide some binary packages for selected system types.
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgsFor.${system};
        in
        (
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

              goSum = ./go.sum;
              vendorHash = "sha256-7H8eyzGHrvyDXCtB39u42mqn682qaE8Q64T4ImyWCpY=";
            };
          }
          // (
            if (system == "aarch64-darwin") then
              { }
            else
              {
                iPXE = self.lib.mkiPXE { inherit system; };
              }
          )
        )
      );

      nixosModules = {
        dhcpv6macd =
          {
            pkgs,
            config,
            lib,
            ...
          }:
          let
            cfg = config.services.detsys.dhcpv6macd;
          in
          {
            options = {
              services.detsys.dhcpv6macd = {
                enable = lib.mkEnableOption "DHCPv6MACd";
                baseAddress = lib.mkOption {
                  type = lib.types.str;
                  description = ''
                    The IPv6 address to start with when issuing IP addresses.
                    The prefix is assumed to be at least a /80.
                    The MAC address is simply concatenated onto the prefix.
                  '';
                };
                interface = lib.mkOption {
                  type = lib.types.nullOr lib.types.str;
                  description = ''
                    The name of the network interface to listen on.
                  '';
                };
                tftpListenAddress = lib.mkOption {
                  type = lib.types.str;
                  default = ":69";
                  description = ''
                    The address to listen on for TFTP requests.
                  '';
                };
                httpListenAddress = lib.mkOption {
                  type = lib.types.str;
                  default = ":80";
                  description = ''
                    The address to listen on for HTTP requests.
                  '';
                };
                httpsListenAddress = lib.mkOption {
                  type = lib.types.str;
                  default = ":443";
                  description = ''
                    The address to listen on for HTTPS requests.
                  '';
                };
                dhcpv6ListenPort = lib.mkOption {
                  type = lib.types.int;
                  default = 547;
                  description = ''
                    The port to listen on for DHCPv6 requests.
                  '';
                };
                httpBootUrlTemplate = lib.mkOption {
                  type = lib.types.str;
                  default = "";
                  description = ''
                    `http://[{{.BaseAddress}}]/?mac={{.MAC}}&payload={{.Payload}}`
                  '';
                };
                httpBootRootCertificate = lib.mkOption {
                  type = lib.types.nullOr lib.types.path;
                  default = null;
                  description = ''
                    Path to a root CA certificate to embed in the iPXE binary for HTTPS boot URL validation.
                    Required when httpBootUrlTemplate uses HTTPS and the server certificate is not signed by a well-known CA.
                  '';
                };
                netbootDirectory = lib.mkOption {
                  type = lib.types.nullOr lib.types.path;
                  description = ''
                    `/netboot/mac`
                  '';
                };
                tlsCertFile = lib.mkOption {
                  type = lib.types.nullOr lib.types.path;
                  default = null;
                  description = ''
                    Location of netboot TLS cert PEM.
                  '';
                };
                tlsKeyFile = lib.mkOption {
                  type = lib.types.nullOr lib.types.path;
                  default = null;
                  description = ''
                    Location of netboot TLS key PEM.
                  '';
                };
                ipxeX8664Efi = lib.mkOption {
                  type = lib.types.nullOr lib.types.path;
                  default =
                    (self.lib.mkiPXE {
                      system = "x86_64-linux";
                      certBundle = cfg.httpBootRootCertificate;
                    })
                    + "/ipxe.efi";

                  description = ''
                    Path to the iPXE EFI binary for x86_64 to serve over TFTP.
                  '';
                };
              };
            };
            config =
              let
                package = self.packages."${pkgs.stdenv.system}".default;
              in
              lib.mkIf cfg.enable {
                networking.firewall.interfaces."${cfg.interface}" = {
                  allowedUDPPorts = [ cfg.dhcpv6ListenPort ];
                  allowedTCPPorts = [ cfg.dhcpv6ListenPort ];
                };

                systemd.services.dhcpv6macd = {
                  wantedBy = [ "multi-user.target" ];
                  serviceConfig = {
                    DynamicUser = true;
                    AmbientCapabilities = "CAP_NET_BIND_SERVICE";
                    ProtectSystem = "strict";
                    ExecStart =
                      "${package}/bin/dhcpv6macd "
                      + (lib.escapeShellArgs [
                        "-base-address"
                        cfg.baseAddress
                        "-interface"
                        cfg.interface
                        "-tftp-listen-addr"
                        cfg.tftpListenAddress
                        "-http-listen-addr"
                        cfg.httpListenAddress
                        "-https-listen-addr"
                        cfg.httpsListenAddress
                        "-dhcpv6-listen-port"
                        (toString cfg.dhcpv6ListenPort)
                        "-http-boot-url-template"
                        cfg.httpBootUrlTemplate
                        "-tls-cert-file"
                        cfg.tlsCertFile
                        "-tls-key-file"
                        cfg.tlsKeyFile
                        "-netboot-dir"
                        cfg.netbootDirectory
                        "-ipxe-x86-64-efi"
                        cfg.ipxeX8664Efi
                      ]);
                  };
                };
              };
          };
      };

      checks.x86_64-linux.package = self.packages.x86_64-linux.default;
      checks.x86_64-linux.nixostest-basic =
        (import (nixpkgs + "/nixos/lib/testing-python.nix") { system = "x86_64-linux"; }).simpleTest
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
                netbootDirectory = "/netboot/mac";
                ipxeX8664Efi = builtins.toFile "ipxe.efi" "its-ipxe\n";
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
              client.succeed("sleep 5")

              eth1_addrs = client.succeed("ip -6 addr show eth1")
              assert "fd19:287e:c5a0:4931:0:2de:adbe:ef01" in eth1_addrs, "Did not find expected client IPv6 addr"
            '';
          };

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
