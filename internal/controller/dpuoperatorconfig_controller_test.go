package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/dpu-operator/internal/scheme"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openshift/dpu-operator/internal/daemon/plugin"
	"github.com/openshift/dpu-operator/internal/testutils"
	"github.com/openshift/dpu-operator/pkgs/vars"

	configv1 "github.com/openshift/dpu-operator/api/v1"
)

var (
	testNamespace              = vars.Namespace
	testDpuOperatorConfigName  = vars.DpuOperatorConfigName
	testDpuOperatorConfigKind  = "DpuOperatorConfig"
	testDpuDaemonName          = "dpu-daemon"
	testNetworkFunctionNADDpu  = "dpunfcni-conf"
	testNetworkFunctionNADHost = "default-sriov-net"
	testClusterName            = "dpu-operator-test-cluster"
	setupLog                   = ctrl.Log.WithName("setup")
)

func dpuOperatorNameSpace() *corev1.Namespace {
	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
		Spec:   corev1.NamespaceSpec{},
		Status: corev1.NamespaceStatus{},
	}
	return namespace
}

func dpuOperatorCR(name string, mode string, ns *corev1.Namespace) *configv1.DpuOperatorConfig {
	config := &configv1.DpuOperatorConfig{}
	config.SetNamespace(ns.Name)
	config.SetName(name)
	config.Spec = configv1.DpuOperatorConfigSpec{
		Mode:     mode,
		LogLevel: 2,
	}
	return config
}

func createNameSpace(client client.Client, ns *v1.Namespace) {
	// ignore error when creating the namespace since it can already exist
	client.Create(context.Background(), ns)
	found := v1.Namespace{}
	Eventually(func() error {
		return client.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: ns.GetName()}, &found)
	}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())
}

func deleteNameSpace(client client.Client, ns *v1.Namespace) {
	client.Delete(context.Background(), ns)
	found := v1.Namespace{}
	Eventually(func() error {
		err := client.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: ns.GetName()}, &found)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())
}

func createDpuOperatorCR(client client.Client, cr *configv1.DpuOperatorConfig) {
	err := client.Create(context.Background(), cr)
	Expect(err).NotTo(HaveOccurred())
	found := configv1.DpuOperatorConfig{}
	Eventually(func() error {
		return client.Get(context.Background(), types.NamespacedName{Namespace: cr.GetNamespace(), Name: cr.GetName()}, &found)
	}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())
}

func deleteDpuOperatorCR(client client.Client, cr *configv1.DpuOperatorConfig) {
	client.Delete(context.Background(), cr)
	found := configv1.DpuOperatorConfig{}
	Eventually(func() error {
		err := client.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: cr.GetName()}, &found)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())
}

func startDPUControllerManager(ctx context.Context, client *rest.Config, wg *sync.WaitGroup) ctrl.Manager {
	var err error

	webhookInstallOptions := &testutils.TestEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(client, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: ":18001",
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElectionID: "1e46962d.openshift.io",
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&configv1.DpuOperatorConfig{}).SetupWebhookWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	b := NewDpuOperatorConfigReconciler(mgr.GetClient(), mgr.GetScheme(), "mock-image", plugin.CreateVspImagesMap(false, setupLog))
	err = b.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	wg.Add(1)
	go func() {
		setupLog.Info("starting manager")
		err := mgr.Start(ctx)
		setupLog.Info("starting manager done", "err", err)
		Expect(err).NotTo(HaveOccurred())
		wg.Done()
	}()
	<-mgr.Elected()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		setupLog.Info(">>>> dial test", "err", err)
		if err != nil {
			return err
		}
		return conn.Close()
	}).Should(Succeed())

	setupLog.Info(">>>> mgr: done")

	existing := &admissionv1.ValidatingWebhookConfiguration{}
	err = mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: "", Name: "validating-webhook-configuration"}, existing)
	setupLog.Info(">>>> mgr: doneXXX", "err", err, "existing", existing)

	return mgr
}

