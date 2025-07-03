package main

func main() {
	config := &CalarbotConfig{}
	err := config.Read()
	if err != nil {
		panic(err)
	}

	bot := Bot{}
	bot.InitBot(config)
	bot.RunBot()
}
