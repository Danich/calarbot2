package main

import "calarbot2/common"

const configPath = "/calarbot.yaml"

func main() {
	config := &CalarbotConfig{}
	err := common.ReadConfig(configPath, config)
	if err != nil {
		panic(err)
	}

	bot := Bot{}
	bot.InitBot(config)
	bot.RunBot()
}
