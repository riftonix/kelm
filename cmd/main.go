package main

import (
	"kelm-operator/internal/app"
	"kelm-operator/internal/pkg/logger"
)

func main() {
	logger.Setup()
	kelm.Init()
}
