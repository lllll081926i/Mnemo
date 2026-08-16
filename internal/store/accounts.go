package store

import (
	"encoding/json"
	"os"
	"sort"

	"mnemo-go/internal/model"
	"mnemo-go/internal/vault"
)

const accountsFile = "accounts.json"

// readAccounts reads the encrypted accounts file from the accounts dir.
func (s *Store) readAccounts() ([]*model.Account, error) {
	p := s.path(accountsFile)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	vaultDir := s.accountsDir
	if vaultDir == "" {
		vaultDir = s.dir
	}
	// Try decrypt; if it fails (legacy plaintext), fall back to raw JSON.
	var list []*model.Account
	plain, derr := vault.Decrypt(string(b), vaultDir)
	if derr == nil {
		if err := json.Unmarshal(plain, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	// legacy plaintext migration
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// writeAccounts encrypts and writes the accounts list.
func (s *Store) writeAccounts(list []*model.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	vaultDir := s.accountsDir
	if vaultDir == "" {
		vaultDir = s.dir
	}
	enc, err := vault.Encrypt(b, vaultDir)
	if err != nil {
		return err
	}
	p := s.path(accountsFile)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(enc), 0o600); err != nil {
		return err
	}
	return renameWithRetry(tmp, p)
}

// ListAccounts returns all persisted accounts sorted by order then user id.
func (s *Store) ListAccounts() ([]*model.Account, error) {
	list, err := s.readAccounts()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].UserID < list[j].UserID
	})
	return list, nil
}

// GetAccount returns one account by user id.
func (s *Store) GetAccount(userID string) (*model.Account, error) {
	list, err := s.ListAccounts()
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		if a.UserID == userID {
			return a, nil
		}
	}
	return nil, os.ErrNotExist
}

// SaveAccount upserts an account.
func (s *Store) SaveAccount(account *model.Account) error {
	if account == nil || account.UserID == "" {
		return errInvalid("account user_id is empty")
	}
	list, err := s.readAccounts()
	if err != nil {
		return err
	}
	replaced := false
	for i, a := range list {
		if a.UserID == account.UserID {
			list[i] = account
			replaced = true
			break
		}
	}
	if !replaced {
		if account.Order == 0 {
			account.Order = int64(len(list)) + 1
		}
		list = append(list, account)
	}
	return s.writeAccounts(list)
}

// DeleteAccount removes an account by user id.
func (s *Store) DeleteAccount(userID string) error {
	list, err := s.readAccounts()
	if err != nil {
		return err
	}
	out := list[:0]
	for _, a := range list {
		if a.UserID != userID {
			out = append(out, a)
		}
	}
	return s.writeAccounts(out)
}

// UpdateAccountToken refreshes the token of an account.
func (s *Store) UpdateAccountToken(userID string, token *model.TokenInfo) error {
	a, err := s.GetAccount(userID)
	if err != nil {
		return err
	}
	a.Token = token
	return s.SaveAccount(a)
}