package cache

import (
	"encoding/json"

	"github.com/vksir/vkiss-lib/pkg/util/errutil"
	"github.com/vksir/vkiss-lib/pkg/util/fileutil"
)

type Cache struct {
	AIUrl        string `toml:"ai_url"`
	AIKey        string `toml:"ai_key"`
	AIChatModel  string `toml:"ai_chat_model"`
	AIEmbedModel string `toml:"ai_embed_model"`
}

var gPath string
var G *Cache

func Save() {
	content, err := json.Marshal(G)
	errutil.Check(err)
	err = fileutil.Write(gPath, content)
	errutil.Check(err)
}

func Init(path string) {
	gPath = path
	G = &Cache{}

	if !fileutil.Exist(path) {
		return
	}

	content, err := fileutil.Read(path)
	errutil.Check(err)
	err = json.Unmarshal(content, G)
	errutil.Check(err)
}
