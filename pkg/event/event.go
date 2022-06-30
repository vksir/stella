package event

const (
	TypePlain = "Plain"
	TypeImage = "Image"
	TypeVoice = "Voice"
)

type Send struct {
	Send []Chain `json:"send"`
}

func (s *Send) AppendChain(chainType, text, url, base64 string) {
	c := NewChain(chainType, text, url, base64)
	s.Send = append(s.Send, c)
}

type Receive struct {
	Receive []Chain `json:"receive"`
}

func (r Receive) GetAllPlainText() string {
	var text string
	for _, e := range r.Receive {
		if e.Type == TypePlain {
			text += e.Text
		}
	}
	return text
}

func (r *Receive) AppendChain(chainType, text, url, base64 string) {
	c := NewChain(chainType, text, url, base64)
	r.Receive = append(r.Receive, c)
}

type Chain struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Url    string `json:"url"`
	Base64 string `json:"base64"`
}

func NewChain(chainType, text, url, base64 string) Chain {
	c := Chain{Type: chainType}
	if text != "" {
		c.Text = text
	}
	if url != "" {
		c.Url = url
	}
	if base64 != "" {
		c.Base64 = base64
	}
	return c
}
