package kubernetes

import (
	"github.com/caos/orbos/internal/secret"
	"github.com/caos/orbos/internal/tree"
	"github.com/pkg/errors"

	"github.com/caos/orbos/mntr"
)

func SecretFunc() secret.Func {

	return func(monitor mntr.Monitor, desiredTree *tree.Tree) (secrets map[string]*secret.Secret, err error) {
		defer func() {
			err = errors.Wrapf(err, "building %s failed", desiredTree.Common.Kind)
		}()

		desiredKind, err := parseDesiredV0(desiredTree)
		if err != nil {
			return nil, errors.Wrap(err, "parsing desired state failed")
		}
		desiredTree.Parsed = desiredKind

		return getSecretsMap(desiredKind), nil
	}
}

func RewriteFunc(newMasterkey string) secret.Func {

	return func(monitor mntr.Monitor, desiredTree *tree.Tree) (secrets map[string]*secret.Secret, err error) {
		defer func() {
			err = errors.Wrapf(err, "building %s failed", desiredTree.Common.Kind)
		}()

		desiredKind, err := parseDesiredV0(desiredTree)
		if err != nil {
			return nil, errors.Wrap(err, "parsing desired state failed")
		}
		desiredTree.Parsed = desiredKind
		secret.Masterkey = newMasterkey

		return getSecretsMap(desiredKind), nil
	}
}

func getSecretsMap(desiredKind *DesiredV0) map[string]*secret.Secret {
	ret := make(map[string]*secret.Secret, 0)
	if desiredKind.Spec.Kubeconfig != nil {
		ret["kubeconfig"] = desiredKind.Spec.Kubeconfig
	}
	return ret
}
