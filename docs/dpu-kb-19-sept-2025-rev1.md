<!-- ANN[th]: Article: https://access.redhat.com/articles/7120276 -->
<!-- ANN[th]: Jira-Issue: https://issues.redhat.com/browse/IIC-887 -->
<!-- ANN[th]: Jira-Issue: https://issues.redhat.com/browse/TELCODOCS-2504 -->
<!-- ANN[th]: Errata v1: https://docs.google.com/document/d/1XEXeKxFQ_Lvqz9PLUoucU7zCUZohpcyBbBdoXvTkMLA/edit?tab=t.0 -->
<!-- ANN[th]: Notes about installing OPILab: https://docs.google.com/document/d/140XMmFQKQorSDLL0IEmtWUDpMNYoF1RECNC8izBKru0/edit?tab=t.0 -->

<!-- ANN[th]: Note: in https://access.redhat.com/articles/7120276, section headings are -->
<!-- ANN[th]:       not linkable (no anchors). Although, there is an index on top. -->

<!-- ANN[th]: Note: The code blocks have a “Raw” link. When you click on them, a -->
<!-- ANN[th]:       new window opens and the text looks raw (good). However, this is not plain -->
<!-- ANN[th]:       text document. If you look at the source of that file, it is HTML. Also, if you click -->
<!-- ANN[th]:       “Save Page As…”, the resulting file is not a usable plain text script. -->

<!-- ANN[th]: Note: there are various pointers to Intel IPU documentation. Would be good to -->
<!-- ANN[th]:       have links. -->

## **Introduction**

This knowledge base details the steps required to deploy a full end-to-end solution with Intel's Infrastructure Processing Unit (Intel®
IPU E2100 Series) integrated into Red Hat OpenShift, the industry-leading hybrid cloud application platform. We will demonstrate how to leverage the OpenShift DPU Operator to offload network functions. Specifically, this involves using F5 NGINX as a reverse proxy for confidential AI workloads running on the host, thereby enhancing application performance and security. Note that most of this knowledge base can be re-used for other workloads as well.

## **Key components and concepts:**

-   **Intel IPU E2100 Series:** An advanced programmable network device featuring an IPU Management Console (IMC) and an Arm-based Compute Complex (ACC). Its Flexible Programmable Packet Processing Engine (FPPE) allows for service function chaining, offloading tasks like firewalls, packet filtering, and compression. This frees host CPU resources and introduces a security boundary.

-   **Arm Compute Complex (ACC):** Runs MicroShift on Red Hat Enterprise Linux (RHEL), enabling management with standard Red Hat tools, treating the IPU like any other server.

-   **OpenShift DPU Operator:** Deploys a daemonset on OpenShift worker nodes. These daemons interface with a daemon on the IPU via a vendor agnostic API that is part of the Open Programmable Infrastructure (OPI) project, managing the lifecycle of IPU workloads through a vendor-agnostic, Kubernetes-native workflow.

-   **Solution Overview:** This knowledge base walks through deploying the F5 NGINX on the IPU. This NGINX instance functions as a reverse proxy, providing access to ResNet application virtual machines (VMs) running on the OpenShift worker nodes.

## **Prerequisites**

Before proceeding, ensure the following prerequisites are met:

-   **Intel IPU E2100 Series Hardware:** Properly installed in your OpenShift worker nodes.

-   **IPU Firmware:** Upgraded to version 2.0.0.11126 or later.

-   **Redfish:**

    -   Enabled and reachable on the IPU.

    -   The Redfish instance must be able to reach the HTTPS server hosting the RHEL ISO for the IPU.

    -   If using self-signed certificates for your HTTPS server, ensure these are trusted by the IPU.

    -   Network Time Protocol (NTP) must be configured on the IPU, as it is a requirement for TLS connections used by Redfish.

-   **IPU Driver Configuration:** The IPU must be configured to use the IDPF driver instead of the default ICC net driver for RHEL
    compatibility. Refer to the official Intel IPU documentation for firmware upgrades, Redfish setup, and driver configuration.

-   **OpenShift Container Platform:** A functional OpenShift cluster minimum version 4.19 must be installed and operational. This knowledge base focuses on worker nodes equipped with IPUs.

-   **Network Connectivity:**

    -   Each network segment (OpenShift cluster, IPU management, IPU data plane) must have DHCP and DNS services.

    -   All components should have internet access for pulling images and packages.

## **Solution architecture and network topology**
[image=[src="images/ipu-dp-ocp-arch_2.png", alt="DPU OpenShift architecture", size="LG - Large", data-cp-size="100%",  ]]

<!-- ANN[th]: Regarding Image^^: -->
<!-- ANN[th]:   - The “VM Workload” term seems not accurate. These are plain containers. -->
<!-- ANN[th]:   - The secondary network interfaces in nginx pod should be called “net1” and “net2” (not “net0” and “net1”) -->
<!-- ANN[th]:   - What is “Normal Container”? -->

