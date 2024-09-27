detect_dpu_kind() {
    local HOSTNAME="$(hostname -f)"

    # We detect the dpu-kind based on the well-known host names.
    #
    # In the future, we may use the google sheet instead. However, note that
    # cluster-deployment-automation also fills in data from the google sheet
    # based on the host name. The detection is thus also based on the host
    # name, except it's encoded in a google sheet.

    case "$HOSTNAME" in
        "wsfd-advnetlab44.anl.eng.bos2.dc.redhat.com" | \
        "wsfd-advnetlab41.anl.eng.bos2.dc.redhat.com" )
            echo "marvell-dpu"
            ;;
        *)
            echo "dpu"
            ;;
    esac
}
