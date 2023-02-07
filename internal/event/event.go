package event

const (
	ChainPlain = "Plain"
	ChainImage = "Image"
	ChainVoice = "Voice"
)

type Send struct {
	Chains []Chain `json:"chains"`
}

func (s *Send) AppendChain(chainType, text, url, base64 string) {
	c := NewChain(chainType, text, url, base64)
	s.Chains = append(s.Chains, c)
}

type Receive struct {
	Chains []Chain `json:"receive"`
}

func (r Receive) GetAllPlainText() string {
	var text string
	for _, e := range r.Chains {
		if e.Type == ChainPlain {
			text += e.Text
		}
	}
	return text
}

func (r *Receive) AppendChain(chainType, text, url, base64 string) {
	c := NewChain(chainType, text, url, base64)
	r.Chains = append(r.Chains, c)
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

type Interface interface {
	ToEvent() *Event
}

type Event struct {
	Chains []Chain
}

type Chain struct {
	Type   string
	Text   string
	Url    string
	Base64 string
}

func (e *Event) GetAllPlainText() string {
	var text string
	for _, c := range e.Chains {
		if c.Type == ChainPlain {
			text += c.Text
		}
	}
	return text
}