func stopDPUControllerManager(cancel context.CancelFunc, wg *sync.WaitGroup) {
	By("shut down controller manager")
	cancel()
	wg.Wait()
}

func waitAllNodesReady(client client.Client) {
	var nodes corev1.NodeList
	Eventually(func() error {
		return client.List(context.Background(), &nodes)
	}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())

	Eventually(func() bool {
		var latestNodes corev1.NodeList
		if err := client.List(context.Background(), &latestNodes); err != nil {
			return false
		}
		readyNodes := 0
		for _, node := range latestNodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					readyNodes++
					break
				}
			}
		}
		return readyNodes == len(latestNodes.Items)
	}, testutils.TestInitialSetupTimeout, testutils.TestRetryInterval).Should(BeTrue())
}

var _ = Describe("Main Controller", Ordered, func() {
	var cancel context.CancelFunc
	var ctx context.Context
	var wg sync.WaitGroup
	var mgr ctrl.Manager
	var testCluster testutils.KindCluster

	BeforeAll(func() {
		opts := zap.Options{
			Development: true,
		}
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
		testCluster = testutils.KindCluster{Name: testClusterName}
		client := testCluster.EnsureExists()
		ctx, cancel = context.WithCancel(context.Background())
		wg = sync.WaitGroup{}
		mgr = startDPUControllerManager(ctx, client, &wg)
		waitAllNodesReady(mgr.GetClient())

		found := configv1.DpuOperatorConfig{}
		Eventually(func() error {
			err := mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: "", Name: ""}, &found)
			if errors.IsNotFound(err) {
				return nil
			} else {
				return err
			}
		}, testutils.TestAPITimeout, testutils.TestRetryInterval).Should(Succeed())
	})
	AfterAll(func() {
		stopDPUControllerManager(cancel, &wg)
		if os.Getenv("FAST_TEST") == "false" {
			testCluster.EnsureDeleted()
		}
	})
	Context("When Host controller manager has started without DpuOperatorConfig CR", func() {
		var cr *configv1.DpuOperatorConfig

		Context("When DpuOperatorConfig CR exists with host mode", func() {
			BeforeAll(func() {
				ns := dpuOperatorNameSpace()
				cr = dpuOperatorCR(testDpuOperatorConfigName, "host", ns)
				createNameSpace(mgr.GetClient(), ns)

				dialer := &net.Dialer{Timeout: time.Second}
				webhookInstallOptions := &testutils.TestEnv.WebhookInstallOptions
				addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
				Eventually(func() error {
					conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
					setupLog.Info(">>>> dial test", "err", err)
					if err != nil {
						return err
					}
					return conn.Close()
				}).Should(Succeed())

				cmd := exec.Command("curl", "-v", "-k", "https://"+webhookInstallOptions.LocalServingHost+":"+strconv.Itoa(webhookInstallOptions.LocalServingPort)+"/validate-config-openshift-io-v1-dpuoperatorconfig?timeout=10s")
				output, err := cmd.CombinedOutput()
				if err != nil {
					setupLog.Error(err, "Command failed")
				}
				setupLog.Info("Command output:", "output", string(output))

				setupLog.Info(">>>> CERT:", "a", webhookInstallOptions.LocalServingCAData)

				err = os.WriteFile("cadata.txt", webhookInstallOptions.LocalServingCAData, 0644)
				if err != nil {
					setupLog.Error(err, "Failed to write file")
				}

				cmd = exec.Command("curl", "-v", "--cacert", "cadata.txt", "https://"+webhookInstallOptions.LocalServingHost+":"+strconv.Itoa(webhookInstallOptions.LocalServingPort)+"/validate-config-openshift-io-v1-dpuoperatorconfig?timeout=10s")
				output, err = cmd.CombinedOutput()
				if err != nil {
					setupLog.Error(err, "Command failed")
				}
				setupLog.Info("Command output:", "output", string(output))

				setupLog.Info(">>>> certdir: " + webhookInstallOptions.LocalServingCertDir)
				cmd = exec.Command("sh", "-c", "find "+webhookInstallOptions.LocalServingCertDir+"; ls -la "+webhookInstallOptions.LocalServingCertDir)
				output, err = cmd.CombinedOutput()
				if err != nil {
					setupLog.Error(err, "Command failed")
				}
				setupLog.Info("Command output:", "output", string(output))

				setupLog.Info(">>>> before createDpuOperatorCR()")

				existing := &admissionv1.ValidatingWebhookConfiguration{}
				err = mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: "", Name: "validating-webhook-configuration"}, existing)
				setupLog.Info(">>>> mgr: doneXXX", "err", err, "existing", existing)

				createDpuOperatorCR(mgr.GetClient(), cr)
			})
			It("should have DPU daemon daemonsets created by controller manager", func() {
				daemonSet := appsv1.DaemonSet{}
				testutils.WaitForDaemonSetReady(&daemonSet, mgr.GetClient(), testNamespace, testDpuDaemonName)
				Expect(daemonSet.Spec.Template.Spec.Containers[0].Args[1]).To(Equal("auto"))
			})
			It("should have the network function NAD created by controller manager", func() {
				nad := &netattdefv1.NetworkAttachmentDefinition{}
				Eventually(func() error {
					return mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: "default", Name: testNetworkFunctionNADHost}, nad)
				}, testutils.TestAPITimeout*3, testutils.TestRetryInterval).ShouldNot(HaveOccurred())
			})
			It("only DpuOperatorConfig \"openshift-dpu-operator/dpu-operator-config\" is allowed", func() {
				ns := dpuOperatorNameSpace()
				client := mgr.GetClient()

				cr2 := dpuOperatorCR("foo", "host", ns)
				err2 := client.Create(context.Background(), cr2)
				/* The validating webhook does not run in this setup. If it were, adding
				 * this CR would be rejected. Instead, it passes. This indicates that the
				 * webhook is not running. */
				Expect(err2).NotTo(HaveOccurred())
				deleteDpuOperatorCR(mgr.GetClient(), cr2)
			})
			AfterAll(func() {
				ns := dpuOperatorNameSpace()
				cr = dpuOperatorCR(testDpuOperatorConfigName, "host", ns)
				deleteDpuOperatorCR(mgr.GetClient(), cr)
			})
		})

		Context("When DpuOperatorConfig CR is created with dpu mode", func() {
			BeforeAll(func() {
				ns := dpuOperatorNameSpace()
				cr = dpuOperatorCR("operator-config", "dpu", ns)
				createNameSpace(mgr.GetClient(), ns)
				createDpuOperatorCR(mgr.GetClient(), cr)
			})
			It("should have DPU daemon daemonsets created by controller manager", func() {
				daemonSet := &appsv1.DaemonSet{}
				Eventually(func() string {
					testutils.WaitForDaemonSetReady(daemonSet, mgr.GetClient(), testNamespace, testDpuDaemonName)
					return daemonSet.Spec.Template.Spec.Containers[0].Args[1]
				}, testutils.TestAPITimeout*2, testutils.TestRetryInterval).Should(Equal("auto"))
			})
			It("should have the network function NAD created by controller manager", func() {
				nad := &netattdefv1.NetworkAttachmentDefinition{}
				Eventually(func() error {
					return mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: testNetworkFunctionNADDpu}, nad)
				}, testutils.TestAPITimeout*3, testutils.TestRetryInterval).ShouldNot(HaveOccurred())
			})
			AfterAll(func() {
				ns := dpuOperatorNameSpace()
				cr = dpuOperatorCR("operator-config", "host", ns)
				deleteDpuOperatorCR(mgr.GetClient(), cr)
			})
		})
	})
})
