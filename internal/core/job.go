package core

import (
	"context"
	"stella/internal/comm"
	"sync"
)

var log = comm.GetSugaredLogger()

type Job interface {
	Name() string
	Run(ctx context.Context) error
}

var jobs []Job

func RegisterJob(j Job) {
	log.Warnf("register job: %s", j.Name())
	jobs = append(jobs, j)
}

func JobLoop(ctx context.Context) {
	go func() {
		w := sync.WaitGroup{}

		for _, j := range jobs {
			w.Add(1)

			go func(j Job) {
				defer w.Done()

				log.Warnf("[%s] job running", j.Name())
				if err := j.Run(ctx); err != nil {
					log.Errorf("[%s] job exit: %s", j.Name(), err)
				} else {
					log.Warnf("[%s] job exit", j.Name())
				}
			}(j)
		}

		w.Wait()
		log.Warnf("exit job loop")
	}()
}
