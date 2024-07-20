package main

import (
	"path"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port int `yaml:"port"`
}

var conf = Config{
	Port: 37015,
}

func LoadConf(run string) (bool, error) {
	bytes, _ := yaml.Marshal(conf)
	data, err := ReadOrCreateFile(path.Join(run, "config.yaml"), bytes)
	if err != nil {
		logrus.Warn("配置文件 config.yaml 不存在,现在已创建")
	}
	err2 := yaml.Unmarshal(data, &conf)
	if err2 != nil {
		logrus.Error("解释配置文件时出错", err2)
		return false, err2
	}
	if conf.Port < 20 || conf.Port > 65534 {
		logrus.Warn("警告! 由于配置文件中端口超出系统范围(20 - 65534),已更改为 80")
		conf.Port = 80
	}
	return true, nil
}
