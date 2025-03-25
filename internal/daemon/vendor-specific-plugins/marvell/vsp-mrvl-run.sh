#!/usr/bin/bash

set -x

load_driver() {
    if [ -z "$(lsmod | grep -w octeon_ep)" ] ; then
        chroot /host modprobe octeon_ep
        echo "Loaded octeon_ep driver"
        sleep 1
    fi
}

unload_driver() {
    if [ -n "$(lsmod | grep -w octeon_ep)" ] ; then
        rmmod octeon_ep -f
        echo "Unloaded octeon_ep drivers, wait for 20 seconds before retry"
        sleep 20
    fi
}

detect_link() {
    pf="0000:$(lspci -d 177d:b900 -n | awk 'NR==1{print $1}')"
    path="/sys/bus/pci/devices/$pf/net/"
    ifname="$(ls -1 "$path" 2>/dev/null | sed -n 1p)" || return 1

    ip link set "$ifname" up || return 1

    for i in {1..15} ; do
        state="$(cat "/sys/class/net/$ifname/operstate")"
        if [ "$state" == "up" ] ; then
            echo "PF found: $ifname"
            return 0
        fi
        sleep 0.1
    done

    return 1
}

setup_host_link() {
    while : ; do
        if detect_link ; then
            echo "Link is up"
            break
        fi
        echo "Link is down"
        unload_driver
        load_driver
    done
}

setup_hugepages() {
    if ! mount | grep -q "^none on /dev/huge type hugetlbfs " ; then
        mkdir -p /dev/huge
        mount -t hugetlbfs none /dev/huge
    fi
}

setup_dpu_link() {
    setup_hugepages

    chroot /host modprobe vfio-pci

    dpi="$(lspci -d 177d:a080 -n | awk 'NR==1{print $1}')"
    if [ -z "$dpi" ] ; then
        echo "DPI device not found"
        return 1
    fi
    echo "DPI device: $dpi"

    pem="$(lspci -d 177d:a06c -n | awk 'NR==1{print $1}')"
    if [ -z "$pem" ] ; then
        echo "PEM device not found"
        return 1
    fi
    echo "PEM device: $pem"

    echo vfio-pci > "/sys/bus/pci/devices/$dpi/driver_override" && echo "$dpi" > /sys/bus/pci/drivers_probe
    echo vfio-pci > "/sys/bus/pci/devices/$pem/driver_override" && echo "$pem" > /sys/bus/pci/drivers_probe

    /usr/bin/octep_cp_agent /usr/bin/cn106xx.cfg -- --dpi_dev $dpi --pem_dev $pem &
}


run() {
    if [ "$(lspci -d 177d:a0f7)" != "" ] ; then
        mode="dpu"
    else
        mode="host"
    fi

    echo "mode: $mode"

    if [ "$mode" = "host" ] ; then
        setup_host_link
    elif [ "$mode" = "dpu" ] ; then
        setup_dpu_link
        if [ $? -ne 0 ]; then
            echo "Failed to set up CP Agent"
            return
        fi
    fi

    /vsp-mrvl
}

run
