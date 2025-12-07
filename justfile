interface := "en0"
ip := "127.0.0.1"
repo_root := justfile_directory()
scratch := justfile_directory() + "/.scratch"
ipxe_result_link := scratch + "/ipxe-result"


run: make-pxe-efi make-cert make-samples
    go run . \
        -interface "{{interface}}" \
        -netboot-dir "{{scratch}}/samples" \
        -tls-cert-file "{{scratch}}/tls.crt" \
        -tls-key-file "{{scratch}}/tls.key" \
        -dhcpv6-listen-port 20547 \
        -http-listen-addr "{{ip}}:20080" \
        -https-listen-addr "{{ip}}:20443" \
        -tftp-listen-addr "{{ip}}:20069" \
        -ipxe-x86-64-efi "{{ipxe_result_link}}/ipxe.efi"

make-pxe-efi: make-scratch
    #!/bin/sh
    if [ ! -e "{{ipxe_result_link}}/ipxe.efi" ]; then
        nix build .#iPXE --out-link "{{ipxe_result_link}}"
    else
        echo "ipxe.efi: not re-building since it already exists in .scratch"
    fi

make-cert: make-scratch
    #!/bin/sh
    if [ ! -e "{{scratch}}/tls.key" ] || [ ! -e "{{scratch}}/tls.crt" ]; then
        openssl req -x509 \
            -newkey rsa:4096 \
            -nodes \
            -keyout "{{scratch}}/tls.key" \
            -out "{{scratch}}/tls.crt" \
            -sha256 \
            -days 1 \
            -subj "/CN=dhcpv6macd"
    else
        echo "scratch TLS key/cert: not recreating them because they exist already"
    fi


make-samples: make-scratch
    #!/bin/sh
    mkdir -p "{{scratch}}/samples/00:00:00:00:00:00"
    cd "{{scratch}}/samples/00:00:00:00:00:00"
    if [ ! -e "boot.efi" ]; then
        yes "dhcpv6macd is a choice" | head -c 52428800 > boot.efi
    else
        echo "00:00:00:00:00:00 sample: not creating it because it already exists"
    fi

make-scratch:
    mkdir -p "{{scratch}}"
