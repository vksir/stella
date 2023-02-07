package girl

import (
	"github.com/go-resty/resty/v2"
	"sync"
)

const (
	hanHanGirlUrl = "https://api.vvhan.com/api/girl?type=https"
)

func getGirlImgUrls(num int) []string {
	c := resty.New().
		SetRedirectPolicy(resty.NoRedirectPolicy())
	var urls []string
	var lock sync.Mutex
	var ws sync.WaitGroup
	ws.Add(num)

	for i := 0; i < num; i++ {
		go func() {
			resp, _ := c.R().Get(hanHanGirlUrl)
			u := resp.Header().Get("location")
			if u == "" {
				return
			}
			lock.Lock()
			urls = append(urls, u)
			lock.Unlock()
			ws.Done()
		}()
	}
	ws.Wait()
	return urls
}
