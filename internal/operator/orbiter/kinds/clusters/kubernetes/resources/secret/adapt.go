package secret

import (
	"github.com/caos/orbos/internal/operator/orbiter/kinds/clusters/kubernetes"
	"github.com/caos/orbos/internal/operator/orbiter/kinds/clusters/kubernetes/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func AdaptFunc(name string, namespace string, labels map[string]string, data map[string]string) (resources.QueryFunc, resources.DestroyFunc, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
	return func(_ *kubernetes.Client) (resources.EnsureFunc, error) {
			return func(k8sClient *kubernetes.Client) error {
				return k8sClient.ApplySecret(secret)
			}, nil
		}, func(k8sClient *kubernetes.Client) error {
			return k8sClient.DeleteSecret(name, namespace)
		}, nil
}
