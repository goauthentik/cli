package main

import (
	log "github.com/sirupsen/logrus"
	"goauthentik.io/cli/pkg/agent"
	systemlog "goauthentik.io/cli/pkg/system_log"
)

func main() {
	log.SetLevel(log.DebugLevel)
	err := systemlog.Setup()
	if err != nil {
		log.WithError(err).Warning("failed to setup logs")
	}
	a, err := agent.New()
	if err != nil {
		log.WithError(err).Warning("failed to start agent")
		return
	}
	a.Start()
}
