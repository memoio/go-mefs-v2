package log

import (
	"github.com/memoio/go-mefs-v2/lib/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var mLogger *zap.SugaredLogger

func Logger(name string) *zap.SugaredLogger {
	return mLogger.Named(name)
}

// StartLogger starts
func init() {

	debugLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.DebugLevel
	})

	debugWriter := getLogWriter("debug")

	encoder := getEncoder()

	core := zapcore.NewCore(encoder, debugWriter, debugLevel)

	// NewProduction
	logger := zap.New(core, zap.AddCaller())

	mLogger = logger.Sugar()

	mLogger.Info("mefs logger init success")
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
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
	root, err := utils.GetMefsPath()
	if err != nil {
		root = "~/.memo"
	}

	lumberJackLogger := &lumberjack.Logger{
		Filename:   root + "/logs/" + filename + ".log",
		MaxSize:    100, //MB
		MaxBackups: 3,
		MaxAge:     30, //days
		Compress:   false,
	}
	return zapcore.AddSync(lumberJackLogger)
}
