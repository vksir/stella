package comm

import (
	"fmt"
	"github.com/spf13/viper"
	"net/url"
	"stella/assets"
)

func SaveConfig() {
	if err := viper.WriteConfigAs(ConfigPath); err != nil {
		panic(err)
	}
}

func checkConfig() {
	var s string

	s = viper.GetString("cqhttp.addr")
	if _, err := url.Parse(s); err != nil {
		panic(fmt.Sprintf("invalid cqhttp.addr: %s, %s", s, err))
	}

	s = viper.GetString("neutronstar.addr")
	if _, err := url.Parse(s); err != nil {
		panic(fmt.Sprintf("invalid neutronstar.addr: %s, %s", s, err))
	}
}

func initConfig() {
	viper.SetConfigName("stella")
	viper.SetConfigType("toml")
	viper.AddConfigPath(StellaHome)
	if err := viper.ReadInConfig(); err != nil {
		r, err := assets.FS.Open("assets/config.toml")
		if err != nil {
			panic(err)
		}
		if err := viper.ReadConfig(r); err != nil {
			panic(err)
		}
		SaveConfig()
	}
	checkConfig()
}
