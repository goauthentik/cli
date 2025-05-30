package main

import (
	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
	"goauthentik.io/cli/pkg/agent"
	"goauthentik.io/cli/pkg/agent/logs"
)

func main() {
	log.SetLevel(log.DebugLevel)
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://c83cdbb55c9bd568ecfa275932b6de17@o4504163616882688.ingest.us.sentry.io/4509208005312512",
		TracesSampleRate: 0.3,
	})
	if err != nil {
		log.WithError(err).Warn("failed to init sentry")
	}
	err = logs.Setup()
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