The deployment involves three networks:

-   **Red Network (OpenShift Network):** The standard OpenShift cluster network for control plane and primary application traffic. This network connects the OpenShift controllers and worker nodes.

-   **Blue Network (Secondary/Data Plane Network):** A secondary network enabling communication between workloads on the IPU (for example NGINX) and pods/VMs on the host that are attached to this secondary network.

-   **Green Network (Provisioning Network):** A separate network meant to access the management complex of systems

## **Step-by-Step deployment** 

### **Preparing the Intel IPU (Installing RHEL and MicroShift)** 
This section outlines building a RHEL ISO with MicroShift and deploying it to the IPU. 

####1. **Create RHEL for edge image with kickstart:**

Start with a Red Hat Enterprise Linux installation ISO. Booting the plain ISO
requires manual steps during installation. Instead, you can build a custom ISO
that includes a kickstart file to automate the installation and preconfigure
the IPU.

A better alternative is to [create a RHEL for Edge image](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/4.19/html/embedding_in_a_rhel_for_edge_image/index). This produces an immutable operating system with MicroShift already bundled.

> **Important kickstart configuration**
>  Include the following in your kickstart file to enable iSCSI boot for the IPU's Arm Compute Complex (ACC). The 192.168.0.0/24 network is internal to the IPU and should not be used elsewhere.

```
bootloader --location=mbr --driveorder=sda --append="ip=192.168.0.2:::255.255.255.0::enp0s1f0:off netroot=iscsi:192.168.0.1::::iqn.e2000:acc"
```

####2. **Boot ISO on IPU via Redfish:**

Use Redfish virtual media to boot the newly created RHEL for Edge ISO on the IPU. Consult the official Intel IPU documentation for specific Redfish procedures.

####3. **Install and Configure MicroShift:**

If your installation ISO does not include MicroShift, you will need to install
it manually. Once RHEL is installed on the IPU, follow the
[Red Hat build of MicroShift documentation](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/latest/html/installing_with_an_rpm_package/index)
to install MicroShift and any additional required packages.

To enable the Operator Lifecycle Manager (OLM), also install the
`microshift-olm` package.

####4. **Copy P4 artifacts to the DPU (IPU):** 

The P4 program defines the packet processing pipeline on the IPU. Obtain the necessary `intel-ipu-acc-components.tar.gz` (or similarly named archive for example `intel-ipu-acc-components-2.0.0.11126.tar.gz`). 

> **Note**
>  This is not part of the OpenShift DPU Operator, but needs to be obtained from Intel directly.

4.1 Transfer and extract these artifacts to the appropriate location on the IPU's ACC by running these commands:

4.1.2 Copy the `intel-ipu-acc-components-2.0.0.11126.tar.gz` to the ACC: 

         curl -L <URL> -o /tmp/p4.tar.gz

4.1.2 Untar the archive as follows: 

        rm -rf /opt/p4 && mkdir -p /opt/p4

        tar -U -C /opt/p4 -xzf /tmp/p4.tar.gz --strip-components=1

4.1.3 Rename directories for internal purposes as follows:

         mv /opt/p4/p4-cp /opt/p4/p4-cp-nws

         mv /opt/p4/p4-sde /opt/p4/p4sde

####5. **Enable `systemd` daemon to create `hugepages` configuration on the ACC:**

Run the following commands to enable a `systemd` service that sets up `hugepages` on the ACC:

```
cat <<EOF > /etc/systemd/system/hugepages-setup.service
[Unit]
Description=Setup Hugepages
Before=sysinit.target
Before=microshift.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/mkdir -p /dev/hugepages
ExecStart=/bin/mount -t hugetlbfs -o pagesize=2M none /dev/hugepages
ExecStart=/bin/sh -c 'echo 512 > /sys/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages'

[Install]
WantedBy=microshift.service
EOF

systemctl daemon-reload
systemctl enable --now hugepages-setup.service
systemctl restart microshift
```

You can also build the installation ISO to include this systemd service.

Alternatively, you can configure your kickstart file to add the following
kernel parameters: `default_hugepagesz=2M hugepagesz=2M hugepages=512`.

####6. **Reload IDPF driver on Host:**

As a workaround for a known issue, after the IPU is fully set up with RHEL and MicroShift, reload the `idpf` driver on the OpenShift worker node hosting the IPU by running the following commands. If you don’t have an Operating System installed on the host, postpone running these commands until the host has been set up. 

```
sudo rmmod idpf 
sudo modprobe idpf
```

### **Install OpenShift Container Platform** 

