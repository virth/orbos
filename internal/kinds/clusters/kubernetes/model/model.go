package model

import (
	"github.com/caos/orbiter/internal/core/operator/orbiter"
"github.com/caos/orbiter/internal/core/operator/common"
	"github.com/caos/orbiter/internal/kinds/clusters/core/infra"
	"github.com/caos/orbiter/logging"
)

type UserSpec struct {
	Destroyed  bool
	Verbose    bool
	Kubernetes string
	Networking struct {
		DNSDomain   string
		Network     string
		ServiceCidr string
		PodCidr     string
	}
	ControlPlane *Pool
	Workers      map[string]*Pool
}

type Parameters struct {
	Logger             logging.Logger
	ID                 string
	RepoURL            string
	RepoKey            string
	MasterKey          string
	OrbiterCommit      string
	CurrentFile        string
	SecretsFile        string
	SelfAbsolutePath   []string
	ConnectFromOutside bool
}

type Config struct {
	Spec      UserSpec
	Params    Parameters
	NodeAgent func(cmp infra.Compute) *common.NodeAgentCurrent
}

type Pool struct {
	UpdatesDisabled bool
	Provider        string
	Nodes           int
	Pool            string
}

var CurrentVersion = "v0"

type Current struct {
	Status   string
	Computes map[string]*Compute `yaml:"computes"`
}

type Compute struct {
	Status    string
	Metadata  *ComputeMetadata
	Nodeagent *common.NodeAgentCurrent `yaml:"-"`
}

type ComputeMetadata struct {
	Tier     Tier
	Provider string
	Pool     string
	Group    string `yaml:",omitempty"`
}

type Tier string

const (
	Controlplane Tier = "controlplane"
	Workers      Tier = "workers"
)
