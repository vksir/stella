package job

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
	"net/url"
	"sort"
	"stella/internal/core"
	"stella/internal/driver/cqhttp"
	"time"
)

type TModJob struct {
}

func (t *TModJob) Name() string {
	return "TMod"
}

func (t *TModJob) Run(ctx context.Context) error {
	var latestTime int64
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			events, err := getTModEvents()
			if err != nil {
				log.Infof("[%s] get events failed: %s", t.Name(), err)
				time.Sleep(60 * time.Second)
				continue
			}

			sort.Sort(TModReportEventList(events))
			for _, e := range events {
				if e.Time >= latestTime {
					log.Infof("[%s] report event: %+v", t.Name(), e)
					latestTime = e.Time

					msg := fmt.Sprintf("[TModLoader] %s", e.Msg)

					if err := cqhttp.SendMsg(cqhttp.MessageTypeGroup, viper.GetInt64("cqhttp.report_id"), msg); err != nil {
						log.Errorf("send msg failed: %s", err)
					}
				}
			}
			latestTime++
			time.Sleep(3 * time.Second)
		}
	}
}

type TModReportEvent struct {
	Time  int64
	Msg   string
	Level string
	Type  string
}

func getTModEvents() ([]*TModReportEvent, error) {
	resp, err := resty.New().R().Get(getNSAddr("tmodloader/events"))
	if err != nil {
		return nil, err
	}

	var events []*TModReportEvent
	if err := json.Unmarshal(resp.Body(), &events); err != nil {
		return nil, err
	}
	return events, nil
}

func getNSAddr(endpoint string) string {
	u, _ := url.JoinPath(viper.GetString("neutronstar.addr"), endpoint)
	return u
}

type TModReportEventList []*TModReportEvent

func (s TModReportEventList) Len() int {
	return len(s)
}

func (s TModReportEventList) Less(i, j int) bool {
	return s[i].Time < s[j].Time
}

func (s TModReportEventList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func initTModJob() {
	t := TModJob{}
	core.RegisterJob(&t)
}

func initNSJob() {
	initTModJob()
}
