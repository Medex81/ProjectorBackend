// pkg/common/logger/logger.go
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var log zerolog.Logger

func Init(serviceName, environment string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05.000",
	}

	if environment == "production" {
		log = zerolog.New(os.Stdout).With().
			Timestamp().
			Str("service", serviceName).
			Str("env", environment).
			Logger()
	} else {
		log = zerolog.New(output).With().
			Timestamp().
			Str("service", serviceName).
			Str("env", environment).
			Logger().Level(zerolog.DebugLevel)
	}
}

func GetLogger() *zerolog.Logger {
	return &log
}

func Info() *zerolog.Event {
	return log.Info()
}

func Error() *zerolog.Event {
	return log.Error()
}

func Debug() *zerolog.Event {
	return log.Debug()
}

func Warn() *zerolog.Event {
	return log.Warn()
}

func Fatal() *zerolog.Event {
	return log.Fatal()
}
