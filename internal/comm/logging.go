package comm

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

func GetSugaredLogger() *zap.SugaredLogger {
	return zap.S()
}

func initLog() {
	encoder := getEncoder()
	writeSyncer := getWriteSyncer()
	core := zapcore.NewCore(encoder, writeSyncer, zap.DebugLevel)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	zap.ReplaceGlobals(logger)
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func getWriteSyncer() zapcore.WriteSyncer {
	f, err := os.OpenFile(LogPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	return zapcore.NewMultiWriteSyncer(zapcore.AddSync(f), zapcore.AddSync(os.Stdout))
}
