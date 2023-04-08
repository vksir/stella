package config

type Config struct {
	Bot struct {
		Mirai struct {
			Me     string `json:"me"`
			Host   string `json:"host"`
			Port   int    `json:"port"`
			Report struct {
				Friend []struct {
					Id    string `json:"id"`
					Level string `json:"level"`
				} `json:"friend"`
				Group []struct {
					Id    string `json:"id"`
					Level string `json:"level"`
				} `json:"group"`
			} `json:"report"`
		} `json:"mirai"`
	} `json:"bot"`
	Listener struct {
		TerrariaRun struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"terraria-run"`
	} `json:"listener"`
	BingUrl string `json:"bing_url"`
}
