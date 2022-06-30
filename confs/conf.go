package confs

import (
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"qq-bot-go/common"
)

var CONF *Conf

type Conf struct {
	Mirai    Server `yaml:"mirai"`
	NsServer Server `yaml:"ns_server"`
	Task     Users  `yaml:"task"`
	Report   Users  `yaml:"report"`
}

type Server struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Users []struct {
	Id        int        `yaml:"id"`
	Type      string     `yaml:"type"`
	Level     string     `yaml:"level"`
	Component Components `yaml:"component"`
}

type Components []struct {
	Name  string `yaml:"name"`
	UUID  string `yaml:"uuid"`
	Short string `yaml:"short"`
}

func NewConf(f *common.FilePath) {
	c := Conf{}

	data, err := os.ReadFile(f.ConfigPath)
	if err != nil {
		c.Save(f)
	}
	err = yaml.Unmarshal(data, &c)
	if err != nil {
		log.Panicln(err)
	}
	CONF = &c
}

func (c *Conf) Save(f *common.FilePath) {
	data, err := yaml.Marshal(c)
	if err != nil {
		log.Panicln(err)
	}
	err = os.WriteFile(f.ConfigPath, data, 655)
	if err != nil {
		log.Panicln(err)
	}
}
