package main

import (
	"context"
	"stella/internal/comm"
	"stella/internal/core"
	"stella/internal/job"
	"stella/internal/server"
)

var log = comm.GetSugaredLogger()

func main() {
	log.Info("Hello stella ^_^")

	job.InitJob()

	core.JobLoop(context.Background())
	server.Run()
}
