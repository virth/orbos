package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/caos/orbos/internal/git"

	"github.com/caos/orbos/internal/executables"

	mach "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"

	"github.com/caos/orbos/internal/operator/orbiter/kinds/clusters/core/infra"
	"github.com/caos/orbos/mntr"
)

type CloudIntegration int

func join(
	monitor mntr.Monitor,
	clusterID string,
	joining *initializedMachine,
	joinAt infra.Machine,
	desired DesiredV0,
	kubeAPI *infra.Address,
	joinToken string,
	kubernetesVersion KubernetesVersion,
	certKey string,
	client *Client,
	imageRepository string,
	gitClient *git.Client) (*string, error) {

	monitor = monitor.WithFields(map[string]interface{}{
		"machine": joining.infra.ID(),
		"tier":    joining.pool.tier,
	})

	var applyNetworkCommand string
	prepareKubeadmInit := func() error { return nil }
	switch desired.Spec.Networking.Network {
	case "cilium":
		ciliumPath := "/var/orbiter/cilium.yaml"
		prepareKubeadmInit = func() error {
			buf := new(bytes.Buffer)
			defer buf.Reset()

			istioReg := desired.Spec.CustomImageRegistry
			if istioReg != "" {
				istioReg += "/"
			}

			ciliumReg := desired.Spec.CustomImageRegistry
			if ciliumReg == "" {
				ciliumReg = "docker.io"
			}

			template.Must(template.New("").Parse(string(executables.PreBuilt("cilium.yaml")))).Execute(buf, struct {
				IstioProxyImageRegistry string
				CiliumImageRegistry     string
			}{
				IstioProxyImageRegistry: istioReg,
				CiliumImageRegistry:     ciliumReg,
			})

			return joining.infra.WriteFile(ciliumPath, bytes.NewReader(executables.PreBuilt("cilium.yaml")), 600)
		}
		applyNetworkCommand = fmt.Sprintf("kubectl create -f %s", ciliumPath)
	case "calico":
		calicoPath := "/var/orbiter/calico.yaml"
		prepareKubeadmInit = func() error {

			reg := desired.Spec.CustomImageRegistry
			if reg != "" {
				reg += "/"
			}

			buf := new(bytes.Buffer)
			defer buf.Reset()
			template.Must(template.New("").Parse(string(executables.PreBuilt("calico.yaml")))).Execute(buf, struct {
				ImageRegistry string
			}{
				ImageRegistry: reg,
			})

			return joining.infra.WriteFile(calicoPath, buf, 600)
		}
		applyNetworkCommand = fmt.Sprintf(`kubectl create -f %s`, calicoPath)
	case "":
		applyNetworkCommand = "true"
	default:
		networkFile := gitClient.Read(desired.Spec.Networking.Network)
		if len(networkFile) == 0 {
			return nil, fmt.Errorf("network file %s is empty or not found in git repository", desired.Spec.Networking.Network)
		}

		remotePath := filepath.Join("/var/orbiter/", filepath.Base(desired.Spec.Networking.Network))
		prepareKubeadmInit = func(closedNetworkFile []byte, closedFilePath string) func() error {
			return func() error {
				return joining.infra.WriteFile(closedFilePath, bytes.NewReader(closedNetworkFile), 600)
			}
		}(networkFile, remotePath)
		applyNetworkCommand = fmt.Sprintf(`kubectl create -f %s`, remotePath)
	}

	kubeadmCfgPath := "/etc/kubeadm/config.yaml"
	kubeadmCfg := fmt.Sprintf(`apiVersion: kubeadm.k8s.io/v1beta2
kind: InitConfiguration
bootstrapTokens:
- groups:
  - system:bootstrappers:kubeadm:default-node-token
  token: %s
  ttl: 10m0s
  usages:
  - signing
  - authentication
localAPIEndpoint:
  advertiseAddress: %s
  bindPort: %d
nodeRegistration:
#	criSocket: /var/run/dockershim.sock
  name: %s
  kubeletExtraArgs:
    node-ip: %s
  taints:
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
---
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
apiServer:
  timeoutForControlPlane: 4m0s
  certSANs:
  - "%s"
certificatesDir: /etc/kubernetes/pki
clusterName: %s
controlPlaneEndpoint: %s
controllerManager: {}
dns:
  type: CoreDNS
etcd:
  local:
    imageRepository: "%s"
    dataDir: /var/lib/etcd
    extraArgs:
      listen-metrics-urls: http://0.0.0.0:2381
imageRepository: %s
kubernetesVersion: %s
networking:
  dnsDomain: %s
  podSubnet: %s
  serviceSubnet: %s
scheduler: {}
`,
		joinToken,
		joining.infra.IP(),
		kubeAPI.BackendPort,
		joining.infra.ID(),
		joining.infra.IP(),
		kubeAPI.Location,
		clusterID,
		kubeAPI,
		imageRepository,
		imageRepository,
		kubernetesVersion,
		desired.Spec.Networking.DNSDomain,
		desired.Spec.Networking.PodCidr,
		desired.Spec.Networking.ServiceCidr)

	if joinAt != nil {
		kubeadmCfg += fmt.Sprintf(`---
apiVersion: kubeadm.k8s.io/v1beta2
kind: JoinConfiguration
caCertPath: /etc/kubernetes/pki/ca.crt
discovery:
  bootstrapToken:
    apiServerEndpoint: %s:%d
    token: %s
    unsafeSkipCAVerification: true
  timeout: 5m0s
nodeRegistration:
  kubeletExtraArgs:
    node-ip: %s
  name: %s
`,
			joinAt.IP(),
			kubeAPI.BackendPort,
			joinToken,
			joining.infra.IP(),
			joining.infra.ID())

		if joining.pool.tier == Controlplane {
			kubeadmCfg += fmt.Sprintf(`controlPlane:
  localAPIEndpoint:
    advertiseAddress: %s
    bindPort: %d
  certificateKey: %s
`,
				joining.infra.IP(),
				kubeAPI.BackendPort,
				certKey)
		}
	}

	if err := infra.Try(monitor, time.NewTimer(7*time.Second), 2*time.Second, joining.infra, func(cmp infra.Machine) error {
		return cmp.WriteFile(kubeadmCfgPath, strings.NewReader(kubeadmCfg), 600)
	}); err != nil {
		return nil, err
	}
	monitor.WithFields(map[string]interface{}{
		"path": kubeadmCfgPath,
	}).Debug("Written file")

	cmd := "sudo kubeadm reset -f && sudo rm -rf /var/lib/etcd"
	resetStdout, err := joining.infra.Execute(nil, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "executing %s failed", cmd)
	}
	monitor.WithFields(map[string]interface{}{
		"stdout": string(resetStdout),
	}).Debug("Cleaned up machine")

	if joinAt != nil {
		cmd := fmt.Sprintf("sudo kubeadm join --ignore-preflight-errors=Port-%d %s:%d --config %s", kubeAPI.BackendPort, joinAt.IP(), kubeAPI.FrontendPort, kubeadmCfgPath)
		joinStdout, err := joining.infra.Execute(nil, cmd)
		if err != nil {
			return nil, errors.Wrapf(err, "executing %s failed", cmd)
		}

		monitor.WithFields(map[string]interface{}{
			"stdout": string(joinStdout),
		}).Debug("Executed kubeadm join")

		if err := joining.pool.infra.EnsureMember(joining.infra); err != nil {
			return nil, err
		}

		joining.currentMachine.Joined = true
		monitor.Changed("Node joined")

		dnsPods, err := client.set.CoreV1().Pods("kube-system").List(context.Background(), mach.ListOptions{LabelSelector: "k8s-app=kube-dns"})
		if err != nil {
			return nil, err
		}

		if len(dnsPods.Items) > 1 {
			err = client.set.CoreV1().Pods("kube-system").Delete(context.Background(), dnsPods.Items[0].Name, mach.DeleteOptions{})
		}

		return nil, err
	}

	if err := joining.pool.infra.EnsureMember(joining.infra); err != nil {
		return nil, err
	}

	if err := prepareKubeadmInit(); err != nil {
		return nil, err
	}

	initCmd := fmt.Sprintf("sudo kubeadm init --ignore-preflight-errors=Port-%d --config %s && mkdir -p ${HOME}/.kube && yes | sudo cp -rf /etc/kubernetes/admin.conf ${HOME}/.kube/config && sudo chown $(id -u):$(id -g) ${HOME}/.kube/config && %s", kubeAPI.BackendPort, kubeadmCfgPath, applyNetworkCommand)
	initStdout, err := joining.infra.Execute(nil, initCmd)
	if err != nil {
		return nil, err
	}
	monitor.WithFields(map[string]interface{}{
		"stdout": string(initStdout),
	}).Debug("Executed kubeadm init")

	kubeconfigBuf := new(bytes.Buffer)
	if err := joining.infra.ReadFile("${HOME}/.kube/config", kubeconfigBuf); err != nil {
		return nil, err
	}

	joining.currentMachine.Joined = true
	monitor.Changed("Cluster initialized")

	kc := strings.ReplaceAll(kubeconfigBuf.String(), "kubernetes-admin", strings.Join([]string{clusterID, "admin"}, "-"))

	return &kc, nil
}
