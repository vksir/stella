package consumer

func levelToPriority(level string) int {
	levelMap := map[string]int{
		"debug":    10,
		"info":     20,
		"warning":  30,
		"error":    40,
		"critical": 50,
	}
	return levelMap[level]
}

func levelAllow(msgLevel, userLevel string) bool {
	return levelToPriority(userLevel) > levelToPriority(msgLevel)
}
