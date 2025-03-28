package cli

import (
	"fmt"
	"os"
	"os/user"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"goauthentik.io/cli/pkg/auth/raw"
	"golang.org/x/crypto/ssh"
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Establish an SSH connection with `host`.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profile := mustFlag(cmd.Flags().GetString("profile"))

		u, err := user.Current()
		if err != nil {
			log.WithError(err).Warning("failed to get user")
			os.Exit(1)
		}

		_ = raw.GetCredentials(cmd.Context(), raw.CredentialsOpts{
			Profile:  profile,
			ClientID: "authentik-pam",
		})

		config := &ssh.ClientConfig{
			User: fmt.Sprintf("%s@ak-token", u.Username),
			Auth: []ssh.AuthMethod{
				ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
					fmt.Println(name, instruction, questions, echos)
					return []string{}, nil
				}),
			},
			// TODO
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", args[0]), config)
		if err != nil {
			log.Fatal("Failed to dial: ", err)
		}
		defer client.Close()
		// Each ClientConn can support multiple interactive sessions,
		// represented by a Session.
		session, err := client.NewSession()
		if err != nil {
			log.Fatal("Failed to create session: ", err)
		}
		defer session.Close()

		session.Stderr = os.Stderr
		session.Stdout = os.Stdout
		session.Stdin = os.Stdin
		session.Shell()
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
