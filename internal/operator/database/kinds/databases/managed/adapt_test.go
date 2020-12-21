package managed

import (
	coremock "github.com/caos/orbos/internal/operator/database/kinds/databases/core/mock"
	"github.com/caos/orbos/mntr"
	kubernetesmock "github.com/caos/orbos/pkg/kubernetes/mock"
	"github.com/caos/orbos/pkg/labels"
	"github.com/caos/orbos/pkg/secret"
	"github.com/caos/orbos/pkg/tree"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
	"time"
)

func getDesiredTree(t *testing.T, masterkey string, desired interface{}) *tree.Tree {
	secret.Masterkey = masterkey

	desiredTree := &tree.Tree{}
	data, err := yaml.Marshal(desired)
	assert.NoError(t, err)
	assert.NoError(t, yaml.Unmarshal(data, desiredTree))

	return desiredTree
}

func TestManaged_Adapt1(t *testing.T) {
	monitor := mntr.Monitor{}

	nodeLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp",
		"app.kubernetes.io/name":       "cockroachdb.node",
		"app.kubernetes.io/part-of":    "testProd",
		"orbos.ch/selectable":          "yes",
	}

	cockroachLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp",
		"app.kubernetes.io/name":       "cockroachdb-budget",
		"app.kubernetes.io/part-of":    "testProd",
		"app.kubernetes.io/version":    "testVersion",
		"caos.ch/apiversion":           "v0",
		"caos.ch/kind":                 "testKind",
	}

	cockroachSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp",
		"app.kubernetes.io/name":       "cockroachdb",
		"app.kubernetes.io/part-of":    "testProd",
		"orbos.ch/selectable":          "yes",
	}

	operatorLabels := labels.MustForOperator("testProd", "testOp", "testVersion")
	apiLabels := labels.MustForAPI(operatorLabels, "testKind", "v0")

	namespace := "testNs"
	timestamp := "testTs"
	nodeselector := map[string]string{"test": "test"}
	tolerations := []corev1.Toleration{}
	version := "testVersion"
	features := []string{"database"}
	masterkey := "testMk"
	k8sClient := kubernetesmock.NewMockClientInt(gomock.NewController(t))
	dbCurrent := coremock.NewMockDatabaseCurrent(gomock.NewController(t))
	queried := map[string]interface{}{}

	desired := getDesiredTree(t, masterkey, &DesiredV0{
		Common: &tree.Common{
			Kind:    "databases.caos.ch/CockroachDB",
			Version: "v0",
		},
		Spec: Spec{
			Verbose:         false,
			ReplicaCount:    1,
			StorageCapacity: "368Gi",
			StorageClass:    "testSC",
			NodeSelector:    map[string]string{},
			ClusterDns:      "testDns",
		},
	})

	unav := intstr.FromInt(1)
	k8sClient.EXPECT().ApplyPodDisruptionBudget(&policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cockroachdb-budget",
			Namespace: namespace,
			Labels:    cockroachLabels,
		},
		Spec: policy.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: cockroachSelectorLabels,
			},
			MaxUnavailable: &unav,
		},
	})
	secretList := &corev1.SecretList{
		Items: []corev1.Secret{},
	}

	k8sClient.EXPECT().ApplyService(gomock.Any()).Times(3)
	k8sClient.EXPECT().ApplyServiceAccount(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyRole(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyClusterRole(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyRoleBinding(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyClusterRoleBinding(gomock.Any()).Times(1)
	//statefulset
	k8sClient.EXPECT().ApplyStatefulSet(gomock.Any(), gomock.Any()).Times(1)
	//running for setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, false, time.Duration(60))
	//not ready for setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, true, time.Duration(1))
	//ready after setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, true, time.Duration(60))
	//client
	k8sClient.EXPECT().ListSecrets(namespace, nodeLabels).Times(1).Return(secretList, nil)
	dbCurrent.EXPECT().GetCertificate().Times(1).Return(nil)
	dbCurrent.EXPECT().GetCertificateKey().Times(1).Return(nil)
	k8sClient.EXPECT().ApplySecret(gomock.Any()).Times(1)
	//node
	k8sClient.EXPECT().ListSecrets(namespace, nodeLabels).Times(1).Return(secretList, nil)
	dbCurrent.EXPECT().GetCertificate().Times(1).Return(nil)
	dbCurrent.EXPECT().GetCertificateKey().Times(1).Return(nil)
	dbCurrent.EXPECT().SetCertificate(gomock.Any()).Times(1)
	dbCurrent.EXPECT().SetCertificateKey(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplySecret(gomock.Any()).Times(1)

	query, _, _, err := AdaptFunc(operatorLabels, apiLabels, namespace, timestamp, nodeselector, tolerations, version, features)(monitor, desired, &tree.Tree{})
	assert.NoError(t, err)

	ensure, err := query(k8sClient, queried)
	assert.NoError(t, err)
	assert.NotNil(t, ensure)

	assert.NoError(t, ensure(k8sClient))
}

func TestManaged_Adapt2(t *testing.T) {
	monitor := mntr.Monitor{}
	namespace := "testNs"
	timestamp := "testTs"

	nodeLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp2",
		"app.kubernetes.io/name":       "cockroachdb.node",
		"app.kubernetes.io/part-of":    "testProd2",
		"orbos.ch/selectable":          "yes",
	}

	cockroachLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp2",
		"app.kubernetes.io/name":       "cockroachdb-budget",
		"app.kubernetes.io/part-of":    "testProd2",
		"app.kubernetes.io/version":    "testVersion2",
		"caos.ch/apiversion":           "v1",
		"caos.ch/kind":                 "testKind2",
	}

	cockroachSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "cockroachdb",
		"app.kubernetes.io/managed-by": "testOp2",
		"app.kubernetes.io/name":       "cockroachdb",
		"app.kubernetes.io/part-of":    "testProd2",
		"orbos.ch/selectable":          "yes",
	}

	operatorLabels := labels.MustForOperator("testProd2", "testOp2", "testVersion2")
	apiLabels := labels.MustForAPI(operatorLabels, "testKind2", "v1")

	nodeselector := map[string]string{"test2": "test2"}
	tolerations := []corev1.Toleration{}
	version := "testVersion2"
	features := []string{"database"}
	masterkey := "testMk2"
	k8sClient := kubernetesmock.NewMockClientInt(gomock.NewController(t))
	dbCurrent := coremock.NewMockDatabaseCurrent(gomock.NewController(t))
	queried := map[string]interface{}{}

	desired := getDesiredTree(t, masterkey, &DesiredV0{
		Common: &tree.Common{
			Kind:    "databases.caos.ch/CockroachDB",
			Version: "v0",
		},
		Spec: Spec{
			Verbose:         false,
			ReplicaCount:    1,
			StorageCapacity: "368Gi",
			StorageClass:    "testSC",
			NodeSelector:    map[string]string{},
			ClusterDns:      "testDns",
		},
	})

	unav := intstr.FromInt(1)
	k8sClient.EXPECT().ApplyPodDisruptionBudget(&policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cockroachdb-budget",
			Namespace: namespace,
			Labels:    cockroachLabels,
		},
		Spec: policy.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: cockroachSelectorLabels,
			},
			MaxUnavailable: &unav,
		},
	})
	secretList := &corev1.SecretList{
		Items: []corev1.Secret{},
	}

	k8sClient.EXPECT().ApplyService(gomock.Any()).Times(3)
	k8sClient.EXPECT().ApplyServiceAccount(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyRole(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyClusterRole(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyRoleBinding(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplyClusterRoleBinding(gomock.Any()).Times(1)
	//statefulset
	k8sClient.EXPECT().ApplyStatefulSet(gomock.Any(), gomock.Any()).Times(1)
	//running for setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, false, time.Duration(60))
	//not ready for setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, true, time.Duration(1))
	//ready after setup
	k8sClient.EXPECT().WaitUntilStatefulsetIsReady(namespace, sfsName, true, true, time.Duration(60))
	//client
	k8sClient.EXPECT().ListSecrets(namespace, nodeLabels).Times(1).Return(secretList, nil)
	dbCurrent.EXPECT().GetCertificate().Times(1).Return(nil)
	dbCurrent.EXPECT().GetCertificateKey().Times(1).Return(nil)
	k8sClient.EXPECT().ApplySecret(gomock.Any()).Times(1)
	//node
	k8sClient.EXPECT().ListSecrets(namespace, nodeLabels).Times(1).Return(secretList, nil)
	dbCurrent.EXPECT().GetCertificate().Times(1).Return(nil)
	dbCurrent.EXPECT().GetCertificateKey().Times(1).Return(nil)
	dbCurrent.EXPECT().SetCertificate(gomock.Any()).Times(1)
	dbCurrent.EXPECT().SetCertificateKey(gomock.Any()).Times(1)
	k8sClient.EXPECT().ApplySecret(gomock.Any()).Times(1)

	query, _, _, err := AdaptFunc(operatorLabels, apiLabels, namespace, timestamp, nodeselector, tolerations, version, features)(monitor, desired, &tree.Tree{})
	assert.NoError(t, err)

	ensure, err := query(k8sClient, queried)
	assert.NoError(t, err)
	assert.NotNil(t, ensure)

	assert.NoError(t, ensure(k8sClient))
}