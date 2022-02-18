package cmd

import "github.com/urfave/cli/v2"

var CommonCmd []*cli.Command

func init() {
	CommonCmd = []*cli.Command{
		InitCmd,
		DaemonCmd,
		AuthCmd,
		WalletCmd,
		NetCmd,
		ConfigCmd,
		StateCmd,
		RoleCmd,
		InfoCmd,
		PledgeCmd,
		registerCmd,
	}
}
