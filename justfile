interface := "en0"
ip := "127.0.0.1"
scratch := justfile_directory() + "/.scratch"



run: make-samples make-cert
    go run . \
        -interface "{{interface}}" \
        -netboot-dir "{{scratch}}/samples" \
        -tls-cert-file "{{scratch}}/tls.crt" \
        -tls-key-file "{{scratch}}/tls.key" \


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
            -nodes \
            -subj "/CN=dhcpv6macd"
    else
        echo "scratch TLS key/cert: not recreating them because they exist already"
    fi


make-samples: make-scratch
    #!/bin/sh
    mkdir -p "{{scratch}}/samples/00:00:00:00:00:00"
    cd "{{scratch}}/samples/00:00:00:00:00:00"
    if [ ! -e "boot.efi" ]; then
        yes "dhcpv6macd is a choice" | head --bytes=50MB > boot.efi
    else
        echo "00:00:00:00:00:00 sample: not creating it because it already exists"
    fi

make-scratch:
    mkdir -p "{{scratch}}"
