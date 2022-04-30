package messageQueue

const (
	TypeFriend = "Friend"
	TypeGroup  = "Group"

	PermissionOwner  = "OWNER"
	PermissionAdmin  = "ADMINISTRATOR"
	PermissionMember = "MEMBER"
)

type MsgQueue struct {
	Msg  chan Msg
	Task chan Task
}

type Msg struct {
	Component string `json:"component"`
	Nickname  string `json:"nickname"`
	UUID      string `json:"uuid"`
	Content   string `json:"msg"`
	Level     string `json:"level"`
}

type Task struct {
	Type    string
	Content string
	Sender  Sender
	Group   Group
}

type Sender struct {
	Id         int
	NickName   string
	Remark     string
	MemberName string
	Permission string
}

type Group struct {
	Id         int
	Name       string
	Permission string
}

func NewMQ() *MsgQueue {
	return &MsgQueue{
		Msg:  make(chan Msg, 24),
		Task: make(chan Task, 24),
	}
}
