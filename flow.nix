package:
{ flow }:

flow.new {
  about = {
    name = "detsys/dhcpv6macd";
    description = "foo";
    flakeref = "https://flakehub.com/f/DeterminateSystems/minnows-flow-dhcpv6macd/0.1";
    tags = [
    ];
    # docs = ./README.md;
    docs = "FOO";

    examples = [
      # {
      #   title = "Enable SSH with a single SSH key";
      #   tags = [ ];
      #   text = ./example1.md;
      # }
    ];
  };

  capabilities = {
    requiredCapabilities.runAsRoot = true;

    optionalCapabilities.allowNonNativeABIs = true;
    optionalCapabilities.controlGroups = true;
    optionalCapabilities.fullDeviceAccess = true;
    optionalCapabilities.fullFilesystemAccess = true;
    optionalCapabilities.fullNetworkAccess = true;
    optionalCapabilities.kernelLogs = true;
    optionalCapabilities.kernelModules = true;
    optionalCapabilities.kernelTunables = true;
    optionalCapabilities.mutableClock = true;
    optionalCapabilities.mutableHostname = true;
    optionalCapabilities.realtimeClock = true;
    optionalCapabilities.writableNixStore = true;
    optionalCapabilities.writeExecuteMemory = true;
  };

  resources = {
    listeningPorts.dhcpv6_tcp = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "tcp";
        port = 547;
      };
    };
    listeningPorts.dhcpv6_udp = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "udp";
        port = 547;
      };
    };
    listeningPorts.tftp = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "udp";
        port = 69;
      };
    };
    listeningPorts.sse = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "tcp";
        port = 6315;
      };
    };
    listeningPorts.http80 = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "tcp";
        port = 80;
      };
    };
    listeningPorts.http443 = {
      description = "Port to listen on";
      example = {
        family = null;
        protocol = "tcp";
        port = 443;
      };
    };
  };

  interface = {
    interface = {
      type = flow.lib.types.nullOr flow.lib.types.str;
      default = null;
    };

    baseAddress = {
      type = flow.lib.types.str;
    };

    httpBootUrlTemplate = {
      type = flow.lib.types.str;
      default = "https://[{{.BaseAddress}}]/mac/{{.MAC}}/boot.efi?payload={{.Payload}}";
    };

    # httpBootRootCertificate = {
    #   type = flow.lib.types.nullOr flow.lib.types.path;
    #   default = null;
    # };

    netbootDirectory = {
      type = flow.lib.types.nullOr flow.lib.types.path;
      default = null;
    };

    tlsCertFile = {
      type = flow.lib.types.nullOr flow.lib.types.path;
      default = null;
    };

    tlsKeyFile = {
      type = flow.lib.types.nullOr flow.lib.types.path;
      default = null;
    };
  };

  implementation =
    {
      lib,
      this,
      flowContext,
      pkgs,
      resources,
      ...
    }:
    let
    in
    {
      systemd.services.dhcpv6macd = {
        Service = {
          ExecStart = lib.escapeShellArgs [
            "${package}/bin/dhcpv6macd"
            "-interface"
            this.interface
            "-base-address"
            this.baseAddress
            "-http-boot-url-template"
            this.httpBootUrlTemplate
            # "-tls-cert-file"
            # this.tlsCertFile
            # "-tls-key-file"
            # this.tlsKeyFile
            "-netboot-dir"
            this.netbootDirectory
          ];
        };
      };
    };
}

