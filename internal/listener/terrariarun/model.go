package terrariarun

type Events struct {
	Events []*Event `json:"events"`
}

type Event struct {
	Level string `json:"level"`
	Time  int64  `json:"time"`
	Msg   string `json:"msg"`
	Type  string `json:"type"`
}

type sorter []*Event

func (s sorter) Len() int {
	return len(s)
}

func (s sorter) Less(i, j int) bool {
	return s[i].Time < s[j].Time
}

func (s sorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
