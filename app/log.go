package app

import "go.uber.org/zap"

var Sugar *zap.SugaredLogger

func InitLogger() {
	// think about adding an environment variable
	// that specifies whether to log in production
	// or development mode
	config := zap.NewDevelopmentConfig()
	config.OutputPaths = []string{"./app.log", "stdout"}
	logger, _ := config.Build()

	defer logger.Sync()
	Sugar = logger.Sugar()
}
