package terrariarun

import (
	"context"
	"qq-bot-go/internal/common/logging"
	"sort"
	"time"
)

var log = logging.GetSugaredLogger()

type Listener struct {
	c              *Client
	reportChannels []chan *Event
}

func NewListener() *Listener {
	l := Listener{
		c: NewClient(),
	}
	return &l
}

func (l *Listener) Start(ctx context.Context) error {
	go l.watchReportEvents(ctx)
	return nil
}

func (l *Listener) RegisterChannel(c chan *Event) {
	l.reportChannels = append(l.reportChannels, c)
}

func (l *Listener) watchReportEvents(ctx context.Context) {
	var latestTime int64
	for {
		select {
		case <-ctx.Done():
			log.Info("Watch report events stopped")
			return
		default:
			resp, err := l.c.GetGameEvents()
			if err != nil {
				log.Error("Get game events failed", err)
				time.Sleep(15 * time.Second)
				break
			}
			events := sorter(resp.Events)
			sort.Sort(events)
			for _, e := range events {
				if e.Time >= latestTime {
					log.Infof("Report event: %+v", e)
					latestTime = e.Time
					for _, c := range l.reportChannels {
						c <- e
					}
				}
			}
			latestTime++
			time.Sleep(3 * time.Second)
		}
	}
}