Ensure you have a fully operational OpenShift cluster. For installation guidance, refer to the [OpenShift documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest#Install). The [Assisted Installer](https://access.redhat.com/documentation/en-us/assisted_installer_for_openshift_container_platform) method supports various deployment platforms with a focus on bare metal.

### **Install the DPU Operator on your OpenShift cluster** 

The DPU Operator manages DPU-specific configurations and the service lifecycle
on the IPU, so you don’t have to handle them manually.

The most common way is via OperatorHub in the OpenShift web console, but
you can also install it via the CLI or YAML manifests if you prefer a more
automated approach. See the
[OpenShift DPU Operator documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html/networking_operators/dpu-operator#installing-dpu-operator).

Then follow the steps to [Configure the DPU Operator](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html/networking_operators/dpu-operator#configuring-dpu-operator).

> **Note**
> At the time of writing, the OperatorHub version may have issues. Be prepared to use workarounds or install from manifest if needed.

### **Install DPU Operator in Microshift on the IPU**

On Microshift, the process is similar but without a console Web UI.
You'll use the Operator Lifecycle Manager (OLM). Ensure the `microshift-olm`
package is installed, then follow the
[OLM Documentation](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/4.19/html/running_applications/operators#microshift-operators-olm).

You will need to create a `CatalogSource`. Inspect what is configured on the
OCP cluster:
```bash
oc get -n openshift-marketplace catalogsource/redhat-operators -o yaml
```
and create a corresponding `CatalogSource` in Microshift on the IPU.

The further steps are the same as installing the DPU operator on the Openshift cluster.

###  **Deploy F5 NGINX on the IPU**

With MicroShift running on the IPU, OpenShift running on the host,  and the DPU Operator installed on both, you can now deploy NGINX to the IPU. This is done by creating a `ServiceFunctionChain` custom resource on the MicroShift instance running on the IPU.

1.  Create the `ServiceFunctionChain` manifest by creating a YAML file, for example named `nginx-sfc.yaml` with the following content. 

```
apiVersion: config.openshift.io/v1
kind: ServiceFunctionChain
metadata:
  name: sfc-test
  namespace: openshift-dpu-operator
spec:
  networkFunctions:
  - name: nginx
    image: nginx
    imagePullPolicy: IfNotPresent
```
2. Apply this manifest to the MicroShift cluster on the IPU by running the following command: 

> **Note**
>  Make sure the correct `kubeconfig` is loaded before running the following command to apply the manifest.

    oc apply -f nginx-sfc.yaml

This command instructs MicroShift (using the components deployed by the DPU Operator's agent) to pull the NGINX image and run it as a service on the IPU hooked up to the blue network as shown in the diagram at the beginning of this knowledge base document.

### **Deploy workload virtual machines or pods on the Host**

Deploy the ResNet (or other workload) pods on the OpenShift worker nodes. These pods will be accessed via the NGINX reverse proxy running on the IPU.
 
1.  Create the following manifest named for example `your-manifest.yaml `.

```
apiVersion: v1
kind: Pod
metadata:
  name: resnet50-model-server-1
  namespace: default
  annotations:
    k8s.v1.cni.cncf.io/networks: default-sriov-net
  labels:
    app: resnet50-model-server-service
spec:
  securityContext:
    runAsUser: 0
  nodeSelector:
    kubernetes.io/hostname: worker-hostname
  volumes:
    - name: model-volume
      emptyDir: {}
  initContainers:
    - name: model-downloader
      image: ubuntu:latest
      securityContext:
        runAsUser: 0
      command:
        - bash
        - -c
        - |
          apt-get update && \
          apt-get install -y wget ca-certificates && \
          mkdir -p /models/1 && \
          wget --no-check-certificate https://storage.openvinotoolkit.org/repositories/open_model_zoo/2022.1/models_bin/2/resnet50-binary-0001/FP32-INT1/resnet50-binary-0001.xml -O /models/1/model.xml && \
          wget --no-check-certificate https://storage.openvinotoolkit.org/repositories/open_model_zoo/2022.1/models_bin/2/resnet50-binary-0001/FP32-INT1/resnet50-binary-0001.bin -O /models/1/model.bin
      volumeMounts:
        - name: model-volume
          mountPath: /models
  containers:
    - name: ovms
      image: openvino/model_server:latest
      args:
        - "--model_path=/models"
        - "--model_name=resnet50"
        - "--port=9000"
        - "--rest_port=8000"
      ports:
        - containerPort: 8000
        - containerPort: 9000
      volumeMounts:
        - name: model-volume
          mountPath: /models
      securityContext:
          privileged: true
      resources:
        requests:
          openshift.io/dpu: '1'
        limits:
          openshift.io/dpu: '1'
```

Adjust the `kubernetes.io/hostname` selector for your cluster.

The `k8s.v1.cni.cncf.io/networks: default-sriov-net` annotation configures a secondary network using the network attachment definition created by the DPU operator.

2. Apply the manifest by running the following command:

```
oc apply -f your-manifest.yaml -n <namespace>
```

3. Repeat the steps above for each ResNet pod you need to deploy.

4. Note their IP addresses on the Blue Network once they are running. For example run `oc -n default exec -ti pod/resnet50-model-server-1 -- hostname -I`. or via
   `oc -n default get pod "resnet50-model-server-1" -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' | jq -r '.[] | select(.interface=="net1") | .ips[0]'`

### **Configure NGINX as a reverse proxy**

The NGINX instance running on the IPU (deployed using `ServiceFunctionChain`) needs to be configured to act as a reverse proxy, forwarding requests to the ResNet pods on the Blue Network.

For production use the NGINX service needs to be configured automatically. For example, by building a specific NGINX container image with the base configuration. There
also needs to be a way to automatically configure the IP addresses of the upstream pods. In the future, DPU Operator's `ServiceFunctionChain` may support features
to help with that like a `ConfigMap` for the network function pod.

In our example, we take the upstream `nginx` container from the Docker Container Registry. We thus need to configure the pod after it started. We
can do sy by accessing the NGINX pod with `oc -n openshift-dpu-operator exec -ti pod/nginx -- bash`.

This configuration typically involves:

1. Obtaining IPs of pods: Identify the IP addresses assigned to your ResNet pods on the Blue Network. 

2. NGINX Configuration (`nginx.conf`):  A typical `nginx.conf` for this purpose would include: 

    * An `upstream` block defining the backend ResNet pods.
    * A `server` block listening on a specific port on the IPU's Blue Network interface.
    * `location` blocks with `proxy_pass` directives pointing to the upstream.

```
http {
    server {
        listen      *:443 ssl http2;

        server_name demo.example.com;
        ssl_certificate /etc/nginx/server.crt;
        ssl_certificate_key /etc/nginx/server.key;

        # proxy gRPC → your upstream
        location / {
            # these must match your gRPC host header
            grpc_set_header   Host              $http_host;
            grpc_set_header   X-Real-IP         $remote_addr;
            grpc_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
            grpc_pass         grpc://model_servers;
        }

        location /nginx_status {
            stub_status on;
        }
    }

    upstream model_servers {
        server 10.56.217.2:9000;
        server 10.56.217.3:9000;
        server 10.56.217.4:9000;
    }
}

events {
    worker_connections 1024;
}
```

3. Configure TLS certificate by providing the files `/etc/nginx/server.{crt,key}` to the pod.

4. Configure IP addresses for the external endpoint and to reach the ResNet pods. The former
   is the IP address which client applications will access. The latter is in the `10.56.217.0/24`
   subnet to communicate with the pods.

5. Reload nginx inside the pod `nginx -s reload`.

### **Accessing the Service and Performing Inference**

Once NGINX is deployed on the IPU and configured to proxy requests to the ResNet VMs:

1. **Identify NGINX Access Point:** Determine the IP address and port on which NGINX is listening on the IPU's Blue Network interface. This is the entry point for your client traffic that we configured in the pervious step.
2. **Client Access:**
    * Clients (for example test scripts, applications) that need to perform inference send their requests to `https://` on the NGINX entry point.
    * These clients must have network reachability to the IPU's NGINX IP on the Blue Network. This might involve:
        * Clients running as pods within the OpenShift cluster, also attached to the Blue Network.
        * Clients external to the cluster, with appropriate routing configured to reach the Blue Network.
3. **Verification:**
    * Send a test request.
    * Verify that the request is routed through NGINX on the IPU to one of the ResNet pods, and you receive the expected response.
    * Monitor NGINX logs on the IPU and application logs on the pods for troubleshooting.

See also the [OPI Lab Demo](https://github.com/opiproject/opi-poc/tree/main/demos/Secure-AI-inferencing-NGINX-IPU/demo) which implements a similar setup.

### **Conclusion**

By following this knowledge base, you have successfully

1. Deployed the DPU Operator on OpenShift.

2. Offloaded an F5 NGINX reverse proxy to the Intel IPU.

3. Exposed ResNet pods running on host worker nodes.

This architecture leverages the IPU's capabilities to free up host CPU resources, potentially improve network performance, and enhance security by isolating network functions.

This setup provides a robust, Kubernetes-native approach to managing and utilizing DPUs within an OpenShift environment, paving the way for more complex service chaining and infrastructure offloading.

### **Further Information**

* Intel IPU Documentation (for firmware and Redfish)
* [Red Hat OpenShift Container Platform Documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest)
* [Red Hat MicroShift Documentation](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/latest)
* [OpenShift DPU Operator Documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/networking/networking-operators#dpu-operator)
* F5 NGINX Documentation (for NGINX configuration details)
