package main

import (
	"github.com/caos/orbos/internal/api"
	boomapi "github.com/caos/orbos/internal/operator/boom/api"
	"github.com/caos/orbos/internal/operator/orbiter/kinds/orb"
	"github.com/caos/orbos/internal/tree"
	"github.com/spf13/cobra"
)

func APICommand(rv RootValues) *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:   "api",
			Short: "Upgrade the yml-files to the newest version",
			Long:  "Upgrade the yml-files to the newest version",
		}
	)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		_, monitor, orbConfig, gitClient, errFunc := rv()
		if errFunc != nil {
			return errFunc(cmd)
		}

		if err := gitClient.Configure(orbConfig.URL, []byte(orbConfig.Repokey)); err != nil {
			return err
		}

		foundOrbiter, err := api.ExistsOrbiterYml(gitClient)
		if err != nil {
			return err
		}

		if foundOrbiter {
			adaptFunc := orb.AdaptFunc(orbConfig, gitCommit, true, false)

			desired, err := api.ReadOrbiterYml(gitClient)
			if err != nil {
				return err
			}

			finishedChan := make(chan struct{})
			_, _, migrate, err := adaptFunc(monitor, finishedChan, desired, &tree.Tree{})
			if migrate {
				if err := api.PushOrbiterYml(monitor, "Update orbiter.yml", gitClient, desired); err != nil {
					return err
				}
			}

		}
		foundBoom, err := api.ExistsBoomYml(gitClient)
		if err != nil {
			return err
		}
		if foundBoom {

			desired, err := api.ReadBoomYml(gitClient)
			if err != nil {
				return err
			}

			toolset, migrate, err := boomapi.ParseToolset(desired)
			if migrate {
				desired.Parsed = toolset
				if err := api.PushBoomYml(monitor, "Update boom.yml", gitClient, desired); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return cmd
}
