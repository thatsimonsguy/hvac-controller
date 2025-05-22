package datadog

import (
	"github.com/DataDog/datadog-go/statsd"
	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/env"
)

var dogstatsd *statsd.Client

func InitMetrics() {
	var err error
	dogstatsd, err = statsd.New(env.Cfg.DDAgentAddr)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create DogStatsD client")
		return
	}

	dogstatsd.Namespace = env.Cfg.DDNamespace
	dogstatsd.Tags = env.Cfg.DDTags

	log.Info().
		Str("addr", env.Cfg.DDAgentAddr).
		Str("namespace", env.Cfg.DDNamespace).
		Strs("tags", env.Cfg.DDTags).
		Msg("Datadog metrics initialized")
}

func Gauge(name string, value float64, tags ...string) {
	if dogstatsd != nil {
		err := dogstatsd.Gauge(name, value, tags, 1)
		if err != nil && env.Cfg.EnableDatadog {
			log.Warn().Err(err).Str("metric", name).Msg("Failed to emit gauge metric")
		}
	}
}
