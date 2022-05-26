package log

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"
	"gopkg.in/natefinch/lumberjack.v2"
)

var mLogger *zap.SugaredLogger
var mLoglevel zap.AtomicLevel
var lk sync.Mutex

func Logger(name string) *zap.SugaredLogger {
	lk.Lock()
	defer lk.Unlock()
	return mLogger.Named(name)
}

// StartLogger starts
func init() {
	mLoglevel = zap.NewAtomicLevel()

	outputs := []string{"stdout"}
	debugWriter, _, err := zap.Open(outputs...)
	if err != nil {
		panic(fmt.Sprintf("unable to open logging output: %v", err))
	}

	lf := os.Getenv("MEFS_LOG_FILE")
	if lf != "" {
		debugWriter = getLogWriter(lf)
	}

	encoder := getEncoder()

	core := zapcore.NewCore(encoder, debugWriter, mLoglevel)

	// NewProduction
	logger := zap.New(core, zap.AddCaller())

	mLogger = logger.Sugar()

	// info level for default
	mLoglevel.SetLevel(zapcore.InfoLevel)

	//mLogger.Info("mefs logger init success")
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "sub",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	return zapcore.NewJSONEncoder(encoderConfig)
}

func getLogWriter(filename string) zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    100, //MB
		MaxBackups: 3,
		MaxAge:     30, //days
		Compress:   false,
	}
	return zapcore.AddSync(lumberJackLogger)
}

func SetLogLevel(level string) error {
	lk.Lock()
	defer lk.Unlock()

	l := zapcore.InfoLevel
	switch level {
	case "debug", "DEBUG":
		l = zapcore.DebugLevel
	case "info", "INFO", "": // make the zero value useful
		l = zapcore.InfoLevel
	case "warn", "WARN":
		l = zapcore.WarnLevel
	case "error", "ERROR":
		l = zapcore.ErrorLevel
	case "dpanic", "DPANIC":
		l = zapcore.DPanicLevel
	case "panic", "PANIC":
		l = zapcore.PanicLevel
	case "fatal", "FATAL":
		l = zapcore.FatalLevel
	default:
		return xerrors.Errorf("level %s is not supported", level)
	}

	mLoglevel.SetLevel(l)
	return nil
}
