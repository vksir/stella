package agent

import (
	"sync"
)

type SessionManager struct {
	sessions sync.Map
}

func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

func (sm *SessionManager) GetSession(sessionID string) any {
	s, ok := sm.sessions.Load(sessionID)
	if ok {
		return s
	}
	return nil
}

func (sm *SessionManager) SetSession(sessionID string, session any) {
	sm.sessions.Store(sessionID, session)
}
