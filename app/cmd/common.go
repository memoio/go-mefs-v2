package cmd

import "github.com/urfave/cli/v2"

var CommonCmd []*cli.Command

func init() {
	CommonCmd = []*cli.Command{
		initCmd,
		daemonCmd,
		authCmd,
		walletCmd,
		netCmd,
		configCmd,
		stateCmd,
		roleCmd,
		infoCmd,
		pledgeCmd,
		registerCmd,
		versionCmd,
		backupCmd,
		bootstrapCmd,
		recoverCmd,
		logCmd,
	}
}
