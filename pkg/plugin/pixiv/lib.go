package pixiv

import (
	"encoding/json"
	"github.com/go-resty/resty/v2"
	"strconv"
)

func GetPixivImageUrls(num int) []string {
	resp, err := resty.New().R().
		SetQueryParam("num", strconv.Itoa(num)).
		Get("https://api.vksir.zone/pixiv")
	var data struct {
		ImgUrls []string `json:"img_urls"`
	}
	if err = json.Unmarshal(resp.Body(), &data); err != nil {
		return []string{}
	} else {
		return data.ImgUrls
	}
}
