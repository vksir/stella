package job

import (
	"stella/internal/comm"
)

var log = comm.GetSugaredLogger()

func InitJob() {
	initNSJob()
}
