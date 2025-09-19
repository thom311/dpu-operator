## **Introduction**

This knowledge base details the steps required to deploy a full end-to-end solution with Intel's Infrastructure Processing Unit (Intel®
IPU E2100 Series) integrated into Red Hat OpenShift, the industry-leading hybrid cloud application platform. We will demonstrate how to leverage the OpenShift DPU Operator to offload network functions. Specifically, this involves using F5 NGINX as a reverse proxy for confidential AI workloads running on the host, thereby enhancing application performance and security. Note that most of this knowledge base can be re-used for other workloads as well.

## **Key components and concepts:**

-   **Intel IPU E2100 Series:** An advanced programmable network device featuring an IPU Management Console (IMC) and an Arm-based Compute Complex (ACC). Its Flexible Programmable Packet Processing Engine (FPPE) allows for service function chaining, offloading tasks like firewalls, packet filtering, and compression. This frees host CPU resources and introduces a security boundary.

-   **Arm Compute Complex (ACC):** Runs MicroShift on Red Hat Enterprise Linux (RHEL), enabling management with standard Red Hat tools, treating the IPU like any other server.

-   **OpenShift DPU Operator:** Deploys a daemonset on OpenShift worker nodes. These daemons interface with a daemon on the IPU via a vendor agnostic API that is part of the Open Programmable Infrastructure (OPI) project, managing the lifecycle of IPU workloads through a vendor-agnostic, Kubernetes-native workflow.

-   **Solution Overview:** This knowledge base walks through deploying the F5 NGINX on the IPU. This NGINX instance functions as a reverse proxy, providing access to ResNet application virtual machines (VMs) running on the OpenShift worker nodes.

**Prerequisites**

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

**Solution architecture and network topology**
[image=[src="images/ipu-dp-ocp-arch_2.png", alt="DPU OpenShift architecture", size="LG - Large", data-cp-size="100%",  ]]

The deployment involves three networks:

-   **Red Network (OpenShift Network):** The standard OpenShift cluster network for control plane and primary application traffic. This network connects the OpenShift controllers and worker nodes.

-   **Blue Network (Secondary/Data Plane Network):** A secondary network enabling communication between workloads on the IPU (for example NGINX) and pods/VMs on the host that are attached to this secondary network.

-   **Green Network (Provisioning Network):** A separate network meant to access the management complex of systems

## **Step-by-Step deployment** 

### **Preparing the Intel IPU (Installing RHEL and MicroShift)** 
This section outlines building a RHEL ISO with MicroShift and deploying it to the IPU. 

####1. **Create RHEL for edge image with kickstart:**

