package logging

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(level zerolog.Level) {
	logFile, err := os.OpenFile("/var/log/hvac-controller.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Errorf("failed to open log file: %w", err))
	}

	multi := zerolog.MultiLevelWriter(logFile)

	logger := zerolog.New(multi).Level(level).With().Timestamp().Logger()
	log.Logger = logger

	if level == zerolog.DebugLevel {
		log.Debug().Msg("Log level set to DEBUG")
	}
}
