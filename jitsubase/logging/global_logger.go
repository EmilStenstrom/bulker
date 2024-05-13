package logging

import (
	"errors"
	"fmt"
	"github.com/jitsucom/bulker/jitsubase/timestamp"
	log "github.com/sirupsen/logrus"
	"io"
)

const (
	errPrefix   = "[ERROR]:"
	warnPrefix  = "[WARN]:"
	infoPrefix  = "[INFO]:"
	debugPrefix = "[DEBUG]:"

	GlobalType = "global"
)

var GlobalLogsWriter io.Writer
var ConfigErr string
var ConfigWarn string

var LogLevel = UNKNOWN

type Config struct {
	FileName    string
	FileDir     string
	RotationMin int64
	MaxBackups  int
	Compress    bool

	RotateOnClose bool
}

func (c Config) Validate() error {
	if c.FileName == "" {
		return errors.New("Logger file name can't be empty")
	}
	if c.FileDir == "" {
		return errors.New("Logger file dir can't be empty")
	}

	return nil
}

// InitGlobalLogger initializes main logger
func InitGlobalLogger(writer io.Writer, levelStr string) error {
	level, err := log.ParseLevel(levelStr)
	if err == nil {
		log.SetLevel(level)
	} else {
		Error(err)
	}
	if ConfigErr != "" {
		Error(ConfigErr)
	}

	if ConfigWarn != "" {
		Warn(ConfigWarn)
	}
	return nil
}

func SetJsonFormatter() {
	log.SetFormatter(&log.JSONFormatter{})
}

func SetTextFormatter() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: timestamp.LogsLayout,
	})
}

func SystemErrorf(format string, v ...any) {
	SystemError(fmt.Sprintf(format, v...))
}

func SystemError(v ...any) {
	msg := []any{"System error:"}
	msg = append(msg, v...)
	Error(msg...)
	//TODO: implement system error notification
	//notifications.SystemError(msg...)
}

func Errorf(format string, v ...any) {
	log.Errorf(format, v...)
}

func Error(v ...any) {
	log.Errorln(v...)
}

func Infof(format string, v ...any) {
	log.Infof(format, v...)
}

func Info(v ...any) {
	log.Infoln(v...)
}

func Debugf(format string, v ...any) {
	log.Debugf(format, v...)
}

func IsDebugEnabled() bool {
	return log.IsLevelEnabled(log.DebugLevel)
}

func Debug(v ...any) {
	log.Debug(v...)
}

func Warnf(format string, v ...any) {
	log.Warnf(format, v...)
}

func Warn(v ...any) {
	log.Warnln(v...)
}

func Fatal(v ...any) {
	log.Fatal(v...)
}

func Fatalf(format string, v ...any) {
	log.Fatalf(format, v...)
}
