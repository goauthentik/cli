package agentsystem

import (
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"goauthentik.io/platform/pkg/agent_system/config"
	"goauthentik.io/platform/pkg/storage"
)

var configFile string
var defaultConfigFile string

var rootCmd = &cobra.Command{
	Use:   "ak-sysd",
	Short: fmt.Sprintf("authentik System Agent v%s", storage.FullVersion()),
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config-file", defaultConfigFile, "Config file path")
}

func agentPrecheck() error {
	if runtime.GOOS != "windows" {
		if os.Getuid() != 0 {
			return errors.New("authentik system agent must run as root")
		}
	}
	if _, err := os.Stat(configFile); err != nil {
		return errors.Wrap(err, "failed to check config file")
	}
	return config.Init(configFile)
}