Follow the guidance in [Creating the RHEL for Edge image](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/4.15/html/installing/microshift-embed-in-rpm-ostree#microshift-creating-ostree-iso_microshift-embed-in-rpm-ostree) to create a kickstart file.

> **Important kickstart configuration**
>  Include the following in your kickstart file to enable iSCSI boot for the IPU's Arm Compute Complex (ACC). The 192.168.0.0/24 network is internal to the IPU and should not be used elsewhere.

```
bootloader --location=mbr --driveorder=sda --append="ip=192.168.0.2:::255.255.255.0::enp0s1f0:off netroot=iscsi:192.168.0.1::::iqn.e2000:acc"
```

####2. **Boot ISO on IPU via Redfish:**

Use Redfish virtual media to boot the newly created RHEL for Edge ISO on the IPU. Consult the official Intel IPU documentation for specific Redfish procedures.

####3. **Install and Configure MicroShift:**

Once RHEL is installed on the IPU, follow the guidance in the [Red Hat build of MicroShift documentation](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/latest/html/installing_with_an_rpm_package/index) to install MicroShift and any additional required packages.

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

Run the following command enable `systemd` daemon and create `hugepages` configuration on the ACC:

```
cat << EOF > /etc/systemd/system/hugepages-setup.service
Description=Setup Hugepages
Before=microshift.service
Wants=microshift.service
   
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/mkdir -p /dev/hugepages
ExecStart=/bin/mount -t hugetlbfs -o pagesize=2M none /dev/hugepages
ExecStart=/bin/sh -c 'echo 512 > /sys/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages'

[Install]
WantedBy=multi-user.target"""
EOF
sudo systemctl daemon-reload
sudo systemctl enable hugepages-setup.service
sudo systemctl start hugepages-setup.service
systemctl restart microshift
```

####6. **Reload IDPF driver on Host:**

As a workaround for a known issue, after the IPU is fully set up with RHEL and MicroShift, reload the `idpf` driver on the OpenShift worker node hosting the IPU by running the following commands. If you don’t have an Operating System installed on the host, postpone running these commands until the host has been set up. 

```
sudo rmmod idpf 
sudo modprobe idpf
```

### **Install OpenShift Container Platform** 

Ensure you have a fully operational OpenShift cluster. For installation guidance, refer to the [OpenShift documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest#Install). The [Assisted Installer](https://access.redhat.com/documentation/en-us/assisted_installer_for_openshift_container_platform) method supports various deployment platforms with a focus on bare metal.

### **Install the DPU Operator on your OpenShift cluster** 

Install the DPU Operator on your OpenShift cluster. The DPU Operator manages the DPU-specific configurations and life cycle of services on the IPU. Follow the [OpenShift DPU Operator documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/networking/networking-operators#dpu-operator) for installation instructions. 

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
    kubernetes.io/hostname: worker-238
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
Key points in the manifest: 

* `.spec.template.spec.networks` and `.spec.template.spec.domain.devices.interfaces`define network attachments. The `blue-network` connects to the `default-sriov-net`.

2. Apply the manifest by running the following command:

```
oc apply -f your-manifest.yaml -n <namespace>
```

3. Repeat the steps above for each ResNet pod you need to deploy. Note their IP addresses on the Blue Network once they are running. 

### **Configure NGINX as a reverse proxy**
The NGINX instance running on the IPU (deployed using `ServiceFunctionChain`) needs to be configured to act as a reverse proxy, forwarding requests to the ResNet pods on the Blue Network.

This configuration typically involves:

1. Obtaining IPs of pods: Identify the IP addresses assigned to your ResNet pods on the Blue Network. 

2. NGINX Configuration (`nginx.conf`):  A typical `nginx.conf` for this purpose would include: 

    * An `upstream` block defining the backend ResNet pods.
    * A `server` block listening on a specific port on the IPU's Blue Network interface.
    * `location` blocks with `proxy_pass` directives pointing to the upstream.

```
 http {
    server {
        listen      172.16.3.200:443 ssl http2;
        server_name grpc.example.com;

        #ssl_certificate     /path/to/fullchain.pem;
        #ssl_certificate_key /path/to/privkey.pem;
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
        # you can also add `keepalive` here
    }
}

events {
    worker_connections 1024;
}
```

### **Applying the NGINX Configuration**
The method for applying this configuration depends on the NGINX image and the `ServiceFunctionChain` capabilities: 

* **Pre-configured Image:** The `<F5_NGINX_DPU_IMAGE_URL>` might already contain a default configuration or expect environment variables for backend IPs.
* **ConfigMap:** If the `ServiceFunctionChain` CRD on MicroShift supports mounting `ConfigMaps`, you would create a `ConfigMap` containing your `nginx.conf` and reference it in the `ServiceFunctionChain` manifest. This is a common pattern in Kubernetes.

Consult the documentation for your specific F5 NGINX DPU image and the `ServiceFunctionChain` implementation on the IPU for the correct method. 

### **Accessing the Service and Performing Inference**

Once NGINX is deployed on the IPU and configured to proxy requests to the ResNet VMs:

1. **Identify NGINX Access Point:** Determine the IP address and port on which NGINX is listening on the IPU's Blue Network interface. This is the entry point for your client traffic.
2. **Client Access:**
    * Clients (for example test scripts, applications) that need to perform inference send their requests to `http://<NGINX_IPU_BLUE_NETWORK_IP>:<PORT>`.
    * These clients must have network reachability to the IPU's NGINX IP on the Blue Network. This might involve:
        * Clients running as pods within the OpenShift cluster, also attached to the Blue Network.
        * Clients external to the cluster, with appropriate routing configured to reach the Blue Network.
3. **Verification:**
    * Send a test request (for example, `curl http://<NGINX_IPU_BLUE_NETWORK_IP>:<PORT>/your-resnet-endpoint`).
    * Verify that the request is routed through NGINX on the IPU to one of the ResNet pods, and you receive the expected response.
    * Monitor NGINX logs on the IPU and application logs on the pods for troubleshooting.

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