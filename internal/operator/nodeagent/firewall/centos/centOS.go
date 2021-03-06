package centos

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/caos/orbos/internal/operator/common"
	"github.com/caos/orbos/internal/operator/nodeagent"
	"github.com/caos/orbos/mntr"
)

func Ensurer(monitor mntr.Monitor, ignore []string) nodeagent.FirewallEnsurer {
	return nodeagent.FirewallEnsurerFunc(func(desired common.Firewall) (common.Current, func() error, error) {
		ensurers := make([]func() error, 0)
		current := make(common.Current, 0)

		if desired.Zones == nil {
			desired.Zones = make(map[string]*common.Zone, 0)
		}

		for name, _ := range desired.Zones {
			currentZone, ensureFunc, err := ensureZone(monitor, name, desired, ignore)
			if err != nil {
				return current, nil, err
			}
			current = append(current, currentZone)
			if ensureFunc != nil {
				ensurers = append(ensurers, ensureFunc)
			}
		}

		_, inactiveErr := runCommand(monitor, "systemctl", "is-active", "firewalld")
		if inactiveErr == nil && len(ensurers) == 0 {
			monitor.Debug("Not changing firewall")
			return current, nil, nil
		}

		current.Sort()

		return current, func() error {
			monitor.Debug("Ensuring firewall")
			for _, ensurer := range ensurers {
				if err := ensurer(); err != nil {
					return err
				}
			}
			return nil
		}, nil
	})
}

func ensureZone(monitor mntr.Monitor, zoneName string, desired common.Firewall, ignore []string) (*common.ZoneDesc, func() error, error) {
	current := &common.ZoneDesc{
		Name:       zoneName,
		Interfaces: []string{},
		Services:   []*common.Service{},
		FW:         []*common.Allowed{},
	}

	ifaces, err := getInterfaces(monitor, zoneName)
	if err != nil {
		return current, nil, err
	}
	current.Interfaces = ifaces

	sources, err := getSources(monitor, zoneName)
	if err != nil {
		return current, nil, err
	}
	current.Sources = sources

	addPorts, removePorts, err := getAddAndRemovePorts(monitor, zoneName, current, desired.Ports(zoneName), ignore)
	if err != nil {
		return current, nil, err
	}

	ensureIfaces, removeIfaces, err := getEnsureAndRemoveInterfaces(zoneName, current, desired)
	if err != nil {
		return current, nil, err
	}

	addSources, removeSources, err := getAddAndRemoveSources(zoneName, current, desired)
	if err != nil {
		return current, nil, err
	}

	ensureTarget, err := getEnsureTarget(monitor, zoneName)
	if err != nil {
		return current, nil, err
	}

	monitor.WithFields(map[string]interface{}{
		"open":  strings.Join(addPorts, ";"),
		"close": strings.Join(removePorts, ";"),
	}).Debug("firewall changes determined")

	if len(addPorts) == 0 &&
		len(removePorts) == 0 &&
		len(addSources) == 0 &&
		len(removeSources) == 0 &&
		len(ensureIfaces) == 0 &&
		len(removeIfaces) == 0 &&
		len(ensureTarget) == 0 {
		return current, nil, nil
	}

	zoneNameCopy := zoneName
	return current, func() error {
		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", removeIfaces, zoneName))
		if err := ensure(monitor, removeIfaces, zoneNameCopy); err != nil {
			return err
		}

		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", ensureIfaces, zoneName))
		if err := ensure(monitor, ensureIfaces, zoneNameCopy); err != nil {
			return err
		}
		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", ensureTarget, zoneName))
		if err := ensure(monitor, ensureTarget, zoneNameCopy); err != nil {
			return err
		}

		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", removeSources, zoneName))
		if err := ensure(monitor, removeSources, zoneNameCopy); err != nil {
			return err
		}

		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", addSources, zoneName))
		if err := ensure(monitor, addSources, zoneNameCopy); err != nil {
			return err
		}

		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", removePorts, zoneName))
		if err := ensure(monitor, removePorts, zoneNameCopy); err != nil {
			return err
		}

		monitor.Debug(fmt.Sprintf("Ensuring part of firewall with %s in zone %s", addPorts, zoneName))
		return ensure(monitor, addPorts, zoneNameCopy)
	}, nil
}

func ensure(monitor mntr.Monitor, changes []string, zone string) error {
	if changes == nil || len(changes) == 0 {
		return nil
	}

	if _, err := runCommand(monitor, "systemctl", "enable", "firewalld"); err != nil {
		return err
	}

	if _, err := runCommand(monitor, "systemctl", "start", "firewalld"); err != nil {
		return err
	}

	return changeFirewall(monitor, changes, zone)
}

func changeFirewall(monitor mntr.Monitor, changes []string, zone string) (err error) {
	if len(changes) == 0 {
		return nil
	}

	if _, err := runFirewallCommand(monitor, append([]string{"--permanent", "--zone", zone}, changes...)...); err != nil {
		return err
	}

	return reloadFirewall(monitor)
}

func reloadFirewall(monitor mntr.Monitor) error {

	_, err := runFirewallCommand(monitor, "--reload")
	return err
}

func listFirewall(monitor mntr.Monitor, zone string, arg string) ([]string, error) {

	out, err := runFirewallCommand(monitor, "--zone", zone, arg)
	return strings.Fields(out), err
}

func runFirewallCommand(monitor mntr.Monitor, args ...string) (string, error) {
	return runCommand(monitor, "firewall-cmd", args...)
}

func runCommand(monitor mntr.Monitor, binary string, args ...string) (string, error) {

	outBuf := new(bytes.Buffer)
	defer outBuf.Reset()
	errBuf := new(bytes.Buffer)
	defer errBuf.Reset()

	cmd := exec.Command(binary, args...)
	cmd.Stderr = errBuf
	cmd.Stdout = outBuf

	fullCmd := fmt.Sprintf("'%s'", strings.Join(cmd.Args, "' '"))
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(`running %s failed with stderr %s: %w`, fullCmd, errBuf.String(), err)
	}

	stdout := outBuf.String()
	if monitor.IsVerbose() {
		fmt.Println(fullCmd)
		fmt.Println(stdout)
	}

	return strings.TrimSuffix(stdout, "\n"), nil
}
