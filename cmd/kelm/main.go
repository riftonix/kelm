package main

import (
	kelm "kelm/internal/app"
	"kelm/internal/pkg/logger"
)

func main() {
	logger.Setup()
	kelm.Init()
}
