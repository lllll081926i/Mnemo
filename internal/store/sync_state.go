package store

import (
	"errors"
	"os"
	"time"

	"mnemo-go/internal/model"
)

type SyncRunState struct {
	Status    string                `json:"status"`
	Issue     *model.OperationIssue `json:"issue,omitempty"`
	UpdatedAt int64                 `json:"updatedAt"`
}

func (s *Store) ListSyncRunStates() (map[string]SyncRunState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states := map[string]SyncRunState{}
	err := s.readJSON("sync_run_states.json", &states)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return states, err
}

func (s *Store) SaveSyncRunState(id string, state SyncRunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		return errors.New("同步任务 ID 为空")
	}
	states := map[string]SyncRunState{}
	if err := s.readJSON("sync_run_states.json", &states); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if states == nil {
		states = map[string]SyncRunState{}
	}
	state.UpdatedAt = time.Now().Unix()
	states[id] = state
	return s.writeJSONUnlocked("sync_run_states.json", states)
}
